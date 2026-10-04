package swap

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/txnbuild"
)

// twoPathPaymentBaseFeesXLM is two path-payment txs at MinBaseFee (one op each).
const twoPathPaymentBaseFeesXLM = 2 * float64(txnbuild.MinBaseFee) / 1e7

// AssetConfig maps asset codes to their issuer addresses for testnet/mainnet
type AssetConfig struct {
	Code   string
	Issuer string
}

// Testnet assets
var TestnetAssets = map[string]AssetConfig{
	"XLM":  {Code: "XLM", Issuer: ""},
	"USDC": {Code: "USDC", Issuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"},
	"EURC": {Code: "EURC", Issuer: "GAKMOVSF35IPK5HTDN4B3ITIR5R4AX6PZFAXNPJFDHNIUQKDT5O6G2E"},
	"XRF":  {Code: "XRF", Issuer: "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF"},
}

// Mainnet assets (verified working on Stellar mainnet)
var MainnetAssets = map[string]AssetConfig{
	"XLM":  {Code: "XLM", Issuer: ""},
	"USDC": {Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"},
	"EURC": {Code: "EURC", Issuer: "GDUKMGUGDZQK6YHYA5Z6AY2G4XDSZDW2WER5GZ5GUESDSEZNCNDJID9"},
	"yXLM": {Code: "yXLM", Issuer: "GARDNV3Q7YGT4AKSDF25LT32YSCCW4EV22Y2TV3I2PU2MMXJTEDL5T55"},
	"XRF":  {Code: "XRF", Issuer: "GCHI6I3X62ND5XUMWINNNKXS2HPYZWKFQBZZYBSMHJ4MIP2XJXSZTXRF"},
}

type Service struct {
	client     *horizonclient.Client
	passphrase string
	assets     map[string]AssetConfig
	baseFee    int64
	signer     Signer
}

// Option configures a Service.
type Option func(*Service)

// WithClient overrides the Horizon client (e.g. a self-hosted Horizon).
func WithClient(c *horizonclient.Client) Option {
	return func(s *Service) {
		if c != nil {
			s.client = c
		}
	}
}

// WithPassphrase overrides the network passphrase (required when WithClient
// points at a non-default network).
func WithPassphrase(p string) Option {
	return func(s *Service) {
		if p != "" {
			s.passphrase = p
		}
	}
}

// WithAssets merges additional assets into the service's asset registry.
func WithAssets(assets map[string]AssetConfig) Option {
	return func(s *Service) {
		for k, v := range assets {
			s.assets[k] = v
		}
	}
}

// WithSigner sets the signer used by Execute* methods. Quote-only and
// BuildSwapTransaction flows do not need a signer.
func WithSigner(signer Signer) Option {
	return func(s *Service) { s.signer = signer }
}

// WithKeypair is sugar for WithSigner(KeypairSigner{kp}).
func WithKeypair(kp *keypair.Full) Option {
	return func(s *Service) {
		if kp != nil {
			s.signer = KeypairSigner{KP: kp}
		}
	}
}

// WithBaseFee overrides the per-operation fee (stroops) used for submitted
// transactions.
func WithBaseFee(stroops int64) Option {
	return func(s *Service) { s.SetBaseFee(stroops) }
}

// SetBaseFee overrides the per-operation fee (stroops) used for submitted transactions.
func (s *Service) SetBaseFee(stroops int64) {
	if stroops >= txnbuild.MinBaseFee {
		s.baseFee = stroops
	}
}

func (s *Service) feeStroops() int64 {
	if s.baseFee > 0 {
		return s.baseFee
	}
	return txnbuild.MinBaseFee
}

// Signer returns the configured signer, or nil when the service is quote-only.
func (s *Service) Signer() Signer { return s.signer }

// Network returns the network this service targets.
func (s *Service) Network() models.Network {
	if s.passphrase == network.PublicNetworkPassphrase {
		return models.NetworkStellarMainnet
	}
	return models.NetworkStellarTestnet
}

// NetworkPassphrase returns the Stellar network passphrase.
func (s *Service) NetworkPassphrase() string { return s.passphrase }

// NewService creates a swap service for the given network. Options inject a
// custom Horizon client, passphrase, asset registry, base fee, or signer —
// services without a signer can quote and build unsigned transactions but
// cannot execute.
func NewService(net models.Network, opts ...Option) *Service {
	var client *horizonclient.Client
	var passphrase string
	var registry map[string]AssetConfig

	switch net {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		passphrase = network.PublicNetworkPassphrase
		registry = MainnetAssets
	default:
		client = horizonclient.DefaultTestNetClient
		passphrase = network.TestNetworkPassphrase
		registry = TestnetAssets
	}

	// Copy the registry so WithAssets never mutates the package-level maps.
	assets := make(map[string]AssetConfig, len(registry))
	for k, v := range registry {
		assets[k] = v
	}

	s := &Service{
		client:     client,
		passphrase: passphrase,
		assets:     assets,
		baseFee:    txnbuild.MinBaseFee,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// GetQuote fetches path payment quotes from Horizon
func (s *Service) GetQuote(req models.SwapRequest) (*models.SwapQuote, error) {
	if req.SwapType == "" {
		req.SwapType = models.SwapStrictSend
	}

	// Validate assets before querying Horizon to avoid 400s on bad codes
	if err := s.validateAsset(req.SourceAsset); err != nil {
		return nil, err
	}
	if err := s.validateAsset(req.DestAsset); err != nil {
		return nil, err
	}

	// Get paths from Horizon
	paths, err := s.findPaths(req)
	if err != nil {
		return nil, fmt.Errorf("failed to find paths: %w", err)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("no paths found for %s -> %s", req.SourceAsset, req.DestAsset)
	}

	// Use the best path (first one with best rate)
	bestPath := paths[0]

	// Calculate price impact (simplified)
	priceImpact := s.calculatePriceImpact(req, bestPath)

	now := time.Now().UTC()
	quote := &models.SwapQuote{
		QuoteID:        mpCrypto.RandomHex(12),
		SourceAsset:    req.SourceAsset,
		DestAsset:      req.DestAsset,
		SwapType:       req.SwapType,
		Amount:         req.Amount,
		ExpectedAmount: bestPath.DestAmount,
		PriceImpact:    priceImpact,
		NetworkFee:     "0.00001 XLM",
		Paths:          paths,
		ValidUntil:     now.Add(30 * time.Second),
		CreatedAt:      now,
	}

	return quote, nil
}

// sourceAccount resolves the account used as tx source / Horizon
// source_account: the explicit request value, else the signer's address.
func (s *Service) sourceAccount(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if s.signer != nil {
		return s.signer.Address(), nil
	}
	return "", fmt.Errorf("no source account: set SwapRequest.SourceAccount or configure a signer with WithSigner")
}

// findPaths queries Horizon for available swap paths. Strict-send quotes need
// no account; strict-receive quotes resolve SourceAccount from the request or
// the configured signer.
func (s *Service) findPaths(req models.SwapRequest) ([]models.SwapPath, error) {
	sourceAsset, err := s.resolveAsset(req.SourceAsset)
	if err != nil {
		return nil, err
	}
	destAsset, err := s.resolveAsset(req.DestAsset)
	if err != nil {
		return nil, err
	}

	var pathsPage horizon.PathsPage

	switch req.SwapType {
	case models.SwapStrictSend:
		// Use destination_assets for the target asset only. Passing destination_account
		// makes Horizon expand all trusted assets; accounts with many trustlines hit a
		// 15-asset limit (400). Horizon does not allow destination_account and
		// destination_assets together.
		hzReq := horizonclient.StrictSendPathsRequest{
			SourceAssetType:   s.getAssetType(sourceAsset),
			SourceAssetCode:   s.getAssetCode(sourceAsset),
			SourceAssetIssuer: s.getAssetIssuer(sourceAsset),
			SourceAmount:      req.Amount,
			DestinationAssets: s.destinationAssetsQueryParam(destAsset),
		}
		pathsPage, err = s.client.StrictSendPaths(hzReq)
		if err != nil {
			return nil, fmt.Errorf("strict send paths: %w", err)
		}
	case models.SwapStrictReceive:
		src, err := s.sourceAccount(req.SourceAccount)
		if err != nil {
			return nil, err
		}
		dst := req.Destination
		if dst == "" {
			dst = src
		}
		// For strict receive, find paths that can deliver the dest amount
		pathsReq := horizonclient.PathsRequest{
			SourceAccount:          src,
			SourceAssets:           s.getAssetString(sourceAsset),
			DestinationAccount:     dst,
			DestinationAssetType:   s.getAssetType(destAsset),
			DestinationAssetCode:   s.getAssetCode(destAsset),
			DestinationAssetIssuer: s.getAssetIssuer(destAsset),
			DestinationAmount:      req.Amount,
		}
		pathsPage, err = s.client.Paths(pathsReq)
		if err != nil {
			return nil, fmt.Errorf("strict receive paths: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported swap type: %s", req.SwapType)
	}

	var swapPaths []models.SwapPath
	for _, p := range pathsPage.Embedded.Records {
		pathAssets := make([]models.PathAsset, len(p.Path))
		skipPath := false
		for i, a := range p.Path {
			// Skip paths with assets that have no issuer - causes op_malformed
			if a.Code != "XLM" && a.Issuer == "" {
				skipPath = true
				break
			}
			pathAssets[i] = models.PathAsset{
				Code:   a.Code,
				Issuer: a.Issuer,
			}
		}
		if skipPath {
			continue
		}
		swapPaths = append(swapPaths, models.SwapPath{
			Path:         pathAssets,
			SourceAmount: p.SourceAmount,
			DestAmount:   p.DestinationAmount,
			Price:        s.calculatePrice(p.SourceAmount, p.DestinationAmount),
		})
	}

	sortPathsBySwapType(req.SwapType, swapPaths)
	return swapPaths, nil
}

func sortPathsBySwapType(swapType models.SwapType, paths []models.SwapPath) {
	switch swapType {
	case models.SwapStrictSend:
		sort.Slice(paths, func(i, j int) bool {
			di, err := strconv.ParseFloat(paths[i].DestAmount, 64)
			if err != nil {
				return false
			}
			dj, err := strconv.ParseFloat(paths[j].DestAmount, 64)
			if err != nil {
				return true
			}
			return di > dj
		})
	case models.SwapStrictReceive:
		sort.Slice(paths, func(i, j int) bool {
			si, err := strconv.ParseFloat(paths[i].SourceAmount, 64)
			if err != nil {
				return false
			}
			sj, err := strconv.ParseFloat(paths[j].SourceAmount, 64)
			if err != nil {
				return true
			}
			return si < sj
		})
	}
}

// buildSwapOperation builds the path payment operation for a quote, using the
// best (first) path and dynamic slippage based on path complexity.
func (s *Service) buildSwapOperation(quote *models.SwapQuote, maxSlippage float64, destination string) (txnbuild.Operation, error) {
	sourceAsset, err := s.resolveAsset(quote.SourceAsset)
	if err != nil {
		return nil, err
	}
	destAsset, err := s.resolveAsset(quote.DestAsset)
	if err != nil {
		return nil, err
	}
	if len(quote.Paths) == 0 {
		return nil, fmt.Errorf("no paths in quote")
	}
	bestPath := quote.Paths[0]
	builtPath, err := s.buildPath(bestPath.Path)
	if err != nil {
		return nil, err
	}

	switch quote.SwapType {
	case models.SwapStrictSend:
		dynamicSlippage := s.calculateDynamicSlippage(maxSlippage/100.0, bestPath.Path)
		destMin := s.applySlippage(bestPath.DestAmount, dynamicSlippage, false)
		return &txnbuild.PathPaymentStrictSend{
			SendAsset:   sourceAsset,
			SendAmount:  quote.Amount,
			DestAsset:   destAsset,
			DestMin:     destMin,
			Destination: destination,
			Path:        builtPath,
		}, nil
	case models.SwapStrictReceive:
		dynamicSlippage := s.calculateDynamicSlippage(maxSlippage/100.0, bestPath.Path)
		sendMax := s.applySlippage(bestPath.SourceAmount, dynamicSlippage, true)
		return &txnbuild.PathPaymentStrictReceive{
			SendAsset:   sourceAsset,
			SendMax:     sendMax,
			DestAsset:   destAsset,
			DestAmount:  quote.Amount,
			Destination: destination,
			Path:        builtPath,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported swap type: %s", quote.SwapType)
	}
}

// FetchAccount loads an account from Horizon for use as a transaction source.
func (s *Service) FetchAccount(address string) (txnbuild.Account, error) {
	acct, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: address})
	if err != nil {
		return nil, fmt.Errorf("horizon: account not found: %w", err)
	}
	return &acct, nil
}

// BuildSwapTransaction builds an unsigned path-payment transaction for a
// quote. The caller supplies the source account (e.g. from FetchAccount or
// txnbuild.SimpleAccount) and is responsible for signing and submission —
// enabling non-custodial flows where a backend returns XDR for a wallet to
// sign. No signer is required.
func (s *Service) BuildSwapTransaction(quote *models.SwapQuote, source txnbuild.Account, maxSlippage float64, destination string) (*txnbuild.Transaction, error) {
	op, err := s.buildSwapOperation(quote, maxSlippage, destination)
	if err != nil {
		return nil, err
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        source,
		IncrementSequenceNum: true,
		BaseFee:              s.feeStroops(),
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}
	return tx, nil
}

// BuildSwapTransactionXDR builds an unsigned swap transaction and returns it
// as base64-encoded XDR for a wallet to sign.
func (s *Service) BuildSwapTransactionXDR(quote *models.SwapQuote, source txnbuild.Account, maxSlippage float64, destination string) (string, error) {
	tx, err := s.BuildSwapTransaction(quote, source, maxSlippage, destination)
	if err != nil {
		return "", err
	}
	txB64, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("serialize transaction: %w", err)
	}
	return txB64, nil
}

// SubmitTransaction submits a signed transaction to Horizon, unwrapping
// failure result codes into descriptive errors.
func (s *Service) SubmitTransaction(tx *txnbuild.Transaction) (*horizon.Transaction, error) {
	txB64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("serialize transaction: %w", err)
	}

	resp, err := s.client.SubmitTransactionXDR(txB64)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			rc, rcErr := herr.ResultCodes()
			if rcErr == nil && rc != nil {
				for _, opCode := range rc.OperationCodes {
					if opCode == "op_under_dest_min" {
						return nil, fmt.Errorf("tx failed — insufficient destination amount received (op_under_dest_min). Market conditions changed between quote and execution — try increasing slippage")
					}
				}
				return nil, fmt.Errorf("tx failed — code: %s, ops: %v", rc.TransactionCode, rc.OperationCodes)
			}
			return nil, fmt.Errorf("horizon error: %s", herr.Problem.Title)
		}
		return nil, fmt.Errorf("submit to Horizon: %w", err)
	}
	return &resp, nil
}

