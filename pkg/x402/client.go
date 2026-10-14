package x402

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultMaxResponseBodyBytes  = int64(4 << 20)
	defaultMaxControlBodyBytes   = int64(1 << 20)
	defaultMaxRequestHeaders     = 64
	defaultMaxRequestHeaderBytes = 8 << 10
	defaultMaxPaymentHeaderBytes = 64 << 10
)

// HTTPClient is the subset of *http.Client used by Client.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// PaymentBuilder creates the scheme-specific payload for one selected payment
// requirement. For Stellar this builds and signs the Soroban transfer
// authorization transaction.
type PaymentBuilder interface {
	CreatePaymentPayload(ctx context.Context, required *PaymentRequired, accepted *PaymentRequirements) (*PaymentPayload, error)
}

// Config configures an x402 HTTP client.
type Config struct {
	HTTPClient          HTTPClient
	PaymentBuilder      PaymentBuilder
	Network             string
	MaxResponseBodySize int64
}

// Request describes the protected resource request.
type Request struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    []byte
}

// FetchOptions controls payment handling for a request.
type FetchOptions struct {
	// DryRun performs the 402 challenge and builds a signed payment payload, but
	// does not send the paid retry request.
	DryRun bool
	// MaxAtomicAmount is an optional client-side spend cap. It compares against
	// the requirement's atomic integer amount.
	MaxAtomicAmount string
}

// Result is the outcome of the HTTP resource request and, when applicable, the
// x402 payment retry.
type Result struct {
	StatusCode       int                  `json:"statusCode"`
	FinalURL         string               `json:"finalUrl"`
	Headers          map[string][]string  `json:"headers"`
	Body             []byte               `json:"-"`
	BodyText         string               `json:"body"`
	PaymentRequired  *PaymentRequired     `json:"paymentRequired,omitempty"`
	Accepted         *PaymentRequirements `json:"accepted,omitempty"`
	PaymentPayload   *PaymentPayload      `json:"paymentPayload,omitempty"`
	PaymentAttempted bool                 `json:"paymentAttempted"`
	DryRun           bool                 `json:"dryRun"`
	Settlement       *SettlementResponse  `json:"settlement,omitempty"`
}

// Client performs the HTTP 402 challenge/payment/retry flow.
type Client struct {
	httpClient     HTTPClient
	builder        PaymentBuilder
	network        string
	maxBody        int64
	maxControlBody int64
}

func NewClient(cfg Config) (*Client, error) {
	network, err := CanonicalNetwork(cfg.Network)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if hc, ok := httpClient.(*http.Client); ok {
		// Prevent a redirect from carrying PAYMENT-SIGNATURE to another origin.
		clone := *hc
		clone.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		httpClient = &clone
	}
	maxBody := cfg.MaxResponseBodySize
	if maxBody <= 0 {
		maxBody = defaultMaxResponseBodyBytes
	}
	return &Client{
		httpClient:     httpClient,
		builder:        cfg.PaymentBuilder,
		network:        network,
		maxBody:        maxBody,
		maxControlBody: defaultMaxControlBodyBytes,
	}, nil
}

