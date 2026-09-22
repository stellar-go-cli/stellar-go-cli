package triangular

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// BacktestResult holds the results of a backtest simulation
type BacktestResult struct {
	Path              string
	TotalTrades       int
	ProfitableTrades  int
	LosingTrades      int
	TotalProfitXLM    float64
	AvgProfitPerTrade float64
	MaxProfitXLM      float64
	MaxLossXLM        float64
	SharpeRatio       float64
	MaxDrawdown       float64
	WinRate           float64
	DataPointsUsed    int
	StartTime         time.Time
	EndTime           time.Time
}

// BacktestEngine runs historical simulations
type BacktestEngine struct {
	history *HistoryStore
	stats   *StatsCalculator
}

// NewBacktestEngine creates a new backtesting engine
func NewBacktestEngine(history *HistoryStore) *BacktestEngine {
	return &BacktestEngine{
		history: history,
		stats:   NewStatsCalculator(20),
	}
}

// RunBacktest simulates trading on historical data
func (b *BacktestEngine) RunBacktest(path string, days int, threshold float64) (*BacktestResult, error) {
	// Get historical data
	records, err := b.history.GetHistory(path, days*100) // Assume ~100 scans per day
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	if len(records) < 20 {
		return nil, fmt.Errorf("insufficient data: need at least 20 records, got %d", len(records))
	}

	result := &BacktestResult{
		Path:           path,
		DataPointsUsed: len(records),
		StartTime:      records[len(records)-1].Timestamp,
		EndTime:        records[0].Timestamp,
	}

	// Simulate trades
	var returns []float64
	var profits []float64

	for i, record := range records {
		// Skip first 20 records to build up statistics
		if i < 20 {
			continue
		}

		// Get recent history for this point in time
		recentHistory := records[i-20 : i]

		// Calculate z-score at this point
		zScore := b.stats.CalculateZScore(record.CombinedRate, recentHistory)

		// Simulate mean reversion trade
		// If z-score > threshold, bet on reversion (rate will go down)
		// If z-score < -threshold, bet on reversion (rate will go up)
		if math.Abs(zScore) > threshold {
			result.TotalTrades++

			// Simulate profit/loss
			// Simplified: profit is proportional to deviation from mean
			mean := b.stats.CalculateMean(recentHistory)
			expectedProfit := (mean - record.CombinedRate) * 100 // Convert to XLM equivalent

			if expectedProfit > 0 {
				result.ProfitableTrades++
				result.TotalProfitXLM += expectedProfit

				if expectedProfit > result.MaxProfitXLM {
					result.MaxProfitXLM = expectedProfit
				}
			} else {
				result.LosingTrades++
				result.TotalProfitXLM += expectedProfit // Adds negative

				if expectedProfit < result.MaxLossXLM {
					result.MaxLossXLM = expectedProfit
				}
			}

			returns = append(returns, expectedProfit)
			profits = append(profits, result.TotalProfitXLM)
		}
	}

	// Calculate metrics
	if result.TotalTrades > 0 {
		result.WinRate = float64(result.ProfitableTrades) / float64(result.TotalTrades) * 100
		result.AvgProfitPerTrade = result.TotalProfitXLM / float64(result.TotalTrades)
	}

	if len(returns) > 0 {
		result.SharpeRatio = b.stats.CalculateSharpeRatio(returns, 0.0)
		result.MaxDrawdown = b.stats.CalculateMaxDrawdown(profits)
	}

	return result, nil
}

// DisplayBacktest formats backtest results for display
func DisplayBacktest(result *BacktestResult) string {
	output := fmt.Sprintf("\n  Backtest Results: %s\n", result.Path)
	output += "  ─────────────────────────────────\n\n"

	output += fmt.Sprintf("  Period: %s to %s\n",
		result.StartTime.Format("2006-01-02"),
		result.EndTime.Format("2006-01-02"))
	output += fmt.Sprintf("  Data Points: %d\n\n", result.DataPointsUsed)

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

	// Interpretation
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

	return output
}

// ComparePaths runs backtest on multiple paths and ranks them
func (b *BacktestEngine) ComparePaths(paths []string, days int, threshold float64) ([]BacktestResult, error) {
	var results []BacktestResult

	for _, path := range paths {
		result, err := b.RunBacktest(path, days, threshold)
		if err != nil {
			continue // Skip paths with insufficient data
		}
		results = append(results, *result)
	}

	// Sort by Sharpe ratio (best first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].SharpeRatio > results[j].SharpeRatio
	})

	return results, nil
}