// ExecuteSwap builds, signs, and submits a path payment transaction. Requires
// a signer (see WithSigner); for non-custodial flows use
// BuildSwapTransaction instead.
func (s *Service) ExecuteSwap(quote *models.SwapQuote, maxSlippage float64, destination string) (*models.Payment, error) {
	if s.signer == nil {
		return nil, fmt.Errorf("no signer configured: pass WithSigner to NewService")
	}
	src := s.signer.Address()

	// Use sender as destination if not specified
	if destination == "" {
		destination = src
	}

	// Fetch source account
	sourceAcct, err := s.FetchAccount(src)
	if err != nil {
		return nil, err
	}

	// Check trustline for destination asset
	if err := s.ensureTrustline(src, quote.DestAsset); err != nil {
		return nil, fmt.Errorf("trustline check failed: %w", err)
	}

	tx, err := s.BuildSwapTransaction(quote, sourceAcct, maxSlippage, destination)
	if err != nil {
		return nil, err
	}

	// Sign
	tx, err = s.signer.Sign(tx, s.passphrase)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}

	// Submit
	resp, err := s.SubmitTransaction(tx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	confirmed := now.Add(5 * time.Second)

	return &models.Payment{
		ID:          "swap-" + resp.Hash[:12],
		From:        src,
		To:          destination,
		Amount:      quote.Amount,
		Asset:       quote.SourceAsset,
		Rail:        models.RailSwap,
		Status:      models.PaymentConfirmed,
		Network:     s.Network(),
		Memo:        fmt.Sprintf("Swap %s→%s", quote.SourceAsset, quote.DestAsset),
		FXRate:      fmt.Sprintf("%.6f", quote.Paths[0].Price),
		Fee:         quote.NetworkFee,
		TxHash:      resp.Hash,
		LedgerSeq:   int64(resp.Ledger),
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}, nil
}

