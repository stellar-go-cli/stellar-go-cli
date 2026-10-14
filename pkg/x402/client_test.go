package x402

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testPaymentBuilder struct {
	payload     *PaymentPayload
	err         error
	calls       atomic.Int32
	gotAccepted *PaymentRequirements
}

func (b *testPaymentBuilder) CreatePaymentPayload(_ context.Context, required *PaymentRequired, accepted *PaymentRequirements) (*PaymentPayload, error) {
	b.calls.Add(1)
	b.gotAccepted = accepted
	if b.err != nil {
		return nil, b.err
	}
	if b.payload != nil {
		return b.payload, nil
	}
	return &PaymentPayload{
		X402Version: Version2,
		Payload:     map[string]interface{}{"transaction": "AAAA"},
		Accepted:    *accepted,
		Resource:    required.Resource,
		Extensions:  required.Extensions,
	}, nil
}

func paymentRequiredHeader(t *testing.T, required PaymentRequired) string {
	t.Helper()
	data, err := json.Marshal(required)
	if err != nil {
		t.Fatalf("marshal payment required: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func settlementHeader(t *testing.T, response SettlementResponse) string {
	t.Helper()
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal settlement: %v", err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestClientFetchPaidRetry(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Resource:    &ResourceInfo{URL: "https://example.test/resource"},
		Accepts: []PaymentRequirements{
			validRequirement(t, NetworkStellarTestnet),
		},
	}
	settlement := SettlementResponse{
		Success:     true,
		Transaction: "abc123",
		Network:     NetworkStellarTestnet,
		Payer:       "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
	}

	var requests atomic.Int32
	var sawSignature bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(body) != "request-body" {
			t.Fatalf("unexpected request body %q", body)
		}
		if r.Header.Get("X-Test") != "yes" {
			t.Fatalf("missing request header")
		}
		if requests.Load() == 1 {
			if r.Header.Get(HeaderPaymentSignature) != "" {
				t.Fatal("unexpected payment signature on initial request")
			}
			w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte("payment required"))
			return
		}
		signature := r.Header.Get(HeaderPaymentSignature)
		if signature == "" {
			t.Fatal("missing PAYMENT-SIGNATURE retry header")
		}
		sawSignature = true
		decoded, err := base64.StdEncoding.DecodeString(signature)
		if err != nil {
			t.Fatalf("decode signature header: %v", err)
		}
		var payload PaymentPayload
		if err := json.Unmarshal(decoded, &payload); err != nil {
			t.Fatalf("unmarshal signature payload: %v", err)
		}
		if payload.X402Version != Version2 || payload.Payload["transaction"] != "AAAA" {
			t.Fatalf("unexpected signature payload: %+v", payload)
		}
		w.Header().Set(HeaderPaymentResponse, settlementHeader(t, settlement))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	required.Resource.URL = server.URL + "/"

	builder := &testPaymentBuilder{}
	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: builder,
		Network:        "stellar-testnet",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Fetch(context.Background(), Request{
		Method: http.MethodPost,
		URL:    server.URL,
		Headers: map[string]string{
			"X-Test":              "yes",
			"payment-signature":   "forged",
			HeaderPaymentRequired: "forged",
			HeaderPaymentResponse: "forged",
		},
		Body: []byte("request-body"),
	}, FetchOptions{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if requests.Load() != 2 || !sawSignature {
		t.Fatalf("expected challenge and paid retry, got %d requests", requests.Load())
	}
	if result.StatusCode != http.StatusOK || result.BodyText != `{"ok":true}` {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Settlement == nil || !result.Settlement.Success || result.Settlement.Transaction != "abc123" {
		t.Fatalf("missing settlement: %+v", result.Settlement)
	}
	if builder.calls.Load() != 1 || builder.gotAccepted == nil {
		t.Fatalf("payment builder not called as expected")
	}
}

func TestClientFetchDryRunDoesNotRetry(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Accepts:     []PaymentRequirements{validRequirement(t, NetworkStellarPubnet)},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: &testPaymentBuilder{},
		Network:        "mainnet",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("dry run sent retry request")
	}
	if !result.DryRun || result.PaymentPayload == nil || result.PaymentAttempted {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
	if result.PaymentPayload.Resource == nil || result.PaymentPayload.Resource.URL != server.URL {
		t.Fatal("expected request URL as fallback resource info")
	}
}

func TestClientFetchRejectsRetry402(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Accepts:     []PaymentRequirements{validRequirement(t, NetworkStellarTestnet)},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: &testPaymentBuilder{},
		Network:        NetworkStellarTestnet,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected payment rejection, got result=%+v err=%v", result, err)
	}
	if requests.Load() != 2 || !result.PaymentAttempted {
		t.Fatalf("expected paid retry attempt, got %d requests", requests.Load())
	}
}

func TestClientFetchNoChallengeDoesNotBuildPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("free"))
	}))
	defer server.Close()

	builder := &testPaymentBuilder{err: fmt.Errorf("builder must not be called")}
	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: builder,
		Network:        NetworkStellarTestnet,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	result, err := client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if result.StatusCode != http.StatusOK || result.PaymentAttempted || builder.calls.Load() != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestClientFetchRejectsMismatchedChallengeResource(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Resource:    &ResourceInfo{URL: "https://example.test/other"},
		Accepts:     []PaymentRequirements{validRequirement(t, NetworkStellarTestnet)},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	builder := &testPaymentBuilder{}
	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: builder,
		Network:        NetworkStellarTestnet,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "does not match requested resource") {
		t.Fatalf("expected resource mismatch error, got %v", err)
	}
	if requests.Load() != 1 || builder.calls.Load() != 0 {
		t.Fatalf("payment builder should not be called for a mismatched resource")
	}
}

func TestClientFetchRejectsUnsupportedChallenge(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Accepts: []PaymentRequirements{
			{Scheme: SchemeExact, Network: "eip155:8453", Asset: "0xabc", Amount: "1", PayTo: "0xdef", MaxTimeoutSeconds: 60},
		},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
		w.WriteHeader(http.StatusPaymentRequired)
	}))
	defer server.Close()

	builder := &testPaymentBuilder{}
	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: builder,
		Network:        NetworkStellarTestnet,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "no usable Stellar exact") {
		t.Fatalf("expected unsupported challenge error, got %v", err)
	}
	if requests.Load() != 1 || builder.calls.Load() != 0 {
		t.Fatalf("payment builder should not be called")
	}
}

func TestClientFetchSettlementFailure(t *testing.T) {
	required := PaymentRequired{
		X402Version: Version2,
		Accepts:     []PaymentRequirements{validRequirement(t, NetworkStellarTestnet)},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if requests.Load() == 1 {
			w.Header().Set(HeaderPaymentRequired, paymentRequiredHeader(t, required))
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		w.Header().Set(HeaderPaymentResponse, settlementHeader(t, SettlementResponse{
			Success:      false,
			ErrorReason:  "invalid_signature",
			ErrorMessage: "bad payer signature",
		}))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		HTTPClient:     server.Client(),
		PaymentBuilder: &testPaymentBuilder{},
		Network:        NetworkStellarTestnet,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Fetch(context.Background(), Request{URL: server.URL}, FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "invalid_signature") {
		t.Fatalf("expected settlement failure, got %v", err)
	}
}
