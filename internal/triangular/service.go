package triangular

import (
	"fmt"
	"strconv"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/swap"
)

// TriangularPath represents a 3-leg arbitrage opportunity
type TriangularPath struct {
	Name          string
	Leg1          models.SwapRequest
	Leg2          models.SwapRequest
	Leg3          models.SwapRequest
	CombinedRate  float64
	Deviation     float64 // From 1.0 (perfect efficiency)
	IsOpportunity bool
}

// TriangularResult holds the result of a triangular scan
type TriangularResult struct {
	Path          TriangularPath
	Leg1Quote     *models.SwapQuote
	Leg2Quote     *models.SwapQuote
	Leg3Quote     *models.SwapQuote
	StartingXLM   float64
	FinalXLM      float64
	NetProfitXLM  float64
	ProfitPercent float64
	Timestamp     time.Time
	// Statistical analysis fields
	ZScore           float64
	HistoricalMean   float64
	Volatility       float64
	IsMeanReversion  bool
	OpportunityScore int // 0-100
}

// Service handles triangular arbitrage scanning
type Service struct {
	swapSvc *swap.Service
	network models.Network
	history *HistoryStore
	stats   *StatsCalculator
}

// NewService creates a new triangular arbitrage service
func NewService(network models.Network) *Service {
	history, err := NewHistoryStore()
	if err != nil {
		// Log error but continue without history
		history = nil
	}

	return &Service{
		swapSvc: swap.NewService(network),
		network: network,
		history: history,
		stats:   NewStatsCalculator(20),
	}
}