// Helper methods

func (s *Service) getAssetString(asset txnbuild.Asset) string {
	if asset.IsNative() {
		return "native"
	}
	ca := asset.(txnbuild.CreditAsset)
	return ca.Code + ":" + ca.Issuer
}

// destinationAssetsQueryParam formats the destination asset for Horizon paths/strict-send
// (?destination_assets=). Use "native" or "CODE:ISSUER".
func (s *Service) destinationAssetsQueryParam(dest txnbuild.Asset) string {
	if dest.IsNative() {
		return "native"
	}
	ca := dest.(txnbuild.CreditAsset)
	return ca.Code + ":" + ca.Issuer
}

func (s *Service) getAssetType(asset txnbuild.Asset) horizonclient.AssetType {
	if asset.IsNative() {
		return "native"
	}
	return "credit_alphanum4"
}

func (s *Service) getAssetCode(asset txnbuild.Asset) string {
	if asset.IsNative() {
		return ""
	}
	ca := asset.(txnbuild.CreditAsset)
	return ca.Code
}

func (s *Service) getAssetIssuer(asset txnbuild.Asset) string {
	if asset.IsNative() {
		return ""
	}
	ca := asset.(txnbuild.CreditAsset)
	return ca.Issuer
}

// resolveAsset converts an asset spec to a txnbuild.Asset:
//
//	"XLM" | "native" | ""  → native asset
//	"CODE:ISSUER"           → credit asset (issuer must be a valid ed25519 address)
//	"CODE"                  → looked up in the service's asset registry
func (s *Service) resolveAsset(spec string) (txnbuild.Asset, error) {
	spec = strings.TrimSpace(spec)
	upper := strings.ToUpper(spec)
	if upper == "" || upper == "XLM" || upper == "NATIVE" {
		return txnbuild.NativeAsset{}, nil
	}
	if i := strings.Index(spec, ":"); i > 0 {
		code := strings.ToUpper(strings.TrimSpace(spec[:i]))
		issuer := strings.TrimSpace(spec[i+1:])
		if len(code) == 0 || len(code) > 12 {
			return nil, fmt.Errorf("invalid asset code in %q", spec)
		}
		if !strkey.IsValidEd25519PublicKey(issuer) {
			return nil, fmt.Errorf("invalid issuer address in %q", spec)
		}
		return txnbuild.CreditAsset{Code: code, Issuer: issuer}, nil
	}
	cfg, ok := s.assets[upper]
	if !ok {
		return nil, fmt.Errorf("unsupported asset %q (supported: %s)", spec, strings.Join(s.supportedCodes(), ", "))
	}
	return txnbuild.CreditAsset{Code: cfg.Code, Issuer: cfg.Issuer}, nil
}

