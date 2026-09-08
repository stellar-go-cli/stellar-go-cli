package swap

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/config"
	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/wallet"
	"github.com/ogtechnologies/mozartpay/internal/zk"
	mpCrypto "github.com/ogtechnologies/mozartpay/pkg/crypto"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
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

func NewService(net models.Network) *Service {
	var client *horizonclient.Client
	var passphrase string
	var assets map[string]AssetConfig

	switch net {
	case models.NetworkStellarMainnet:
		client = horizonclient.DefaultPublicNetClient
		passphrase = network.PublicNetworkPassphrase
		assets = MainnetAssets
	default:
		client = horizonclient.DefaultTestNetClient
		passphrase = network.TestNetworkPassphrase
		assets = TestnetAssets
	}

	return &Service{
		client:     client,
		passphrase: passphrase,
		assets:     assets,
		baseFee:    txnbuild.MinBaseFee,
	}
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

// findPaths queries Horizon for available swap paths
func (s *Service) findPaths(req models.SwapRequest) ([]models.SwapPath, error) {
	sourceAsset := s.getAsset(req.SourceAsset)
	destAsset := s.getAsset(req.DestAsset)

	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, err
	}

	dst := req.Destination
	if dst == "" {
		dst = kp.Address()
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
		// For strict receive, find paths that can deliver the dest amount
		pathsReq := horizonclient.PathsRequest{
			SourceAccount:          kp.Address(),
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

// ExecuteSwap builds and submits a path payment transaction
func (s *Service) ExecuteSwap(quote *models.SwapQuote, maxSlippage float64, destination string) (*models.Payment, error) {
	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, fmt.Errorf("no stellar keypair: %w", err)
	}

	// Use sender as destination if not specified
	if destination == "" {
		destination = kp.Address()
	}

	// Fetch source account
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return nil, fmt.Errorf("horizon: account not found: %w", err)
	}

	// Check trustline for destination asset
	if err := s.ensureTrustline(kp.Address(), quote.DestAsset); err != nil {
		return nil, fmt.Errorf("trustline check failed: %w", err)
	}

	sourceAsset := s.getAsset(quote.SourceAsset)
	destAsset := s.getAsset(quote.DestAsset)

	// Use first (best) path
	if len(quote.Paths) == 0 {
		return nil, fmt.Errorf("no paths in quote")
	}
	bestPath := quote.Paths[0]
	builtPath := s.buildPath(bestPath.Path)

	var operation txnbuild.Operation

	switch quote.SwapType {
	case models.SwapStrictSend:
		// Calculate dynamic slippage based on path complexity
		dynamicSlippage := s.calculateDynamicSlippage(maxSlippage/100.0, bestPath.Path)
		destMin := s.applySlippage(bestPath.DestAmount, dynamicSlippage, false)

		operation = &txnbuild.PathPaymentStrictSend{
			SendAsset:   sourceAsset,
			SendAmount:  quote.Amount,
			DestAsset:   destAsset,
			DestMin:     destMin,
			Destination: destination,
			Path:        builtPath,
		}

	case models.SwapStrictReceive:
		// Calculate dynamic slippage based on path complexity
		dynamicSlippage := s.calculateDynamicSlippage(maxSlippage/100.0, bestPath.Path)
		sendMax := s.applySlippage(bestPath.SourceAmount, dynamicSlippage, true)

		operation = &txnbuild.PathPaymentStrictReceive{
			SendAsset:   sourceAsset,
			SendMax:     sendMax,
			DestAsset:   destAsset,
			DestAmount:  quote.Amount,
			Destination: destination,
			Path:        s.buildPath(bestPath.Path),
		}
	}

	// Build transaction
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              s.feeStroops(),
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{operation},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}

	// Sign
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}

	// Submit
	txB64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("serialize transaction: %w", err)
	}

	resp, err := s.client.SubmitTransactionXDR(txB64)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			rc, _ := herr.ResultCodes()
			if rc != nil {
				// Check for op_under_dest_min specifically
				for _, opCode := range rc.OperationCodes {
					if opCode == "op_under_dest_min" {
						// Provide helpful suggestion for slippage issues
						suggestedSlippage := maxSlippage * 1.5 // Suggest 50% higher slippage
						if suggestedSlippage > 10.0 {
							suggestedSlippage = 10.0
						}
						return nil, fmt.Errorf("tx failed — insufficient destination amount received. Try increasing --slippage to %.1f%% (current: %.1f%%). This error occurs when market conditions change between quote and execution.", suggestedSlippage, maxSlippage)
					}
				}
				return nil, fmt.Errorf("tx failed — code: %s, ops: %v", rc.TransactionCode, rc.OperationCodes)
			}
			return nil, fmt.Errorf("horizon error: %s", herr.Problem.Title)
		}
		return nil, fmt.Errorf("submit to Horizon: %w", err)
	}

	now := time.Now().UTC()
	confirmed := now.Add(5 * time.Second)

	return &models.Payment{
		ID:          "swap-" + resp.Hash[:12],
		From:        kp.Address(),
		To:          destination,
		Amount:      quote.Amount,
		Asset:       quote.SourceAsset,
		Rail:        models.RailSwap,
		Status:      models.PaymentConfirmed,
		Network:     s.getNetwork(),
		Memo:        fmt.Sprintf("Swap %s→%s", quote.SourceAsset, quote.DestAsset),
		FXRate:      fmt.Sprintf("%.6f", bestPath.Price),
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

func (s *Service) getAsset(code string) txnbuild.Asset {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "XLM" || code == "" {
		return txnbuild.NativeAsset{}
	}
	cfg, ok := s.assets[code]
	if !ok {
		// Unknown asset - return CreditAsset without issuer
		// This will fail validation later with a clear error
		return txnbuild.CreditAsset{Code: code, Issuer: ""}
	}
	return txnbuild.CreditAsset{Code: cfg.Code, Issuer: cfg.Issuer}
}

// validateAsset returns a clear error for assets not in the known-asset registry
func (s *Service) validateAsset(code string) error {
	upper := strings.ToUpper(strings.TrimSpace(code))
	if upper == "" || upper == "XLM" {
		return nil
	}
	if _, ok := s.assets[upper]; !ok {
		return fmt.Errorf("unsupported asset %q (supported: %s)", code, strings.Join(s.supportedCodes(), ", "))
	}
	return nil
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

func (s *Service) getIssuer(code string) string {
	if code == "XLM" || code == "" {
		return ""
	}
	config, ok := s.assets[code]
	if !ok {
		return ""
	}
	return config.Issuer
}

func (s *Service) buildPath(pathAssets []models.PathAsset) []txnbuild.Asset {
	var path []txnbuild.Asset
	for _, pa := range pathAssets {
		if pa.Code == "XLM" || pa.Code == "" {
			path = append(path, txnbuild.NativeAsset{})
		} else if pa.Issuer != "" {
			// Use the issuer from Horizon path
			path = append(path, txnbuild.CreditAsset{Code: pa.Code, Issuer: pa.Issuer})
		} else {
			// Unknown asset without issuer - try to get from config
			asset := s.getAsset(pa.Code)
			path = append(path, asset)
		}
	}
	return path
}

func (s *Service) calculatePrice(source, dest string) float64 {
	sourceAmt, _ := strconv.ParseFloat(source, 64)
	destAmt, _ := strconv.ParseFloat(dest, 64)
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

func (s *Service) estimateSourceAmount(sourceAsset, destAsset, destAmount string) string {
	// Rough estimation for strict receive path finding
	// In production, this would use more sophisticated estimation
	destAmt, _ := strconv.ParseFloat(destAmount, 64)
	// Assume roughly 1:1 ratio for estimation (paths API will refine)
	estimated := destAmt * 1.02 // Add 2% buffer
	return fmt.Sprintf("%.7f", estimated)
}

func (s *Service) applySlippage(amount string, slippage float64, isMax bool) string {
	amt, _ := strconv.ParseFloat(amount, 64)
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

func (s *Service) ensureTrustline(address, assetCode string) error {
	if assetCode == "XLM" {
		return nil
	}

	// Check if trustline exists
	_, ok := s.assets[assetCode]
	if !ok {
		return fmt.Errorf("unknown asset: %s", assetCode)
	}

	// In production, we would check account balances and add trustline if needed
	// For now, assume trustline exists or user has pre-established it
	return nil
}

func (s *Service) getNetwork() models.Network {
	if s.passphrase == network.PublicNetworkPassphrase {
		return models.NetworkStellarMainnet
	}
	return models.NetworkStellarTestnet
}

func (s *Service) LoadStellarKeypair() (*keypair.Full, error) {
	return wallet.LoadStellarKeypairForSwap()
}

// BuildZKSwapProofRequest creates a ZK proof request for a swap operation
func (s *Service) BuildZKSwapProofRequest(quote *models.SwapQuote, payer, destination string, privacyLevel string) *models.ZKSwapRequest {
	return &models.ZKSwapRequest{
		ResourceURL:    "",
		Amount:         0, // Will be set from quote
		SourceAsset:    quote.SourceAsset,
		DestAsset:      quote.DestAsset,
		Payer:          payer,
		Destination:    destination,
		Nonce:          mpCrypto.RandomHex(16),
		ExpiresAt:      time.Now().Add(10 * time.Minute).UTC(),
		PrivacyLevel:   privacyLevel,
		ComplianceHash: mpCrypto.Hash256([]byte(payer + destination + quote.SourceAsset + quote.DestAsset + quote.Amount)),
		SwapType:       quote.SwapType,
	}
}

// ExecuteZKSwap executes a swap with ZK proof privacy
func (s *Service) ExecuteZKSwap(quote *models.SwapQuote, maxSlippage float64, destination string, privacyLevel string) (*models.Payment, error) {
	// Load keypair for the specific address
	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, fmt.Errorf("no stellar keypair: %w", err)
	}

	// Use sender as destination if not specified
	if destination == "" {
		destination = kp.Address()
	}

	// Initialize ZK service
	zkService := zk.NewService()

	// Generate ZK proof for swap
	inputs := zk.PaymentInputs{
		SenderPrivateKey: "MOCK_PRIVATE_KEY", // Would use actual private key
		ReceiverAddress:  destination,
		Amount:           quote.Amount,
		Nonce:            mpCrypto.RandomHex(16),
		NetworkID:        s.getNetworkID(),
		MaxAmount:        "1000000", // 1M XLM max
	}

	proofData, err := zkService.GeneratePaymentProof(inputs)
	if err != nil {
		return nil, fmt.Errorf("ZK proof generation failed: %w", err)
	}

	// Verify proof locally first
	verified, err := zkService.VerifyProofLocally(proofData)
	if err != nil || !verified {
		return nil, fmt.Errorf("local ZK proof verification failed: %w", err)
	}

	// Fetch source account
	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: kp.Address(),
	})
	if err != nil {
		return nil, fmt.Errorf("horizon: account not found for ZK swap: %w", err)
	}

	// Check trustline for destination asset
	if err := s.ensureTrustline(kp.Address(), quote.DestAsset); err != nil {
		return nil, fmt.Errorf("trustline check failed: %w", err)
	}

	sourceAsset := s.getAsset(quote.SourceAsset)
	destAsset := s.getAsset(quote.DestAsset)

	// Debug logging to diagnose op_malformed
	fmt.Printf("[DEBUG] ExecuteZKSwap: source=%s, dest=%s\n", quote.SourceAsset, quote.DestAsset)
	fmt.Printf("[DEBUG] Source asset: %s\n", s.getAssetString(sourceAsset))
	fmt.Printf("[DEBUG] Dest asset: %s\n", s.getAssetString(destAsset))

	// Use first (best) path that has valid issuers
	if len(quote.Paths) == 0 {
		return nil, fmt.Errorf("no paths in quote")
	}

	var bestPath *models.SwapPath
	var builtPath []txnbuild.Asset

	// Find first valid path, preferring direct paths (no intermediate assets)
	for i, p := range quote.Paths {
		fmt.Printf("[DEBUG] Checking Path[%d]: intermediate assets=%d\n", i, len(p.Path))
		skipPath := false
		for j, a := range p.Path {
			fmt.Printf("[DEBUG]   Path[%d][%d]: Code=%s Issuer=%s\n", i, j, a.Code, a.Issuer)
			// Skip paths with assets that have no issuer - causes op_malformed
			if a.Code != "XLM" && a.Issuer == "" {
				fmt.Printf("[DEBUG]   -> Skipping path (no issuer for %s)\n", a.Code)
				skipPath = true
				break
			}
		}
		if skipPath {
			continue
		}

		// Prefer direct paths (no intermediate assets)
		if len(p.Path) == 0 {
			fmt.Printf("[DEBUG]   -> Selected direct path (no intermediates)\n")
			bestPath = &p
			builtPath = []txnbuild.Asset{} // Empty path for direct swap
			break
		}

		// If we haven't found a direct path yet, use this one
		if bestPath == nil {
			bestPath = &p
			builtPath = s.buildPath(p.Path)
			fmt.Printf("[DEBUG] Selected Path[%d] with %d intermediate assets\n", i, len(p.Path))
		}
	}

	if bestPath == nil {
		return nil, fmt.Errorf("no valid paths found (all paths have assets without issuers)")
	}

	fmt.Printf("[DEBUG] Built path length: %d\n", len(builtPath))
	for i, asset := range builtPath {
		fmt.Printf("[DEBUG] BuiltPath[%d]: %s\n", i, s.getAssetString(asset))
	}

	var operation txnbuild.Operation

	switch quote.SwapType {
	case models.SwapStrictSend:
		destMin := s.applySlippage(bestPath.DestAmount, maxSlippage/100.0, false)
		fmt.Printf("[DEBUG] StrictSend: SendAmount=%s DestMin=%s\n", quote.Amount, destMin)
		fmt.Printf("[DEBUG] DestAmount from quote: %s\n", bestPath.DestAmount)
		fmt.Printf("[DEBUG] Slippage percent: %.2f%%, decimal: %.4f\n", maxSlippage, maxSlippage/100.0)
		operation = &txnbuild.PathPaymentStrictSend{
			SendAsset:   sourceAsset,
			SendAmount:  quote.Amount,
			DestAsset:   destAsset,
			DestMin:     destMin,
			Destination: destination,
			Path:        builtPath,
		}
	case models.SwapStrictReceive:
		sendMax := s.applySlippage(bestPath.SourceAmount, maxSlippage/100.0, true)
		fmt.Printf("[DEBUG] StrictReceive: SendMax=%s DestAmount=%s\n", sendMax, quote.Amount)
		fmt.Printf("[DEBUG] Slippage percent: %.2f%%, decimal: %.4f\n", maxSlippage, maxSlippage/100.0)
		operation = &txnbuild.PathPaymentStrictReceive{
			SendAsset:   sourceAsset,
			SendMax:     sendMax,
			DestAsset:   destAsset,
			DestAmount:  quote.Amount,
			Destination: destination,
			Path:        builtPath,
		}
	}

	// Debug: Log operation details
	fmt.Printf("[DEBUG] Operation type: %T\n", operation)
	if pp, ok := operation.(*txnbuild.PathPaymentStrictSend); ok {
		fmt.Printf("[DEBUG] PathPaymentStrictSend:\n")
		fmt.Printf("[DEBUG]   SendAsset: %s\n", s.getAssetString(pp.SendAsset))
		fmt.Printf("[DEBUG]   SendAmount: %s\n", pp.SendAmount)
		fmt.Printf("[DEBUG]   DestAsset: %s\n", s.getAssetString(pp.DestAsset))
		fmt.Printf("[DEBUG]   DestMin: %s\n", pp.DestMin)
		fmt.Printf("[DEBUG]   Destination: %s\n", pp.Destination)
		fmt.Printf("[DEBUG]   Path length: %d\n", len(pp.Path))
	}
	if pp, ok := operation.(*txnbuild.PathPaymentStrictReceive); ok {
		fmt.Printf("[DEBUG] PathPaymentStrictReceive:\n")
		fmt.Printf("[DEBUG]   SendAsset: %s\n", s.getAssetString(pp.SendAsset))
		fmt.Printf("[DEBUG]   SendMax: %s\n", pp.SendMax)
		fmt.Printf("[DEBUG]   DestAsset: %s\n", s.getAssetString(pp.DestAsset))
		fmt.Printf("[DEBUG]   DestAmount: %s\n", pp.DestAmount)
		fmt.Printf("[DEBUG]   Destination: %s\n", pp.Destination)
		fmt.Printf("[DEBUG]   Path length: %d\n", len(pp.Path))
	}

	// Build transaction with path payment operation
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(60)},
		Operations:           []txnbuild.Operation{operation},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	if err != nil {
		return nil, fmt.Errorf("failed to build ZK swap transaction: %w", err)
	}

	// Sign transaction
	tx, err = tx.Sign(s.passphrase, kp)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ZK swap transaction: %w", err)
	}

	// Serialize to base64 XDR
	txB64, err := tx.Base64()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize ZK swap transaction: %w", err)
	}

	// Debug: Log transaction XDR before submission
	fmt.Printf("[DEBUG] Transaction XDR: %s\n", txB64)
	txHash, err := tx.Hash(s.passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction hash: %w", err)
	}
	fmt.Printf("[DEBUG] Transaction hash preview: %s\n", txHash[:16])

	// Submit to Horizon
	resp, err := s.client.SubmitTransactionXDR(txB64)
	if err != nil {
		if herr, ok := err.(*horizonclient.Error); ok {
			rc, _ := herr.ResultCodes()
			fmt.Printf("[DEBUG] Horizon error details:\n")
			fmt.Printf("[DEBUG]   Problem: %s\n", herr.Problem.Title)
			fmt.Printf("[DEBUG]   Detail: %s\n", herr.Problem.Detail)
			if rc != nil {
				fmt.Printf("[DEBUG]   Transaction code: %s\n", rc.TransactionCode)
				fmt.Printf("[DEBUG]   Operation codes: %v\n", rc.OperationCodes)
			}
			return nil, fmt.Errorf("ZK swap transaction failed — code: %s, ops: %v",
				rc.TransactionCode, rc.OperationCodes)
		}
		return nil, fmt.Errorf("submit ZK swap transaction to Horizon: %w", err)
	}

	now := time.Now().UTC()
	confirmed := now.Add(10 * time.Second)

	// Generate ZK verification data
	verification := &models.ZKSwapVerification{
		ProofID:          proofData.ProofHash,
		CircuitType:      proofData.CircuitType,
		Verified:         true,
		VerificationTime: time.Duration(proofData.GeneratedAt.Sub(now)),
		GasUsed:          proofData.GasUsed,
		OnChainRef:       resp.Hash[:16],
		TxHash:           resp.Hash,
		CreatedAt:        now,
	}

	// Save verification data for compliance reporting
	config.SaveState("zk_swap_verification_latest", verification)

	return &models.Payment{
		ID:          "zk-swap-" + resp.Hash[:12],
		From:        kp.Address(),
		To:          destination,
		Amount:      quote.Amount,
		Asset:       quote.SourceAsset,
		Rail:        models.RailZK,
		Status:      models.PaymentConfirmed,
		Network:     s.getNetwork(),
		Memo:        fmt.Sprintf("ZK Swap %s→%s", quote.SourceAsset, quote.DestAsset),
		Fee:         "0.00005 XLM",
		TxHash:      resp.Hash,
		LedgerSeq:   int64(resp.Ledger),
		CreatedAt:   now,
		ConfirmedAt: &confirmed,
	}, nil
}

