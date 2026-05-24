package scanner

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/swap"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const twoPathPaymentBaseFeesXLM = 2 * 0.00001 // 2 txs at 0.00001 XLM each

// Service handles arbitrage scanning and Prometheus metrics
type Service struct {
	network    models.Network
	swapSvc    *swap.Service
	minProfit  float64
	testAmount string
}

// NewService creates a new scanner service
func NewService(network models.Network, minProfit float64, testAmount string) *Service {
	return &Service{
		network:    network,
		swapSvc:    swap.NewService(network),
		minProfit:  minProfit,
		testAmount: testAmount,
	}
}

// ScanResult holds the result of scanning a single pair
type ScanResult struct {
	PairName      string
	BaseAsset     string
	QuoteAsset    string
	TestAmount    string
	LegAQuote     string
	LegBQuote     string
	FinalAmount   float64
	NetProfitXLM  float64
	SpreadPercent float64
	IsProfitable  bool
	LegAPath      string
	LegBPath      string
	Error         error
}

// ScanAll scans all liquid pairs and returns results
func (s *Service) ScanAll() ([]ScanResult, error) {
	pairs := GetPairsForNetwork(s.network)
	results := make([]ScanResult, 0, len(pairs))

	for _, pair := range pairs {
		result := s.scanPair(pair)
		results = append(results, result)

		// Update Prometheus metrics
		s.updateMetrics(pair, result)
	}

	return results, nil
}

// ScanPair scans a specific pair by name
func (s *Service) ScanPair(pairName string) (ScanResult, error) {
	pair, found := GetPairByName(s.network, pairName)
	if !found {
		return ScanResult{}, fmt.Errorf("pair %s not found for network %s", pairName, s.network)
	}

	result := s.scanPair(pair)
	s.updateMetrics(pair, result)
	return result, nil
}

// scanPair performs the actual round-trip scan for a single pair
func (s *Service) scanPair(pair AssetPair) ScanResult {
	start := time.Now()
	defer func() {
		ScanDuration.WithLabelValues(pair.Name, string(s.network)).Observe(time.Since(start).Seconds())
	}()

	result := ScanResult{
		PairName:   pair.Name,
		BaseAsset:  pair.BaseAsset,
		QuoteAsset: pair.QuoteAsset,
		TestAmount: s.testAmount,
	}

	// Leg A: Base -> Quote (just use asset codes, swap service has issuers)
	reqA := models.SwapRequest{
		SourceAsset: pair.BaseAsset,
		DestAsset:   pair.QuoteAsset,
		Amount:      pair.TestAmount,
		SwapType:    models.SwapStrictSend,
		Destination: "", // Not needed for quotes
	}

	quoteA, err := s.swapSvc.GetQuote(reqA)
	if err != nil {
		result.Error = fmt.Errorf("leg A quote failed: %w", err)
		return result
	}

	result.LegAQuote = quoteA.ExpectedAmount
	if len(quoteA.Paths) > 0 {
		result.LegAPath = formatPath(quoteA.Paths[0].Path)
	}

	// Leg B: Quote -> Base (round trip)
	reqB := models.SwapRequest{
		SourceAsset: pair.QuoteAsset,
		DestAsset:   pair.BaseAsset,
		Amount:      quoteA.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
		Destination: "",
	}

	quoteB, err := s.swapSvc.GetQuote(reqB)
	if err != nil {
		result.Error = fmt.Errorf("leg B quote failed: %w", err)
		return result
	}

	result.LegBQuote = quoteB.ExpectedAmount
	if len(quoteB.Paths) > 0 {
		result.LegBPath = formatPath(quoteB.Paths[0].Path)
	}

	// Calculate profit with realistic costs
	testAmountFloat, err := strconv.ParseFloat(pair.TestAmount, 64)
	if err != nil {
		testAmountFloat = 0
	}
	finalAmountFloat, err := strconv.ParseFloat(quoteB.ExpectedAmount, 64)
	if err != nil {
		finalAmountFloat = 0
	}
	netProfit := finalAmountFloat - testAmountFloat
	spreadPercent := 0.0
	if testAmountFloat != 0 {
		spreadPercent = (netProfit / testAmountFloat) * 100
	}

	// Realistic cost calculation for mainnet
	baseFee := 0.00001                           // 0.00001 XLM per operation
	totalFees := baseFee * 2                     // 2 operations
	slippageEstimate := finalAmountFloat * 0.002 // 0.2% slippage for mainnet
	liquidityImpact := finalAmountFloat * 0.001  // 0.1% for larger trades

	estimatedNet := netProfit - totalFees - slippageEstimate - liquidityImpact

	result.FinalAmount = finalAmountFloat
	result.NetProfitXLM = netProfit
	result.SpreadPercent = spreadPercent
	result.IsProfitable = estimatedNet > s.minProfit

	// Update opportunity counter if profitable
	if result.IsProfitable {
		OpportunitiesFound.WithLabelValues(pair.Name, string(s.network)).Inc()
	}

	return result
}

