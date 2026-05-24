package triangular

import (
	"math"
)

// StatsCalculator provides statistical analysis for triangular arbitrage
type StatsCalculator struct {
	window int
}

// NewStatsCalculator creates a new stats calculator with given window
func NewStatsCalculator(window int) *StatsCalculator {
	if window < 10 {
		window = 20 // Default window
	}
	return &StatsCalculator{window: window}
}

// CalculateZScore computes the z-score for a given rate against historical data
func (s *StatsCalculator) CalculateZScore(currentRate float64, history []HistoryRecord) float64 {
	if len(history) < 2 {
		return 0
	}
	
	// Calculate mean
	sum := 0.0
	for _, h := range history {
		sum += h.CombinedRate
	}
	mean := sum / float64(len(history))
	
	// Calculate standard deviation
	sumSqDiff := 0.0
	for _, h := range history {
		diff := h.CombinedRate - mean
		sumSqDiff += diff * diff
	}
	stdDev := math.Sqrt(sumSqDiff / float64(len(history)))
	
	if stdDev == 0 {
		return 0
	}
	
	zScore := (currentRate - mean) / stdDev
	return zScore
}

// CalculateVolatility computes the standard deviation of rates
func (s *StatsCalculator) CalculateVolatility(history []HistoryRecord) float64 {
	if len(history) < 2 {
		return 0
	}
	
	sum := 0.0
	for _, h := range history {
		sum += h.CombinedRate
	}
	mean := sum / float64(len(history))
	
	sumSqDiff := 0.0
	for _, h := range history {
		diff := h.CombinedRate - mean
		sumSqDiff += diff * diff
	}
	
	return math.Sqrt(sumSqDiff / float64(len(history)))
}

// CalculateMean computes the average combined rate
func (s *StatsCalculator) CalculateMean(history []HistoryRecord) float64 {
	if len(history) == 0 {
		return 1.0 // Default to efficient market
	}
	
	sum := 0.0
	for _, h := range history {
		sum += h.CombinedRate
	}
	return sum / float64(len(history))
}

// DetectMeanReversion checks if current rate deviates significantly from mean
// Returns true if z-score exceeds threshold (indicating potential reversion)
func (s *StatsCalculator) DetectMeanReversion(currentRate float64, history []HistoryRecord, threshold float64) (bool, float64) {
	zScore := s.CalculateZScore(currentRate, history)
	return math.Abs(zScore) > threshold, zScore
}

// CalculateSharpeRatio computes risk-adjusted returns for backtesting
func (s *StatsCalculator) CalculateSharpeRatio(returns []float64, riskFreeRate float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	
	// Calculate average return
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	avgReturn := sum / float64(len(returns))
	
	// Calculate volatility (std dev of returns)
	sumSqDiff := 0.0
	for _, r := range returns {
		diff := r - avgReturn
		sumSqDiff += diff * diff
	}
	stdDev := math.Sqrt(sumSqDiff / float64(len(returns)))
	
	if stdDev == 0 {
		return 0
	}
	
	return (avgReturn - riskFreeRate) / stdDev
}

// CalculateMaxDrawdown computes the maximum drawdown from a series of profits
func (s *StatsCalculator) CalculateMaxDrawdown(profits []float64) float64 {
	if len(profits) == 0 {
		return 0
	}
	
	maxDrawdown := 0.0
	peak := profits[0]
	
	for _, profit := range profits {
		if profit > peak {
			peak = profit
		}
		
		drawdown := (peak - profit) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}
	
	return maxDrawdown
}

// OpportunityScore calculates a 0-100 score based on multiple factors
func (s *StatsCalculator) OpportunityScore(result TriangularResult, history []HistoryRecord) int {
	score := 0
	
	// Z-score component (0-40 points)
	zScore := s.CalculateZScore(result.Path.CombinedRate, history)
	zScoreAbs := math.Abs(zScore)
	if zScoreAbs > 2.0 {
		score += 40
	} else if zScoreAbs > 1.5 {
		score += 30
	} else if zScoreAbs > 1.0 {
		score += 20
	} else if zScoreAbs > 0.5 {
		score += 10
	}
	
	// Profit component (0-30 points)
	if result.ProfitPercent > 0.2 {
		score += 30
	} else if result.ProfitPercent > 0.1 {
		score += 20
	} else if result.ProfitPercent > 0.05 {
		score += 10
	}
	
	// Volatility component (0-30 points) - lower volatility = higher score
	volatility := s.CalculateVolatility(history)
	if volatility < 0.001 {
		score += 30
	} else if volatility < 0.005 {
		score += 20
	} else if volatility < 0.01 {
		score += 10
	}
	
	return score
}