// FindTriangularPaths finds all 3-leg arbitrage paths for the given assets
func (s *Service) FindTriangularPaths(assets []string, amount string) ([]TriangularResult, error) {
	var results []TriangularResult

	// Define triangular cycles
	cycles := [][]string{
		{"XLM", "USDC", "yXLM", "XLM"},
		{"XLM", "yXLM", "USDC", "XLM"},
	}

	for _, cycle := range cycles {
		result, err := s.scanTriangularCycle(cycle, amount)
		if err != nil {
			// Log error but continue with other cycles
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

// scanTriangularCycle scans a single 3-leg cycle
func (s *Service) scanTriangularCycle(cycle []string, amount string) (TriangularResult, error) {
	result := TriangularResult{
		Timestamp: time.Now(),
	}

	startingAmount := amount
	if startingAmount == "" {
		startingAmount = "10"
	}
	startingFloat, err := strconv.ParseFloat(startingAmount, 64)
	if err != nil {
		return result, fmt.Errorf("invalid starting amount: %w", err)
	}
	result.StartingXLM = startingFloat

	// Leg 1: Asset1 -> Asset2
	leg1Req := models.SwapRequest{
		SourceAsset: cycle[0],
		DestAsset:   cycle[1],
		Amount:      startingAmount,
		SwapType:    models.SwapStrictSend,
		Destination: "",
	}

	leg1Quote, err := s.swapSvc.GetQuote(leg1Req)
	if err != nil {
		return result, fmt.Errorf("leg 1 failed (%s->%s): %w", cycle[0], cycle[1], err)
	}
	result.Leg1Quote = leg1Quote
	intermediate1 := leg1Quote.ExpectedAmount

	// Leg 2: Asset2 -> Asset3
	leg2Req := models.SwapRequest{
		SourceAsset: cycle[1],
		DestAsset:   cycle[2],
		Amount:      intermediate1,
		SwapType:    models.SwapStrictSend,
		Destination: "",
	}

	leg2Quote, err := s.swapSvc.GetQuote(leg2Req)
	if err != nil {
		return result, fmt.Errorf("leg 2 failed (%s->%s): %w", cycle[1], cycle[2], err)
	}
	result.Leg2Quote = leg2Quote
	intermediate2 := leg2Quote.ExpectedAmount

	// Leg 3: Asset3 -> Asset1 (back to start)
	leg3Req := models.SwapRequest{
		SourceAsset: cycle[2],
		DestAsset:   cycle[3],
		Amount:      intermediate2,
		SwapType:    models.SwapStrictSend,
		Destination: "",
	}

	leg3Quote, err := s.swapSvc.GetQuote(leg3Req)
	if err != nil {
		return result, fmt.Errorf("leg 3 failed (%s->%s): %w", cycle[2], cycle[3], err)
	}
	result.Leg3Quote = leg3Quote

	// Calculate results
	finalAmount, _ := strconv.ParseFloat(leg3Quote.ExpectedAmount, 64) //nolint:errcheck // parse failure yields 0
	result.FinalXLM = finalAmount
	result.NetProfitXLM = finalAmount - startingFloat
	result.ProfitPercent = (result.NetProfitXLM / startingFloat) * 100

	// Calculate combined rate (should be ~1.0 for efficient market)
	result.Path.CombinedRate = finalAmount / startingFloat
	result.Path.Deviation = result.Path.CombinedRate - 1.0
	result.Path.IsOpportunity = result.ProfitPercent > 0.01 // 0.01% threshold
	result.Path.Name = fmt.Sprintf("%s→%s→%s→%s", cycle[0], cycle[1], cycle[2], cycle[3])

	return result, nil
}

// RecordAndAnalyze saves scan to history and performs statistical analysis
func (s *Service) RecordAndAnalyze(result *TriangularResult) error {
	if s.history == nil || s.stats == nil {
		return nil
	}

	// Record to database
	if err := s.history.RecordScan(*result); err != nil {
		return err
	}

	// Get historical data for analysis
	history, err := s.history.GetHistory(result.Path.Name, s.stats.window)
	if err != nil {
		return err
	}

	// Calculate statistics if we have enough data
	if len(history) >= 10 {
		zScore := s.stats.CalculateZScore(result.Path.CombinedRate, history)
		mean := s.stats.CalculateMean(history)
		volatility := s.stats.CalculateVolatility(history)

		result.ZScore = zScore
		result.HistoricalMean = mean
		result.Volatility = volatility

		// Detect mean reversion opportunity
		isReversion, _ := s.stats.DetectMeanReversion(result.Path.CombinedRate, history, 2.0)
		result.IsMeanReversion = isReversion

		// Calculate opportunity score
		result.OpportunityScore = s.stats.OpportunityScore(*result, history)
	}

	return nil
}

// GetHistoricalAnalysis returns statistical summary for a path
func (s *Service) GetHistoricalAnalysis(path string) (*AnalysisSummary, error) {
	if s.history == nil || s.stats == nil {
		return nil, fmt.Errorf("history not available")
	}

	history, err := s.history.GetHistory(path, 100)
	if err != nil {
		return nil, err
	}

	if len(history) == 0 {
		return nil, fmt.Errorf("no historical data for %s", path)
	}

	summary := &AnalysisSummary{
		Path:         path,
		DataPoints:   len(history),
		MeanRate:     s.stats.CalculateMean(history),
		Volatility:   s.stats.CalculateVolatility(history),
		LatestRate:   history[0].CombinedRate,
		LatestZScore: s.stats.CalculateZScore(history[0].CombinedRate, history),
	}

	// Count opportunities
	opportunities := 0
	for _, h := range history {
		if h.IsOpportunity {
			opportunities++
		}
	}
	summary.OpportunitiesFound = opportunities

	return summary, nil
}

// AnalysisSummary holds statistical analysis results
type AnalysisSummary struct {
	Path               string
	DataPoints         int
	MeanRate           float64
	Volatility         float64
	LatestRate         float64
	LatestZScore       float64
	OpportunitiesFound int
}

// ExtendedTriangularResult adds statistical fields
type ExtendedTriangularResult struct {
	TriangularResult
	ZScore           float64
	HistoricalMean   float64
	Volatility       float64
	IsMeanReversion  bool
	OpportunityScore int // 0-100
}

// ExecuteTriangularSwap executes a 3-leg triangular arbitrage swap
// Returns the final payment transaction if successful
func (s *Service) ExecuteTriangularSwap(result TriangularResult, walletAddress string) (*models.Payment, error) {
	if !result.Path.IsOpportunity {
		return nil, fmt.Errorf("not a profitable opportunity")
	}

	// Default slippage tolerance (0.5%)
	maxSlippage := 0.005

	// Execute leg 1 using the stored quote
	fmt.Printf("Executing leg 1: %s -> %s\n", result.Path.Leg1.SourceAsset, result.Path.Leg1.DestAsset)
	tx1, err := s.swapSvc.ExecuteSwap(result.Leg1Quote, maxSlippage, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("leg 1 failed: %w", err)
	}
	fmt.Printf("✓ Leg 1 complete: %s\n", tx1.ID)

	// Execute leg 2
	fmt.Printf("Executing leg 2: %s -> %s\n", result.Path.Leg2.SourceAsset, result.Path.Leg2.DestAsset)
	tx2, err := s.swapSvc.ExecuteSwap(result.Leg2Quote, maxSlippage, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("leg 2 failed: %w", err)
	}
	fmt.Printf("✓ Leg 2 complete: %s\n", tx2.ID)

	// Execute leg 3
	fmt.Printf("Executing leg 3: %s -> %s\n", result.Path.Leg3.SourceAsset, result.Path.Leg3.DestAsset)
	tx3, err := s.swapSvc.ExecuteSwap(result.Leg3Quote, maxSlippage, walletAddress)
	if err != nil {
		return nil, fmt.Errorf("leg 3 failed: %w", err)
	}
	fmt.Printf("✓ Leg 3 complete: %s\n", tx3.ID)

	return tx3, nil
}