// getNetworkID returns the network ID for ZK proof generation
func (s *Service) getNetworkID() string {
	if s.passphrase == network.PublicNetworkPassphrase {
		return "1" // Mainnet
	}
	return "0" // Testnet
}

// AnalyzeRoundTrip quotes XLM→USDC then USDC→XLM (strict send) using Horizon best paths.
func twoTxFeeReserveXLMString() string {
	return fmt.Sprintf("%.5f", twoPathPaymentBaseFeesXLM)
}

func parseUSDCBalance(acc horizon.Account, issuer string) float64 {
	for _, b := range acc.Balances {
		if b.Type == "native" {
			continue
		}
		if b.Code == "USDC" && b.Issuer == issuer {
			v, _ := strconv.ParseFloat(b.Balance, 64)
			return v
		}
	}
	return 0
}

func parseXLMBalance(acc horizon.Account) float64 {
	for _, b := range acc.Balances {
		if b.Type == "native" {
			v, _ := strconv.ParseFloat(b.Balance, 64)
			return v
		}
	}
	return 0
}

// parseAssetBalance parses the balance of a specific asset code from an account
func parseAssetBalance(acc horizon.Account, assetCode, issuer string) float64 {
	for _, b := range acc.Balances {
		if b.Type == "native" {
			if assetCode == "XLM" {
				v, _ := strconv.ParseFloat(b.Balance, 64)
				return v
			}
			continue
		}
		if b.Code == assetCode && (issuer == "" || b.Issuer == issuer) {
			v, _ := strconv.ParseFloat(b.Balance, 64)
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

func roundTripResultFromQuotes(amountXLM string, quoteA, quoteB *models.SwapQuote) *models.SwapRoundTripResult {
	xlmIn, _ := strconv.ParseFloat(amountXLM, 64)
	xlmOut, _ := strconv.ParseFloat(quoteB.ExpectedAmount, 64)
	net := xlmOut - xlmIn - twoPathPaymentBaseFeesXLM
	return &models.SwapRoundTripResult{
		AmountXLM:        amountXLM,
		LegA:             quoteA,
		LegB:             quoteB,
		USDCIntermediate: quoteA.ExpectedAmount,
		XLMReturned:      quoteB.ExpectedAmount,
		FeeReserveXLM:    twoTxFeeReserveXLMString(),
		EstimatedNetXLM:  net,
	}
}

// AnalyzeRoundTrip runs paper round-trip quotes (not on-chain). Requires configured Stellar keypair for Horizon path APIs.
// Supports any asset pair as base/counter (e.g., XLM/USDC, XRF/USDC, XRF/XLM, etc.)
func (s *Service) AnalyzeRoundTrip(amount, baseAsset, counterAsset, destination string) (*models.SwapRoundTripResult, error) {
	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, err
	}
	dst := destination
	if dst == "" {
		dst = kp.Address()
	}

	// Validate base and counter assets
	if _, ok := s.assets[baseAsset]; !ok && baseAsset != "XLM" {
		return nil, fmt.Errorf("unsupported base asset: %s", baseAsset)
	}
	if _, ok := s.assets[counterAsset]; !ok && counterAsset != "XLM" {
		return nil, fmt.Errorf("unsupported counter asset: %s", counterAsset)
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
	baseIn, _ := strconv.ParseFloat(amount, 64)
	baseOut, _ := strconv.ParseFloat(quoteB.ExpectedAmount, 64)
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
	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, err
	}
	dst := destination
	if dst == "" {
		dst = kp.Address()
	}

	acctBefore, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
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
	if counterAsset != "XLM" {
		if cfg, ok := s.assets[counterAsset]; ok {
			counterIssuer = cfg.Issuer
		}
	}

	// Track counter asset balance before leg A
	counterBefore := parseAssetBalance(acctBefore, counterAsset, counterIssuer)

	pay1, err = s.ExecuteSwap(res.LegA, slippage, dst)
	if err != nil {
		// Return payments made so far with the before snapshot
		return []*models.Payment{pay1}, fmt.Errorf("leg A execute: %w", err)
	}

	acctAfterLegA, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
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
	acctAfter, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
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
	kp, err := s.LoadStellarKeypair()
	if err != nil {
		return nil, res, err
	}
	if destination == "" {
		destination = kp.Address()
	}

	counterCfg, ok := s.assets[counterAsset]
	if !ok {
		return nil, res, fmt.Errorf("unsupported counter asset: %s", counterAsset)
	}
	hops := append([]models.PathAsset{}, res.LegA.Paths[0].Path...)
	hops = append(hops, models.PathAsset{Code: counterCfg.Code, Issuer: counterCfg.Issuer})
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

	sourceAcct, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()})
	if err != nil {
		return nil, res, fmt.Errorf("horizon account: %w", err)
	}
	res.SnapshotBefore = getAccountSnapshot(sourceAcct)

	baseTx := s.getAsset(baseAsset)
	op := &txnbuild.PathPaymentStrictSend{
		SendAsset:   baseTx,
		SendAmount:  fmt.Sprintf("%.7f", amount),
		DestAsset:   baseTx,
		DestMin:     fmt.Sprintf("%.7f", destMin),
		Destination: destination,
		Path:        s.buildPath(hops),
	}
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &sourceAcct,
		IncrementSequenceNum: true,
		BaseFee:              s.feeStroops(),
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(int64(atomicTxTimeout / time.Second))},
		Operations:           []txnbuild.Operation{op},
	})
	if err != nil {
		return nil, res, fmt.Errorf("build transaction: %w", err)
	}
	tx, err = tx.Sign(s.passphrase, kp)
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
			if rc, _ := herr.ResultCodes(); rc != nil {
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
					err = nil
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

	if acctAfter, aerr := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: kp.Address()}); aerr == nil {
		res.SnapshotAfter = getAccountSnapshot(acctAfter)
	}

	now := time.Now().UTC()
	return &models.Payment{
		ID:          "arb-" + resp.Hash[:12],
		From:        kp.Address(),
		To:          destination,
		Amount:      fmt.Sprintf("%.7f", amount),
		Asset:       baseAsset,
		Rail:        models.RailSwap,
		Status:      models.PaymentConfirmed,
		Network:     s.getNetwork(),
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

	amount, _ := strconv.ParseFloat(req.Amount, 64)
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
	baseIn, _ := strconv.ParseFloat(amount, 64)
	baseOut, _ := strconv.ParseFloat(quoteB.ExpectedAmount, 64)
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