// validateAsset returns a clear error for unresolvable asset specs.
func (s *Service) validateAsset(spec string) error {
	_, err := s.resolveAsset(spec)
	return err
}

// supportedCodes returns the sorted list of known asset codes for this network
func (s *Service) supportedCodes() []string {
	codes := make([]string, 0, len(s.assets))
	for code := range s.assets {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func (s *Service) buildPath(pathAssets []models.PathAsset) ([]txnbuild.Asset, error) {
	var path []txnbuild.Asset
	for _, pa := range pathAssets {
		spec := pa.Code
		if pa.Issuer != "" {
			// Use the issuer from the Horizon path
			spec = pa.Code + ":" + pa.Issuer
		}
		asset, err := s.resolveAsset(spec)
		if err != nil {
			return nil, fmt.Errorf("path asset %q: %w", pa.Code, err)
		}
		path = append(path, asset)
	}
	return path, nil
}

// atof parses s as a float64, returning 0 when s is not a number.
func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64) //nolint:errcheck // 0 is the intended fallback
	return v
}

func (s *Service) calculatePrice(source, dest string) float64 {
	sourceAmt := atof(source)
	destAmt := atof(dest)
	if destAmt == 0 {
		return 0
	}
	return destAmt / sourceAmt
}

func (s *Service) calculatePriceImpact(req models.SwapRequest, path models.SwapPath) float64 {
	// Simplified price impact calculation
	// In production, this would compare against a reference price
	return 0.1 // Default 0.1%
}

func (s *Service) applySlippage(amount string, slippage float64, isMax bool) string {
	amt := atof(amount)
	// slippage is passed as decimal (e.g., 0.01 = 1%)
	multiplier := 1.0 + slippage
	if !isMax {
		// For minimum receive, reduce amount
		multiplier = 1.0 - slippage
	}

	// Prevent negative amounts
	if multiplier < 0 {
		multiplier = 0
	}

	result := amt * multiplier
	return fmt.Sprintf("%.7f", result)
}