// Fetch requests a resource. If the resource responds with an x402 v2 HTTP 402
// challenge, it selects the matching Stellar exact requirement, builds the
// signed Soroban transfer payload, and retries once with PAYMENT-SIGNATURE.
func (c *Client) Fetch(ctx context.Context, request Request, options FetchOptions) (*Result, error) {
	if strings.TrimSpace(request.URL) == "" {
		return nil, fmt.Errorf("resource URL is required")
	}
	if _, err := canonicalResourceURL(request.URL); err != nil {
		return nil, err
	}
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	if strings.ContainsAny(request.Method, " \t\r\n") {
		return nil, fmt.Errorf("invalid HTTP method %q", request.Method)
	}
	if len(request.Body) > int(c.maxControlBody) {
		return nil, fmt.Errorf("request body exceeds %d bytes", c.maxControlBody)
	}
	if len(request.Headers) > defaultMaxRequestHeaders {
		return nil, fmt.Errorf("request contains more than %d headers", defaultMaxRequestHeaders)
	}
	for name, value := range request.Headers {
		if name == "" || strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("request contains an invalid header")
		}
		if len(value) > defaultMaxRequestHeaderBytes {
			return nil, fmt.Errorf("request header %s exceeds %d bytes", name, defaultMaxRequestHeaderBytes)
		}
	}
	request.Headers = cloneHeadersWithoutPaymentSignature(request.Headers)

	resp, body, err := c.do(ctx, request)
	if err != nil {
		return nil, err
	}
	result := responseResult(resp, body)
	result.DryRun = options.DryRun
	if err := validateFinalURL(resp, request.URL); err != nil {
		return result, err
	}
	if resp.StatusCode != http.StatusPaymentRequired {
		return result, nil
	}

	required, err := decodePaymentRequired(resp.Header, body)
	if err != nil {
		return result, err
	}
	if required.Resource == nil {
		required.Resource = &ResourceInfo{URL: request.URL}
	} else if strings.TrimSpace(required.Resource.URL) == "" {
		required.Resource.URL = request.URL
	} else if err := validateResourceURL(request.URL, required.Resource.URL); err != nil {
		return result, err
	}
	result.PaymentRequired = required

	accepted, err := SelectStellarRequirement(required, c.network)
	if err != nil {
		return result, err
	}
	result.Accepted = accepted

	if options.MaxAtomicAmount != "" {
		limit, err := ParseAtomicAmount(options.MaxAtomicAmount)
		if err != nil {
			return result, fmt.Errorf("invalid maxAtomicAmount: %w", err)
		}
		amount, _ := ParseAtomicAmount(accepted.Amount)
		if amount.Cmp(limit) > 0 {
			return result, fmt.Errorf("payment amount %s exceeds maxAtomicAmount %s", accepted.Amount, options.MaxAtomicAmount)
		}
	}

	if c.builder == nil {
		return result, fmt.Errorf("no Stellar x402 payment builder configured")
	}
	payload, err := c.builder.CreatePaymentPayload(ctx, required, accepted)
	if err != nil {
		return result, fmt.Errorf("create Stellar payment payload: %w", err)
	}
	if err := validatePaymentPayload(payload, accepted); err != nil {
		return result, err
	}
	result.PaymentPayload = payload

	if options.DryRun {
		return result, nil
	}

	signature, err := EncodePaymentSignatureHeader(payload)
	if err != nil {
		return result, err
	}
	if len(signature) > defaultMaxPaymentHeaderBytes {
		return result, fmt.Errorf("%s exceeds %d bytes", HeaderPaymentSignature, defaultMaxPaymentHeaderBytes)
	}

	retry := request
	retry.Headers = make(map[string]string, len(request.Headers)+1)
	for name, value := range request.Headers {
		retry.Headers[name] = value
	}
	retry.Headers[HeaderPaymentSignature] = signature

	resp, body, err = c.do(ctx, retry)
	if err != nil {
		return result, err
	}
	result = responseResult(resp, body)
	if err := validateFinalURL(resp, request.URL); err != nil {
		return result, err
	}
	result.PaymentRequired = required
	result.Accepted = accepted
	result.PaymentPayload = payload
	result.PaymentAttempted = true

	if header := resp.Header.Get(HeaderPaymentResponse); header != "" {
		if int64(len(header)) > c.maxControlBody {
			return result, fmt.Errorf("%s header exceeds %d bytes", HeaderPaymentResponse, c.maxControlBody)
		}
		settlement, err := DecodePaymentResponseHeader(header)
		if err != nil {
			return result, err
		}
		result.Settlement = settlement
		if !settlement.Success {
			return result, fmt.Errorf("x402 settlement failed: %s %s", settlement.ErrorReason, settlement.ErrorMessage)
		}
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		if refreshed, parseErr := decodePaymentRequired(resp.Header, body); parseErr == nil {
			result.PaymentRequired = refreshed
		}
		return result, fmt.Errorf("resource server rejected the x402 payment")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return result, fmt.Errorf("resource request failed after x402 payment: HTTP %d", resp.StatusCode)
	}
	return result, nil
}

