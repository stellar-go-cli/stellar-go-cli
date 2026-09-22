package triangular

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/swap"
)

// RealDataBacktestEngine runs backtests using real-time Horizon quotes
// It collects multiple quote samples to simulate historical data
type RealDataBacktestEngine struct {
	swapSvc *swap.Service
	stats   *StatsCalculator
	network models.Network
}

// RealBacktestResult extends BacktestResult with real data info
type RealBacktestResult struct {
	BacktestResult
	DataSource       string // "horizon-realtime"
	SamplesCollected int
	ActualDateRange  string
}

// NewRealDataBacktestEngine creates a backtest engine using real Horizon data
func NewRealDataBacktestEngine(network models.Network) *RealDataBacktestEngine {
	return &RealDataBacktestEngine{
		swapSvc: swap.NewService(network),
		stats:   NewStatsCalculator(20),
		network: network,
	}
}

// HistoricalTriangularPoint represents a single point in time with all 3 leg rates
type HistoricalTriangularPoint struct {
	Timestamp    time.Time
	Leg1Rate     float64 // Asset1 -> Asset2
	Leg2Rate     float64 // Asset2 -> Asset3
	Leg3Rate     float64 // Asset3 -> Asset1
	CombinedRate float64 // Product of all 3 rates
	Deviation    float64 // CombinedRate - 1.0
}

// collectRealtimeSamples gathers real quote data from Horizon by sampling multiple times
func (r *RealDataBacktestEngine) collectRealtimeSamples(cycle []string, samples int, interval time.Duration) ([]HistoricalTriangularPoint, error) {
	var points []HistoricalTriangularPoint

	fmt.Printf("  Collecting %d real-time samples from Horizon (interval: %v)...\n", samples, interval)

	for i := 0; i < samples; i++ {
		if i > 0 {
			time.Sleep(interval)
		}

		point, err := r.sampleSinglePoint(cycle)
		if err != nil {
			fmt.Printf("    Sample %d failed: %v\n", i+1, err)
			continue
		}

		points = append(points, *point)
		fmt.Printf("    ✓ Sample %d/%d: combined_rate=%.6f\n", i+1, samples, point.CombinedRate)
	}

	if len(points) < 10 {
		return nil, fmt.Errorf("insufficient samples collected: %d (need at least 10)", len(points))
	}

	return points, nil
}

// sampleSinglePoint gets one real-time quote from Horizon
func (r *RealDataBacktestEngine) sampleSinglePoint(cycle []string) (*HistoricalTriangularPoint, error) {
	point := &HistoricalTriangularPoint{
		Timestamp: time.Now(),
	}

	startingAmount := "10"

	// Leg 1
	leg1Req := models.SwapRequest{
		SourceAsset: cycle[0],
		DestAsset:   cycle[1],
		Amount:      startingAmount,
		SwapType:    models.SwapStrictSend,
	}
	leg1Quote, err := r.swapSvc.GetQuote(leg1Req)
	if err != nil {
		return nil, fmt.Errorf("leg1: %w", err)
	}
	point.Leg1Rate = calculateRateFromQuote(leg1Quote)

	// Leg 2
	leg2Req := models.SwapRequest{
		SourceAsset: cycle[1],
		DestAsset:   cycle[2],
		Amount:      leg1Quote.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
	}
	leg2Quote, err := r.swapSvc.GetQuote(leg2Req)
	if err != nil {
		return nil, fmt.Errorf("leg2: %w", err)
	}
	point.Leg2Rate = calculateRateFromQuote(leg2Quote)

	// Leg 3
	leg3Req := models.SwapRequest{
		SourceAsset: cycle[2],
		DestAsset:   cycle[3],
		Amount:      leg2Quote.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
	}
	leg3Quote, err := r.swapSvc.GetQuote(leg3Req)
	if err != nil {
		return nil, fmt.Errorf("leg3: %w", err)
	}
	point.Leg3Rate = calculateRateFromQuote(leg3Quote)

	// Calculate combined
	point.CombinedRate = point.Leg1Rate * point.Leg2Rate * point.Leg3Rate
	point.Deviation = point.CombinedRate - 1.0

	return point, nil
}

func calculateRateFromQuote(quote *models.SwapQuote) float64 {
	// For strict send: rate = expected_amount / amount
	amount, _ := strconv.ParseFloat(quote.Amount, 64)
	expected, _ := strconv.ParseFloat(quote.ExpectedAmount, 64)
	if amount == 0 {
		return 0
	}
	return expected / amount
}