func (s *Service) calculateDynamicSlippage(baseSlippage float64, pathAssets []models.PathAsset) float64 {
	// Add 0.5% extra slippage per intermediate asset
	hopCount := len(pathAssets)
	if hopCount == 0 {
		// Direct path - no adjustment needed
		return baseSlippage
	}

	// Add 0.5% per hop (capped at +3% total)
	extraSlippage := float64(hopCount) * 0.005
	if extraSlippage > 0.03 { // 3% = 0.03 as decimal
		extraSlippage = 0.03
	}

	adjusted := baseSlippage + extraSlippage
	return adjusted
}

func (s *Service) ensureTrustline(address, assetSpec string) error {
	asset, err := s.resolveAsset(assetSpec)
	if err != nil {
		return err
	}
	if asset.IsNative() {
		return nil
	}

	// In production, we would check account balances and add trustline if needed
	// For now, assume trustline exists or user has pre-established it
	return nil
}

// AnalyzeRoundTrip quotes XLM→USDC then USDC→XLM (strict send) using Horizon best paths.
func twoTxFeeReserveXLMString() string {
	return fmt.Sprintf("%.5f", twoPathPaymentBaseFeesXLM)
}

// parseAssetBalance parses the balance of a specific asset code from an account
func parseAssetBalance(acc horizon.Account, assetCode, issuer string) float64 {
	for _, b := range acc.Balances {
		if b.Type == "native" {
			if assetCode == "XLM" {
				v := atof(b.Balance)
				return v
			}
			continue
		}
		if b.Code == assetCode && (issuer == "" || b.Issuer == issuer) {
			v := atof(b.Balance)
			return v
		}
	}
	return 0
}

// getAccountSnapshot captures a complete snapshot of all account balances
func getAccountSnapshot(acc horizon.Account) *models.AccountSnapshot {
	snapshot := &models.AccountSnapshot{
		Timestamp: time.Now().UTC(),
		Assets:    []models.AssetBalance{},
	}

	for _, b := range acc.Balances {
		if b.Type == "native" {
			snapshot.XLM = b.Balance
		} else {
			snapshot.Assets = append(snapshot.Assets, models.AssetBalance{
				Code:    b.Code,
				Issuer:  b.Issuer,
				Balance: b.Balance,
			})
		}
	}

	return snapshot
}

// AnalyzeRoundTrip runs paper round-trip quotes (not on-chain). Supports any
// resolvable asset pair as base/counter (e.g., XLM/USDC, XRF/USDC, XRF/XLM).
// No signer is required — quotes are strict-send.
func (s *Service) AnalyzeRoundTrip(amount, baseAsset, counterAsset, destination string) (*models.SwapRoundTripResult, error) {
	dst := destination
	if dst == "" && s.signer != nil {
		dst = s.signer.Address()
	}

	// Validate base and counter assets
	if _, err := s.resolveAsset(baseAsset); err != nil {
		return nil, fmt.Errorf("unsupported base asset: %w", err)
	}
	if _, err := s.resolveAsset(counterAsset); err != nil {
		return nil, fmt.Errorf("unsupported counter asset: %w", err)
	}

	// Build leg A: baseAsset → counterAsset
	reqA := models.SwapRequest{
		SourceAsset: baseAsset,
		DestAsset:   counterAsset,
		Amount:      amount,
		SwapType:    models.SwapStrictSend,
		Destination: dst,
	}

	quoteA, err := s.GetQuote(reqA)
	if err != nil {
		return nil, fmt.Errorf("leg A (%s→%s): %w", baseAsset, counterAsset, err)
	}

	// Build leg B: counterAsset → baseAsset
	reqB := models.SwapRequest{
		SourceAsset: counterAsset,
		DestAsset:   baseAsset,
		Amount:      quoteA.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
		Destination: dst,
	}

	quoteB, err := s.GetQuote(reqB)
	if err != nil {
		return nil, fmt.Errorf("leg B (%s→%s): %w", counterAsset, baseAsset, err)
	}

	// Calculate profit in base asset terms
	baseIn := atof(amount)
	baseOut := atof(quoteB.ExpectedAmount)
	net := baseOut - baseIn - twoPathPaymentBaseFeesXLM

	// Set result fields
	result := &models.SwapRoundTripResult{
		AmountXLM:       amount,
		BaseAsset:       baseAsset,
		CounterAsset:    counterAsset,
		LegA:            quoteA,
		LegB:            quoteB,
		FeeReserveXLM:   twoTxFeeReserveXLMString(),
		EstimatedNetXLM: net,
	}

	// Store intermediate and returned amounts based on which asset is native/XLM
	if baseAsset == "XLM" {
		result.USDCIntermediate = quoteA.ExpectedAmount
		result.XLMReturned = quoteB.ExpectedAmount
	} else if counterAsset == "XLM" {
		result.XLMReturned = quoteA.ExpectedAmount
		result.USDCIntermediate = quoteB.ExpectedAmount
	} else {
		// Neither is XLM - use generic field names
		result.CounterIntermediate = quoteA.ExpectedAmount
		result.BaseReturned = quoteB.ExpectedAmount
	}

	return result, nil
}