func (c *Client) do(ctx context.Context, request Request) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("build resource request: %w", err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(request.Body)), nil
	}
	for name, value := range request.Headers {
		req.Header.Set(name, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("resource request failed: %w", err)
	}
	defer resp.Body.Close()

	limit := c.maxBody
	if resp.StatusCode == http.StatusPaymentRequired {
		limit = c.maxControlBody
	}
	body, err := readBody(resp.Body, limit)
	if err != nil {
		return resp, nil, err
	}
	return resp, body, nil
}

func cloneHeadersWithoutPaymentSignature(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for name, value := range headers {
		if strings.EqualFold(name, HeaderPaymentSignature) ||
			strings.EqualFold(name, HeaderPaymentRequired) ||
			strings.EqualFold(name, HeaderPaymentResponse) {
			continue
		}
		cloned[name] = value
	}
	return cloned
}

func validateFinalURL(resp *http.Response, requestURL string) error {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return nil
	}
	expected, err := canonicalResourceURL(requestURL)
	if err != nil {
		return err
	}
	actual, err := canonicalResourceURL(resp.Request.URL.String())
	if err != nil {
		return fmt.Errorf("resource response URL is invalid: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("resource redirected from %q to %q; refusing to carry x402 credentials", requestURL, resp.Request.URL.String())
	}
	return nil
}

func validateResourceURL(requestURL, challengeURL string) error {
	expected, err := canonicalResourceURL(requestURL)
	if err != nil {
		return err
	}
	actual, err := canonicalResourceURL(challengeURL)
	if err != nil {
		return fmt.Errorf("payment challenge resource URL is invalid: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("payment challenge resource URL %q does not match requested resource %q", challengeURL, requestURL)
	}
	return nil
}

func canonicalResourceURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" ||
		(u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("resource URL must be a valid HTTP or HTTPS URL")
	}
	if u.User != nil {
		return "", fmt.Errorf("resource URL must not contain credentials")
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	defaultPort := map[string]string{"http": "80", "https": "443"}[scheme]
	if port != "" && port != defaultPort {
		host += ":" + port
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonical := scheme + "://" + host + path
	if query := u.Query().Encode(); query != "" {
		canonical += "?" + query
	}
	return canonical, nil
}

func readBody(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return body, nil
}

func decodePaymentRequired(header http.Header, body []byte) (*PaymentRequired, error) {
	if value := header.Get(HeaderPaymentRequired); value != "" {
		if int64(len(value)) > defaultMaxControlBodyBytes {
			return nil, fmt.Errorf("%s header exceeds %d bytes", HeaderPaymentRequired, defaultMaxControlBodyBytes)
		}
		return DecodePaymentRequiredHeader(value)
	}

	// Compatibility fallback for servers that put a v2 challenge in the body.
	if len(body) > 0 {
		var required PaymentRequired
		if err := json.Unmarshal(body, &required); err == nil && required.X402Version == Version2 {
			return &required, nil
		}
	}
	return nil, fmt.Errorf("HTTP 402 response did not contain a valid %s header", HeaderPaymentRequired)
}

func validatePaymentPayload(payload *PaymentPayload, accepted *PaymentRequirements) error {
	if payload == nil {
		return fmt.Errorf("payment builder returned no payload")
	}
	if payload.X402Version != Version2 {
		return fmt.Errorf("payment payload has x402Version %d, expected %d", payload.X402Version, Version2)
	}
	if payload.Accepted.Scheme != accepted.Scheme || payload.Accepted.Network != accepted.Network ||
		payload.Accepted.Asset != accepted.Asset || payload.Accepted.Amount != accepted.Amount ||
		payload.Accepted.PayTo != accepted.PayTo {
		return fmt.Errorf("payment payload does not match the accepted payment requirement")
	}
	transaction, _ := payload.Payload["transaction"].(string)
	if transaction == "" {
		return fmt.Errorf("Stellar payment payload is missing payload.transaction")
	}
	return nil
}

func responseResult(resp *http.Response, body []byte) *Result {
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return &Result{
		StatusCode: resp.StatusCode,
		FinalURL:   finalURL,
		Headers:    resp.Header.Clone(),
		Body:       body,
		BodyText:   string(body),
	}
}