// updateMetrics updates Prometheus metrics for a scan result
func (s *Service) updateMetrics(pair AssetPair, result ScanResult) {
	SpreadPercent.WithLabelValues(pair.Name, string(s.network)).Set(result.SpreadPercent)
	ProfitXLM.WithLabelValues(pair.Name, string(s.network)).Set(result.NetProfitXLM)
	LastCheckTimestamp.WithLabelValues(pair.Name, string(s.network)).Set(float64(time.Now().Unix()))

	if result.IsProfitable {
		IsProfitable.WithLabelValues(pair.Name, string(s.network)).Set(1)
	} else {
		IsProfitable.WithLabelValues(pair.Name, string(s.network)).Set(0)
	}

	// Update leg quotes if available
	if result.LegAQuote != "" {
		if amt, err := strconv.ParseFloat(result.LegAQuote, 64); err == nil {
			LegAQuote.WithLabelValues(pair.Name, string(s.network), pair.QuoteAsset).Set(amt)
		}
	}
	if result.LegBQuote != "" {
		if amt, err := strconv.ParseFloat(result.LegBQuote, 64); err == nil {
			LegBQuote.WithLabelValues(pair.Name, string(s.network), pair.BaseAsset).Set(amt)
		}
	}
}

// StartMetricsServer starts the Prometheus HTTP server and begins listening
func (s *Service) StartMetricsServer(port int) (*http.Server, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start listening immediately with error channel
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Give server time to start
	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		return nil, fmt.Errorf("failed to start metrics server: %w", err)
	default:
		// Server started successfully
		fmt.Printf("✓ Metrics server started on port %d\n", port)
		return server, nil
	}
}

// StartMetricsServerWithChannel starts the server and returns the error channel for monitoring
func (s *Service) StartMetricsServerWithChannel(port int) (*http.Server, chan error, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Start listening immediately with error channel
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Give server time to start
	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-errChan:
		return nil, nil, fmt.Errorf("failed to start metrics server: %w", err)
	default:
		// Server started successfully
		fmt.Printf("✓ Metrics server started on port %d\n", port)
		return server, errChan, nil
	}
}

// RunScanAndServe performs a scan and optionally keeps metrics server running
func (s *Service) RunScanAndServe(port int, block bool) ([]ScanResult, error) {
	// Start metrics server (now auto-starts)
	server, err := s.StartMetricsServer(port)
	if err != nil {
		return nil, fmt.Errorf("failed to start metrics server: %w", err)
	}

	// Perform scan
	results, err := s.ScanAll()
	if err != nil {
		return nil, err
	}

	// If blocking, keep server running for a short time
	if block {
		time.Sleep(30 * time.Second)
		server.Shutdown(context.Background())
	}

	return results, nil
}

// formatPath formats a path slice into a string
func formatPath(path []models.PathAsset) string {
	if len(path) == 0 {
		return "direct"
	}
	result := ""
	for i, asset := range path {
		if i > 0 {
			result += " -> "
		}
		result += asset.Code
	}
	return result
}
