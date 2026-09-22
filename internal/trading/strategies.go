package trading

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
)

// executeArbitrageStrategy checks for XLM↔USDC arbitrage opportunities
func (s *Service) executeArbitrageStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	// Get round-trip quote
	amount := "5" // Default amount, could be parameterized
	if amt, ok := strategy.Parameters["amount"].(string); ok {
		amount = amt
	}

	// This would call the swap service to get real quotes
	// For now, return a simulated signal based on market conditions
	result, err := s.swapSvc.AnalyzeRoundTrip(amount, "XLM", "USDC", "")
	if err != nil {
		return nil
	}

	// Only signal if profit exceeds threshold
	minProfit := 0.001
	if mp, ok := strategy.Parameters["min_profit"].(float64); ok {
		minProfit = mp
	}

	if result.EstimatedNetXLM > minProfit {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeEnter,
			Confidence: math.Min(result.EstimatedNetXLM*100, 1.0),
			Reason:     fmt.Sprintf("Arbitrage opportunity: +%.7f XLM", result.EstimatedNetXLM),
			Price:      0, // Not applicable for round-trip
			Amount:     0,
			Metadata: map[string]interface{}{
				"leg_a":   result.LegA,
				"leg_b":   result.LegB,
				"net_xlm": result.EstimatedNetXLM,
			},
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0,
		Reason:     fmt.Sprintf("No arbitrage: %.7f XLM", result.EstimatedNetXLM),
	}
}

// executeMeanReversionStrategy uses Bollinger Bands for mean reversion
func (s *Service) executeMeanReversionStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	// Get parameters
	lookback := 20
	if lb, ok := strategy.Parameters["lookback_periods"].(float64); ok {
		lookback = int(lb)
	}

	threshold := 2.0
	if t, ok := strategy.Parameters["std_dev_threshold"].(float64); ok {
		threshold = t
	}

	// Get simulated price history (would fetch from Horizon/price feed)
	prices := s.getSimulatedPriceHistory(lookback + 10)

	if len(prices) < lookback {
		return nil
	}

	currentPrice := prices[len(prices)-1]
	upper, _, lower := bollingerBands(prices, lookback, threshold)

	// Generate signal based on price position relative to bands
	if currentPrice > upper {
		// Price above upper band - sell signal (expect reversion down)
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: math.Min((currentPrice-upper)/upper*10, 0.9),
			Reason:     fmt.Sprintf("Price %.6f above upper Bollinger Band %.6f", currentPrice, upper),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.3,
		}
	}

	if currentPrice < lower {
		// Price below lower band - buy signal (expect reversion up)
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: math.Min((lower-currentPrice)/lower*10, 0.9),
			Reason:     fmt.Sprintf("Price %.6f below lower Bollinger Band %.6f", currentPrice, lower),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.3,
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0.1,
		Reason:     fmt.Sprintf("Price %.6f within Bollinger Bands [%.6f, %.6f]", currentPrice, lower, upper),
		Price:      currentPrice,
	}
}

// executeMomentumStrategy uses moving average crossover
func (s *Service) executeMomentumStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	// Get parameters
	shortPeriod := 10
	if sp, ok := strategy.Parameters["short_ma_periods"].(float64); ok {
		shortPeriod = int(sp)
	}

	longPeriod := 30
	if lp, ok := strategy.Parameters["long_ma_periods"].(float64); ok {
		longPeriod = int(lp)
	}

	// Get simulated price history
	prices := s.getSimulatedPriceHistory(longPeriod + 10)

	if len(prices) < longPeriod {
		return nil
	}

	currentPrice := prices[len(prices)-1]
	shortMA := simpleMovingAverage(prices, shortPeriod)
	longMA := simpleMovingAverage(prices, longPeriod)
	prevShortMA := simpleMovingAverage(prices[:len(prices)-1], shortPeriod)
	prevLongMA := simpleMovingAverage(prices[:len(prices)-1], longPeriod)

	// Golden cross: short MA crosses above long MA - buy signal
	if prevShortMA <= prevLongMA && shortMA > longMA {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: 0.8,
			Reason:     fmt.Sprintf("Golden Cross: Short MA %.6f crossed above Long MA %.6f", shortMA, longMA),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.5,
		}
	}

	// Death cross: short MA crosses below long MA - sell signal
	if prevShortMA >= prevLongMA && shortMA < longMA {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: 0.8,
			Reason:     fmt.Sprintf("Death Cross: Short MA %.6f crossed below Long MA %.6f", shortMA, longMA),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.5,
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0.2,
		Reason:     fmt.Sprintf("No crossover - Short MA: %.6f, Long MA: %.6f", shortMA, longMA),
		Price:      currentPrice,
	}
}