// ExecuteRoundTrip submits leg A then leg B. Handles any asset pair arbitrage.
func (s *Service) ExecuteRoundTrip(amount, slippage float64, destination string, minProfitXLM float64, baseAsset, counterAsset string) ([]*models.Payment, error) {
	res, err := s.AnalyzeRoundTrip(fmt.Sprintf("%.7f", amount), baseAsset, counterAsset, destination)
	if err != nil {
		return nil, err
	}
	if res.EstimatedNetXLM < minProfitXLM {
		return nil, fmt.Errorf("estimated net XLM %.7f is below minimum %.7f (won't execute)", res.EstimatedNetXLM, minProfitXLM)
	}
	if s.signer == nil {
		return nil, fmt.Errorf("no signer configured: pass WithSigner to NewService")
	}
	src := s.signer.Address()
	dst := destination
	if dst == "" {
		dst = src
	}

	acctBefore, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: src})
	if err != nil {
		return nil, fmt.Errorf("horizon account before leg A: %w", err)
	}

	// Capture full account snapshot before execution
	res.SnapshotBefore = getAccountSnapshot(acctBefore)

	// Validate quotes are still fresh before executing
	now := time.Now()
	if res.LegA != nil && now.Sub(res.LegA.CreatedAt).Seconds() > 15 {
		return nil, fmt.Errorf("leg A quote is too old (%.1fs old, max 15s)", now.Sub(res.LegA.CreatedAt).Seconds())
	}
	if res.LegB != nil && now.Sub(res.LegB.CreatedAt).Seconds() > 15 {
		return nil, fmt.Errorf("leg B quote is too old (%.1fs old, max 15s)", now.Sub(res.LegB.CreatedAt).Seconds())
	}

	var pay1, pay2 *models.Payment
	var amountB string

	// Get issuer for counter asset
	var counterIssuer string
	if a, err := s.resolveAsset(counterAsset); err == nil && !a.IsNative() {
		counterIssuer = a.(txnbuild.CreditAsset).Issuer
	}

	// Track counter asset balance before leg A
	counterBefore := parseAssetBalance(acctBefore, counterAsset, counterIssuer)

	pay1, err = s.ExecuteSwap(res.LegA, slippage, dst)
	if err != nil {
		// Return payments made so far with the before snapshot
		return []*models.Payment{pay1}, fmt.Errorf("leg A execute: %w", err)
	}

	acctAfterLegA, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: src})
	if err != nil {
		// Still return payment and before snapshot
		return []*models.Payment{pay1}, fmt.Errorf("horizon account after leg A: %w", err)
	}

	// Capture snapshot after leg A - this shows any intermediate assets
	res.SnapshotAfterLegA = getAccountSnapshot(acctAfterLegA)

	counterAfter := parseAssetBalance(acctAfterLegA, counterAsset, counterIssuer)
	delta := counterAfter - counterBefore
	if delta <= 0 {
		// Return payment and snapshots so far for analysis
		return []*models.Payment{pay1}, fmt.Errorf("no %s received from leg A (delta %.7f)", counterAsset, delta)
	}
	amountB = fmt.Sprintf("%.7f", delta)

	// Build leg B request: counterAsset -> baseAsset
	reqB := models.SwapRequest{
		SourceAsset: counterAsset,
		DestAsset:   baseAsset,
		Amount:      amountB,
		SwapType:    models.SwapStrictSend,
		Destination: dst,
	}

	quoteB, err := s.GetQuote(reqB)
	if err != nil {
		// Return payment and all snapshots for debugging
		return []*models.Payment{pay1}, fmt.Errorf("leg B quote after fill: %w", err)
	}
	pay2, err = s.ExecuteSwap(quoteB, slippage, dst)
	if err != nil {
		// Return both payments and all snapshots for analysis
		return []*models.Payment{pay1, pay2}, fmt.Errorf("leg B execute: %w", err)
	}

	// Capture final account snapshot after both legs complete
	acctAfter, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: src})
	if err != nil {
		// Return payments but can't capture final snapshot
		return []*models.Payment{pay1, pay2}, fmt.Errorf("horizon account after leg B: %w", err)
	}
	res.SnapshotAfter = getAccountSnapshot(acctAfter)

	return []*models.Payment{pay1, pay2}, nil
}

// atomicTxTimeout bounds how long an atomic round-trip tx stays valid for inclusion.
const atomicTxTimeout = 30 * time.Second