// RunBacktestWithRealData collects real data and runs backtest
func (r *RealDataBacktestEngine) RunBacktestWithRealData(path string, hoursBack int, threshold float64) (*RealBacktestResult, error) {
	// Parse path to get cycle
	cycle := parsePathToCycle(path)
	if len(cycle) != 4 {
		return nil, fmt.Errorf("invalid path format: %s", path)
	}

	// Calculate samples needed (1 sample every 30 seconds for the hours requested)
	samples := (hoursBack * 3600) / 30
	if samples > 200 {
		samples = 200 // Cap at 200 samples to avoid too long collection
	}
	if samples < 20 {
		samples = 20
	}

	// Collect real-time samples
	points, err := r.collectRealtimeSamples(cycle, samples, 30*time.Second)
	if err != nil {
		return nil, err
	}

	// Convert to HistoryRecord format
	records := make([]HistoryRecord, len(points))
	for i, p := range points {
		records[i] = HistoryRecord{
			Timestamp:    p.Timestamp,
			Path:         path,
			CombinedRate: p.CombinedRate,
			Deviation:    p.Deviation,
		}
	}

	result := &RealBacktestResult{
		BacktestResult: BacktestResult{
			Path:      path,
			StartTime: records[0].Timestamp,
			EndTime:   records[len(records)-1].Timestamp,
		},
		DataSource:       "horizon-realtime",
		SamplesCollected: len(records),
	}

	// Run backtest simulation
	var returns []float64
	var profits []float64
	windowSize := 20

	for i := windowSize; i < len(records); i++ {
		recentHistory := records[i-windowSize : i]
		currentRecord := records[i]

		zScore := r.stats.CalculateZScore(currentRecord.CombinedRate, recentHistory)

		if math.Abs(zScore) > threshold {
			result.TotalTrades++

			mean := r.stats.CalculateMean(recentHistory)
			expectedProfit := (mean - currentRecord.CombinedRate) * 10 // 10 XLM trade

			if expectedProfit > 0 {
				result.ProfitableTrades++
				result.TotalProfitXLM += expectedProfit
				if expectedProfit > result.MaxProfitXLM {
					result.MaxProfitXLM = expectedProfit
				}
			} else {
				result.LosingTrades++
				result.TotalProfitXLM += expectedProfit
				if expectedProfit < result.MaxLossXLM {
					result.MaxLossXLM = expectedProfit
				}
			}

			returns = append(returns, expectedProfit)
			profits = append(profits, result.TotalProfitXLM)
		}
	}

	// Calculate metrics
	result.DataPointsUsed = len(records)
	if result.TotalTrades > 0 {
		result.WinRate = float64(result.ProfitableTrades) / float64(result.TotalTrades) * 100
		result.AvgProfitPerTrade = result.TotalProfitXLM / float64(result.TotalTrades)
	}

	if len(returns) > 0 {
		result.SharpeRatio = r.stats.CalculateSharpeRatio(returns, 0.0)
		result.MaxDrawdown = r.stats.CalculateMaxDrawdown(profits)
	}

	result.ActualDateRange = fmt.Sprintf("%s to %s (%v elapsed)",
		result.StartTime.Format("15:04:05"),
		result.EndTime.Format("15:04:05"),
		result.EndTime.Sub(result.StartTime).Round(time.Second))

	return result, nil
}

// parsePathToCycle converts path string like "XLM→USDC→yXLM→XLM" to cycle array
func parsePathToCycle(path string) []string {
	// Simple parser for paths in format "XLM→USDC→yXLM→XLM"
	var cycle []string
	current := ""
	for _, r := range path {
		if r == '→' || r == '-' || r == '>' {
			if current != "" {
				cycle = append(cycle, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		cycle = append(cycle, current)
	}
	return cycle
}

// DisplayRealBacktest formats real data backtest results
func DisplayRealBacktest(result *RealBacktestResult) string {
	output := fmt.Sprintf("\n  Backtest Results: %s (Real Horizon Data)\n", result.Path)
	output += "  ─────────────────────────────────────────\n\n"

	output += fmt.Sprintf("  Data Source:       %s\n", result.DataSource)
	output += fmt.Sprintf("  Collection Time:   %s\n", result.ActualDateRange)
	output += fmt.Sprintf("  Samples Collected: %d\n", result.SamplesCollected)
	output += fmt.Sprintf("  Data Points:       %d\n\n", result.DataPointsUsed)

	output += fmt.Sprintf("  Total Trades:       %d\n", result.TotalTrades)
	output += fmt.Sprintf("  Profitable:         %d (%.1f%%)\n",
		result.ProfitableTrades, result.WinRate)
	output += fmt.Sprintf("  Losing:             %d\n", result.LosingTrades)
	output += fmt.Sprintf("  Total Profit:       %.6f XLM\n", result.TotalProfitXLM)
	output += fmt.Sprintf("  Avg per Trade:      %.6f XLM\n", result.AvgProfitPerTrade)
	output += fmt.Sprintf("  Max Profit:         %.6f XLM\n", result.MaxProfitXLM)
	output += fmt.Sprintf("  Max Loss:           %.6f XLM\n", result.MaxLossXLM)
	output += fmt.Sprintf("  Sharpe Ratio:       %.2f\n", result.SharpeRatio)
	output += fmt.Sprintf("  Max Drawdown:       %.2f%%\n", result.MaxDrawdown*100)

	output += "\n  Interpretation:\n"
	if result.SharpeRatio > 1.0 {
		output += "  • Good risk-adjusted returns\n"
	} else if result.SharpeRatio > 0.5 {
		output += "  • Moderate risk-adjusted returns\n"
	} else {
		output += "  • Poor risk-adjusted returns\n"
	}

	if result.WinRate > 60 {
		output += "  • High win rate - strategy is reliable\n"
	} else if result.WinRate > 50 {
		output += "  • Positive edge but high variance\n"
	} else {
		output += "  • Low win rate - consider higher thresholds\n"
	}

	output += "  • Based on real-time Horizon path quotes\n"
	output += "  • Data collected live from Stellar DEX\n"

	return output
}