// executeGridStrategy manages grid trading levels
func (s *Service) executeGridStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	// Get grid parameters
	upperPrice := strategy.Parameters["upper_price"].(float64)
	lowerPrice := strategy.Parameters["lower_price"].(float64)
	numGrids := int(strategy.Parameters["num_grids"].(float64))

	// Get current price
	currentPrice := s.getSimulatedPrice()

	// Calculate grid spacing
	gridSpacing := (upperPrice - lowerPrice) / float64(numGrids)

	// Find current grid level
	currentLevel := int((currentPrice - lowerPrice) / gridSpacing)
	if currentLevel < 0 {
		currentLevel = 0
	}
	if currentLevel >= numGrids {
		currentLevel = numGrids - 1
	}

	// Calculate level prices
	levelPrice := lowerPrice + float64(currentLevel)*gridSpacing
	nextLevelUp := levelPrice + gridSpacing
	nextLevelDown := levelPrice - gridSpacing

	// Generate signal based on proximity to grid lines
	// Buy when price approaches lower grid level
	if currentPrice <= nextLevelDown*1.001 {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: 0.7,
			Reason:     fmt.Sprintf("Grid Level %d: Price %.6f at/below grid line %.6f", currentLevel, currentPrice, nextLevelDown),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize / float64(numGrids),
		}
	}

	// Sell when price approaches upper grid level
	if currentPrice >= nextLevelUp*0.999 {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: 0.7,
			Reason:     fmt.Sprintf("Grid Level %d: Price %.6f at/above grid line %.6f", currentLevel, currentPrice, nextLevelUp),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize / float64(numGrids),
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0.1,
		Reason:     fmt.Sprintf("Grid Level %d: Price %.6f between %.6f and %.6f", currentLevel, currentPrice, nextLevelDown, nextLevelUp),
		Price:      currentPrice,
	}
}

// executeDCAStrategy executes Dollar Cost Averaging
func (s *Service) executeDCAStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	// DCA buys fixed amount at regular intervals
	amountPerOrder := strategy.Parameters["amount_per_order"].(float64)

	currentPrice := s.getSimulatedPrice()

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeBuy,
		Confidence: 0.5,
		Reason:     fmt.Sprintf("DCA: Regular purchase of %.2f %s at %.6f", amountPerOrder, strategy.BaseAsset, currentPrice),
		Price:      currentPrice,
		Amount:     amountPerOrder,
	}
}

// executeBreakoutStrategy detects price breakouts
func (s *Service) executeBreakoutStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	lookback := 20
	if lb, ok := strategy.Parameters["lookback_periods"].(float64); ok {
		lookback = int(lb)
	}

	prices := s.getSimulatedPriceHistory(lookback + 5)

	if len(prices) < lookback {
		return nil
	}

	currentPrice := prices[len(prices)-1]

	// Find recent high and low
	recentHigh := 0.0
	recentLow := prices[0]
	for _, p := range prices[len(prices)-lookback-1 : len(prices)-1] {
		if p > recentHigh {
			recentHigh = p
		}
		if p < recentLow {
			recentLow = p
		}
	}

	breakoutThreshold := 0.005 // 0.5% breakout threshold
	if bt, ok := strategy.Parameters["breakout_threshold"].(float64); ok {
		breakoutThreshold = bt
	}

	// Breakout above resistance
	if currentPrice > recentHigh*(1+breakoutThreshold) {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: 0.75,
			Reason:     fmt.Sprintf("Breakout above resistance: %.6f > %.6f", currentPrice, recentHigh),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.4,
		}
	}

	// Breakdown below support
	if currentPrice < recentLow*(1-breakoutThreshold) {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: 0.75,
			Reason:     fmt.Sprintf("Breakdown below support: %.6f < %.6f", currentPrice, recentLow),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.4,
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0.15,
		Reason:     fmt.Sprintf("No breakout - Price: %.6f, Support: %.6f, Resistance: %.6f", currentPrice, recentLow, recentHigh),
		Price:      currentPrice,
	}
}