// awaitTransaction polls Horizon for a submitted tx hash until it appears or the wait expires.
func (s *Service) awaitTransaction(hash string, wait time.Duration) (*horizon.Transaction, error) {
	deadline := time.Now().Add(wait)
	for {
		tx, err := s.client.TransactionDetail(hash)
		if err == nil {
			return &tx, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(3 * time.Second)
	}
}

// ExecuteRoundTripAtomic submits the round trip as ONE path payment base→…→counter→…→base
// to the sender's own account. DestMin is set to amount + fee + minProfitXLM, so the ledger
// either fills the whole cycle at a profit or fails the op (op_under_dest_min) — no stranded
// inventory and a single base fee.
func (s *Service) ExecuteRoundTripAtomic(amount float64, destination string, minProfitXLM float64, baseAsset, counterAsset string) (*models.Payment, *models.SwapRoundTripResult, error) {
	res, err := s.AnalyzeRoundTrip(fmt.Sprintf("%.7f", amount), baseAsset, counterAsset, destination)
	if err != nil {
		return nil, nil, err
	}
	if res.LegA == nil || res.LegB == nil || len(res.LegA.Paths) == 0 || len(res.LegB.Paths) == 0 {
		return nil, res, fmt.Errorf("no paths for atomic round trip")
	}
	if s.signer == nil {
		return nil, res, fmt.Errorf("no signer configured: pass WithSigner to NewService")
	}
	src := s.signer.Address()
	if destination == "" {
		destination = src
	}

	counterTxAsset, err := s.resolveAsset(counterAsset)
	if err != nil {
		return nil, res, fmt.Errorf("unsupported counter asset: %w", err)
	}
	counterHop := models.PathAsset{Code: "XLM"}
	if !counterTxAsset.IsNative() {
		ca := counterTxAsset.(txnbuild.CreditAsset)
		counterHop = models.PathAsset{Code: ca.Code, Issuer: ca.Issuer}
	}
	hops := append([]models.PathAsset{}, res.LegA.Paths[0].Path...)
	hops = append(hops, counterHop)
	hops = append(hops, res.LegB.Paths[0].Path...)
	if len(hops) > 5 {
		return nil, res, fmt.Errorf("combined path has %d hops (max 5)", len(hops))
	}

	feeXLM := float64(s.feeStroops()) / 1e7
	destMin := amount + minProfitXLM
	if baseAsset == "XLM" {
		destMin += feeXLM
	}
	// Round up to 7 decimals so DestMin never undercuts the required profit.
	destMin = math.Ceil(destMin*1e7) / 1e7

	sourceAcct, err := s.FetchAccount(src)
	if err != nil {
		return nil, res, err
	}
	acctBefore, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: src})
	if err == nil {
		res.SnapshotBefore = getAccountSnapshot(acctBefore)
	}

	baseTx, err := s.resolveAsset(baseAsset)
	if err != nil {
		return nil, res, fmt.Errorf("unsupported base asset: %w", err)
	}
	builtHops, err := s.buildPath(hops)
	if err != nil {
		return nil, res, err
	}
	op := &txnbuild.PathPaymentStrictSend{
		SendAsset:   baseTx,
		SendAmount:  fmt.Sprintf("%.7f", amount),
		DestAsset:   baseTx,
		DestMin:     fmt.Sprintf("%.7f", destMin),
		Destination: destination,
		Path:        builtHops,
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              s.feeStroops(),
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(atomicTxTimeout / time.Second))},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, res, fmt.Errorf("build transaction: %w", err)
	}
	tx, err = s.signer.Sign(tx, s.passphrase)
	if err != nil {
		return nil, res, fmt.Errorf("sign transaction: %w", err)
	}
	txB64, err := tx.Base64()
	if err != nil {
		return nil, res, fmt.Errorf("serialize transaction: %w", err)
	}
	txHash, err := tx.HashHex(s.passphrase)
	if err != nil {
		return nil, res, fmt.Errorf("hash transaction: %w", err)
	}
	resp, err := s.client.SubmitTransactionXDR(txB64)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			if rc, rcErr := herr.ResultCodes(); rcErr == nil && rc != nil {
				for _, code := range rc.OperationCodes {
					if code == "op_under_dest_min" {
						return nil, res, fmt.Errorf("cycle no longer profitable at execution (op_under_dest_min) — nothing swapped, only the base fee was paid")
					}
				}
				return nil, res, fmt.Errorf("tx failed — code: %s, ops: %v", rc.TransactionCode, rc.OperationCodes)
			}
			if herr.Problem.Status == 504 {
				// Horizon gave up waiting; the tx may still be included before its time bound expires.
				if landed, perr := s.awaitTransaction(txHash, atomicTxTimeout+10*time.Second); perr == nil {
					resp = *landed
				} else {
					return nil, res, fmt.Errorf("horizon submission timeout; tx %s not found on ledger (%v)", txHash, perr)
				}
			} else {
				return nil, res, fmt.Errorf("horizon error: %s", herr.Problem.Title)
			}
		} else {
			return nil, res, fmt.Errorf("submit to Horizon: %w", err)
		}
	}
	if !resp.Successful {
		return nil, res, fmt.Errorf("tx %s included but failed (result: %s)", resp.Hash, resp.ResultXdr)
	}

	if acctAfter, aerr := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: src}); aerr == nil {
		res.SnapshotAfter = getAccountSnapshot(acctAfter)
	}

	now := time.Now().UTC()
	return &models.Payment{
		ID:          "arb-" + resp.Hash[:12],
		From:        src,
		To:          destination,
		Amount:      fmt.Sprintf("%.7f", amount),
		Asset:       baseAsset,
		Rail:        models.RailSwap,
		Status:      models.PaymentConfirmed,
		Network:     s.Network(),
		Memo:        fmt.Sprintf("Atomic round trip %s→%s→%s", baseAsset, counterAsset, baseAsset),
		Fee:         fmt.Sprintf("%.7f XLM", feeXLM),
		TxHash:      resp.Hash,
		LedgerSeq:   int64(resp.Ledger),
		CreatedAt:   now,
		ConfirmedAt: &now,
	}, res, nil
}

// Simulated swap for development/testing without real Horizon calls
type SimulatedService struct {
	rates map[string]float64
}

func NewSimulatedService() *SimulatedService {
	return &SimulatedService{
		rates: map[string]float64{
			"XLM-USDC":  0.1142,
			"USDC-XLM":  8.756,
			"XLM-EURC":  0.1054,
			"EURC-XLM":  9.487,
			"USDC-EURC": 0.9231,
			"EURC-USDC": 1.0833,
		},
	}
}