// executeScalpingStrategy uses RSI and Stochastic Oscillator for short-term momentum
func (s *Service) executeScalpingStrategy(strategy *models.TradingStrategy) *models.StrategySignal {
	rsiPeriod := 14
	if rp, ok := strategy.Parameters["rsi_period"].(float64); ok {
		rsiPeriod = int(rp)
	}

	stochKPeriod := 14
	if skp, ok := strategy.Parameters["stochastic_k_period"].(float64); ok {
		stochKPeriod = int(skp)
	}

	stochDPeriod := 3
	if sdp, ok := strategy.Parameters["stochastic_d_period"].(float64); ok {
		stochDPeriod = int(sdp)
	}

	stochOverbought := 80.0
	if so, ok := strategy.Parameters["stochastic_overbought"].(float64); ok {
		stochOverbought = so
	}

	stochOversold := 20.0
	if su, ok := strategy.Parameters["stochastic_oversold"].(float64); ok {
		stochOversold = su
	}

	prices := s.getSimulatedPriceHistory(rsiPeriod + stochKPeriod + stochDPeriod + 10)

	if len(prices) < rsiPeriod || len(prices) < stochKPeriod+stochDPeriod {
		return nil
	}

	currentPrice := prices[len(prices)-1]
	rsiValue := rsi(prices, rsiPeriod)
	stochK, stochD := stochastic(prices, stochKPeriod, stochDPeriod)

	// Strong Sell: RSI overbought AND Stochastic overbought (confluence)
	if rsiValue > 70 && stochK > stochOverbought {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: math.Min((rsiValue-70)/30+(stochK-stochOverbought)/20, 0.9),
			Reason:     fmt.Sprintf("Strong Sell - RSI: %.1f (overbought), Stoch %%K: %.1f (overbought)", rsiValue, stochK),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.3,
			StopLoss:   currentPrice * 1.002,
			TakeProfit: currentPrice * 0.995,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "confluence",
			},
		}
	}

	// Strong Buy: RSI oversold AND Stochastic oversold (confluence)
	if rsiValue < 30 && stochK < stochOversold {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: math.Min((30-rsiValue)/30+(stochOversold-stochK)/20, 0.9),
			Reason:     fmt.Sprintf("Strong Buy - RSI: %.1f (oversold), Stoch %%K: %.1f (oversold)", rsiValue, stochK),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.3,
			StopLoss:   currentPrice * 0.998,
			TakeProfit: currentPrice * 1.005,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "confluence",
			},
		}
	}

	// Moderate Sell: RSI overbought (primary) or Stochastic overbought (secondary)
	if rsiValue > 70 {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: math.Min((rsiValue-70)/30, 0.7),
			Reason:     fmt.Sprintf("Sell - RSI Overbought: %.1f > 70, Stoch %%K: %.1f", rsiValue, stochK),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.25,
			StopLoss:   currentPrice * 1.002,
			TakeProfit: currentPrice * 0.995,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "rsi_primary",
			},
		}
	}

	// Moderate Buy: RSI oversold (primary) or Stochastic oversold (secondary)
	if rsiValue < 30 {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: math.Min((30-rsiValue)/30, 0.7),
			Reason:     fmt.Sprintf("Buy - RSI Oversold: %.1f < 30, Stoch %%K: %.1f", rsiValue, stochK),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.25,
			StopLoss:   currentPrice * 0.998,
			TakeProfit: currentPrice * 1.005,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "rsi_primary",
			},
		}
	}

	// Stochastic-only signals (weaker confidence)
	if stochK > stochOverbought {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeSell,
			Confidence: math.Min((stochK-stochOverbought)/20, 0.6),
			Reason:     fmt.Sprintf("Weak Sell - Stoch %%K Overbought: %.1f > %.0f (RSI: %.1f)", stochK, stochOverbought, rsiValue),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.2,
			StopLoss:   currentPrice * 1.002,
			TakeProfit: currentPrice * 0.995,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "stoch_only",
			},
		}
	}

	if stochK < stochOversold {
		return &models.StrategySignal{
			StrategyID: strategy.ID,
			Action:     models.TradeBuy,
			Confidence: math.Min((stochOversold-stochK)/20, 0.6),
			Reason:     fmt.Sprintf("Weak Buy - Stoch %%K Oversold: %.1f < %.0f (RSI: %.1f)", stochK, stochOversold, rsiValue),
			Price:      currentPrice,
			Amount:     strategy.RiskLimits.MaxPositionSize * 0.2,
			StopLoss:   currentPrice * 0.998,
			TakeProfit: currentPrice * 1.005,
			Metadata: map[string]interface{}{
				"rsi":         rsiValue,
				"stoch_k":     stochK,
				"stoch_d":     stochD,
				"signal_type": "stoch_only",
			},
		}
	}

	return &models.StrategySignal{
		StrategyID: strategy.ID,
		Action:     models.TradeHold,
		Confidence: 0.1,
		Reason:     fmt.Sprintf("Neutral - RSI: %.1f, Stoch %%K: %.1f, Stoch %%D: %.1f", rsiValue, stochK, stochD),
		Price:      currentPrice,
		Metadata: map[string]interface{}{
			"rsi":     rsiValue,
			"stoch_k": stochK,
			"stoch_d": stochD,
		},
	}
}

// getSimulatedPrice returns a simulated current price
func (s *Service) getSimulatedPrice() float64 {
	// Base price around 0.11 XLM/USDC with some randomness
	basePrice := 0.11
	noise := (rand.Float64() - 0.5) * 0.02 // ±1% noise
	return basePrice * (1 + noise)
}

// getSimulatedPriceHistory returns simulated price history
func (s *Service) getSimulatedPriceHistory(length int) []float64 {
	prices := make([]float64, length)
	basePrice := 0.11

	// Generate random walk prices
	for i := 0; i < length; i++ {
		change := (rand.Float64() - 0.5) * 0.01 // ±0.5% change
		basePrice = basePrice * (1 + change)
		prices[i] = basePrice
	}

	return prices
}