func (s *SimulatedService) GetQuote(req models.SwapRequest) (*models.SwapQuote, error) {
	pair := req.SourceAsset + "-" + req.DestAsset
	rate, ok := s.rates[pair]
	if !ok {
		return nil, fmt.Errorf("no rate for pair: %s", pair)
	}

	amount := atof(req.Amount)
	var expectedAmount float64

	switch req.SwapType {
	case models.SwapStrictSend:
		expectedAmount = amount * rate * 0.997 // 0.3% fee
	case models.SwapStrictReceive:
		expectedAmount = amount / rate * 1.003 // 0.3% fee added
	}

	now := time.Now().UTC()
	return &models.SwapQuote{
		QuoteID:        mpCrypto.RandomHex(12),
		SourceAsset:    req.SourceAsset,
		DestAsset:      req.DestAsset,
		SwapType:       req.SwapType,
		Amount:         req.Amount,
		ExpectedAmount: fmt.Sprintf("%.7f", expectedAmount),
		PriceImpact:    0.3,
		NetworkFee:     "0.00001 XLM",
		Paths: []models.SwapPath{{
			Path: []models.PathAsset{
				{Code: req.SourceAsset},
				{Code: req.DestAsset},
			},
			SourceAmount: req.Amount,
			DestAmount:   fmt.Sprintf("%.7f", expectedAmount),
			Price:        rate,
		}},
		ValidUntil: now.Add(30 * time.Second),
		CreatedAt:  now,
	}, nil
}

func (s *SimulatedService) ExecuteSwap(quote *models.SwapQuote, maxSlippage float64, destination string) (*models.Payment, error) {
	now := time.Now().UTC()
	confirmed := now.Add(5 * time.Second)

	return &models.Payment{
		ID:          "swap-" + mpCrypto.RandomHex(12),
		From:        "SIMULATED",
		To:          destination,
		Amount:      quote.Amount,
		Asset:       quote.SourceAsset,
		Rail:        models.RailSwap,
		Status:      models.PaymentConfirmed,
		Network:     models.NetworkStellarTestnet,
		Memo:        fmt.Sprintf("Swap %s→%s", quote.SourceAsset, quote.DestAsset),
		FXRate:      fmt.Sprintf("%.6f", quote.Paths[0].Price),
		Fee:         quote.NetworkFee,
		TxHash:      mpCrypto.StellarTxHash(),
		LedgerSeq:   int64(50000000),
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}, nil
}

// AnalyzeRoundTrip simulates an arbitrage round trip using fixed rates (no Horizon).
// Supports any asset pair as base/counter.
func (s *SimulatedService) AnalyzeRoundTrip(amount, baseAsset, counterAsset string) (*models.SwapRoundTripResult, error) {
	// Build leg A: baseAsset → counterAsset
	quoteA, err := s.GetQuote(models.SwapRequest{
		SourceAsset: baseAsset,
		DestAsset:   counterAsset,
		Amount:      amount,
		SwapType:    models.SwapStrictSend,
	})
	if err != nil {
		return nil, fmt.Errorf("leg A (%s→%s): %w", baseAsset, counterAsset, err)
	}

	// Build leg B: counterAsset → baseAsset
	quoteB, err := s.GetQuote(models.SwapRequest{
		SourceAsset: counterAsset,
		DestAsset:   baseAsset,
		Amount:      quoteA.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
	})
	if err != nil {
		return nil, fmt.Errorf("leg B (%s→%s): %w", counterAsset, baseAsset, err)
	}

	// Calculate profit in base asset terms
	baseIn := atof(amount)
	baseOut := atof(quoteB.ExpectedAmount)
	net := baseOut - baseIn - twoPathPaymentBaseFeesXLM

	// Set result fields
	result := &models.SwapRoundTripResult{
		AmountXLM:       amount,
		BaseAsset:       baseAsset,
		CounterAsset:    counterAsset,
		LegA:            quoteA,
		LegB:            quoteB,
		FeeReserveXLM:   twoTxFeeReserveXLMString(),
		EstimatedNetXLM: net,
	}

	// Store intermediate and returned amounts based on which asset is native/XLM
	if baseAsset == "XLM" {
		result.USDCIntermediate = quoteA.ExpectedAmount
		result.XLMReturned = quoteB.ExpectedAmount
	} else if counterAsset == "XLM" {
		result.XLMReturned = quoteA.ExpectedAmount
		result.USDCIntermediate = quoteB.ExpectedAmount
	} else {
		// Neither is XLM - use generic field names
		result.CounterIntermediate = quoteA.ExpectedAmount
		result.BaseReturned = quoteB.ExpectedAmount
	}

	return result, nil
}

// ExecuteRoundTrip runs two simulated path payments (no chain).
func (s *SimulatedService) ExecuteRoundTrip(amount, slippage float64, destination string, minProfitXLM float64, baseAsset, counterAsset string) ([]*models.Payment, error) {
	res, err := s.AnalyzeRoundTrip(fmt.Sprintf("%.7f", amount), baseAsset, counterAsset)
	if err != nil {
		return nil, err
	}
	if res.EstimatedNetXLM < minProfitXLM {
		return nil, fmt.Errorf("estimated net XLM %.7f is below minimum %.7f (won't execute)", res.EstimatedNetXLM, minProfitXLM)
	}
	p1, err := s.ExecuteSwap(res.LegA, slippage, destination)
	if err != nil {
		return nil, err
	}
	p2, err := s.ExecuteSwap(res.LegB, slippage, destination)
	if err != nil {
		return []*models.Payment{p1}, err
	}
	return []*models.Payment{p1, p2}, nil
}
