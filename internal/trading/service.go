package trading

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/swap"
)

// Service handles trading strategy execution and management
type Service struct {
	network     models.Network
	swapSvc     *swap.Service
	strategies  map[string]*models.TradingStrategy
	executions  []models.StrategyExecution
	performance map[string]*models.StrategyPerformance
	mu          sync.RWMutex
	stopCh      chan struct{}
}

// NewService creates a new trading service
func NewService(network models.Network) *Service {
	return &Service{
		network:     network,
		swapSvc:     swap.NewService(network),
		strategies:  make(map[string]*models.TradingStrategy),
		executions:  make([]models.StrategyExecution, 0),
		performance: make(map[string]*models.StrategyPerformance),
		stopCh:      make(chan struct{}),
	}
}

// CreateStrategy creates and configures a new trading strategy
func (s *Service) CreateStrategy(
	name string,
	strategyType models.StrategyType,
	baseAsset, quoteAsset string,
	params map[string]interface{},
	riskLimits models.RiskLimits,
) (*models.TradingStrategy, error) {
	strategy := &models.TradingStrategy{
		ID:          generateStrategyID(),
		Name:        name,
		Type:        strategyType,
		Network:     s.network,
		BaseAsset:   baseAsset,
		QuoteAsset:  quoteAsset,
		Parameters:  params,
		RiskLimits:  riskLimits,
		Status:      models.StrategyStopped,
		TotalTrades: 0,
		TotalProfit: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Validate parameters based on strategy type
	if err := s.validateStrategyParams(strategyType, params); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	s.mu.Lock()
	s.strategies[strategy.ID] = strategy
	s.performance[strategy.ID] = &models.StrategyPerformance{
		StrategyID:  strategy.ID,
		LastUpdated: time.Now(),
	}
	s.mu.Unlock()

	return strategy, nil
}

// StartStrategy activates a trading strategy
func (s *Service) StartStrategy(strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if strategy.Status == models.StrategyActive {
		return fmt.Errorf("strategy already active")
	}

	now := time.Now()
	strategy.Status = models.StrategyActive
	strategy.ActiveSince = &now
	strategy.UpdatedAt = now

	// Start strategy execution loop based on type
	go s.runStrategy(strategy)

	return nil
}

// StopStrategy deactivates a trading strategy
func (s *Service) StopStrategy(strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	strategy.Status = models.StrategyStopped
	strategy.UpdatedAt = time.Now()
	strategy.ActiveSince = nil

	return nil
}

// PauseStrategy temporarily pauses a strategy
func (s *Service) PauseStrategy(strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if strategy.Status != models.StrategyActive {
		return fmt.Errorf("strategy not active")
	}

	strategy.Status = models.StrategyPaused
	strategy.UpdatedAt = time.Now()

	return nil
}

// GetStrategy retrieves a strategy by ID
func (s *Service) GetStrategy(strategyID string) (*models.TradingStrategy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyID)
	}

	return strategy, nil
}

// GetAllStrategies returns all configured strategies
func (s *Service) GetAllStrategies() []*models.TradingStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.TradingStrategy, 0, len(s.strategies))
	for _, strategy := range s.strategies {
		result = append(result, strategy)
	}

	return result
}

// GetActiveStrategies returns currently active strategies
func (s *Service) GetActiveStrategies() []*models.TradingStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.TradingStrategy, 0)
	for _, strategy := range s.strategies {
		if strategy.Status == models.StrategyActive {
			result = append(result, strategy)
		}
	}

	return result
}

// GetPerformance returns performance metrics for a strategy
func (s *Service) GetPerformance(strategyID string) (*models.StrategyPerformance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	perf, exists := s.performance[strategyID]
	if !exists {
		return nil, fmt.Errorf("no performance data for strategy %s", strategyID)
	}

	return perf, nil
}

// GetExecutions returns execution history for a strategy
func (s *Service) GetExecutions(strategyID string, limit int) []models.StrategyExecution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]models.StrategyExecution, 0)
	count := 0

	// Iterate in reverse to get latest first
	for i := len(s.executions) - 1; i >= 0; i-- {
		if s.executions[i].StrategyID == strategyID {
			result = append(result, s.executions[i])
			count++
			if limit > 0 && count >= limit {
				break
			}
		}
	}

	return result
}

// DeleteStrategy removes a strategy
func (s *Service) DeleteStrategy(strategyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy, exists := s.strategies[strategyID]
	if !exists {
		return fmt.Errorf("strategy %s not found", strategyID)
	}

	if strategy.Status == models.StrategyActive {
		return fmt.Errorf("cannot delete active strategy, stop it first")
	}

	delete(s.strategies, strategyID)
	delete(s.performance, strategyID)

	return nil
}

// runStrategy executes the main strategy loop
func (s *Service) runStrategy(strategy *models.TradingStrategy) {
	ticker := time.NewTicker(getStrategyInterval(strategy.Type))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.RLock()
			currentStatus := strategy.Status
			s.mu.RUnlock()

			if currentStatus != models.StrategyActive {
				return
			}

			s.executeStrategyIteration(strategy)

		case <-s.stopCh:
			return
		}
	}
}

// executeStrategyIteration runs a single iteration of a strategy
func (s *Service) executeStrategyIteration(strategy *models.TradingStrategy) {
	defer func() {
		if r := recover(); r != nil {
			s.mu.Lock()
			strategy.Status = models.StrategyError
			s.mu.Unlock()
		}
	}()

	var signal *models.StrategySignal

	switch strategy.Type {
	case models.StrategyArbitrage:
		signal = s.executeArbitrageStrategy(strategy)
	case models.StrategyMeanReversion:
		signal = s.executeMeanReversionStrategy(strategy)
	case models.StrategyMomentum:
		signal = s.executeMomentumStrategy(strategy)
	case models.StrategyGridTrading:
		signal = s.executeGridStrategy(strategy)
	case models.StrategyDCA:
		signal = s.executeDCAStrategy(strategy)
	case models.StrategyBreakout:
		signal = s.executeBreakoutStrategy(strategy)
	case models.StrategyScalping:
		signal = s.executeScalpingStrategy(strategy)
	}

	if signal != nil && signal.Action != models.TradeHold {
		s.executeSignal(strategy, signal)
	}

	s.mu.Lock()
	now := time.Now()
	strategy.LastRunAt = &now
	s.mu.Unlock()
}

// executeSignal processes a trading signal
func (s *Service) executeSignal(strategy *models.TradingStrategy, signal *models.StrategySignal) {
	execution := models.StrategyExecution{
		ID:           generateExecutionID(),
		StrategyID:   strategy.ID,
		StrategyType: strategy.Type,
		Timestamp:    time.Now(),
		Action:       signal.Action,
		BaseAsset:    strategy.BaseAsset,
		QuoteAsset:   strategy.QuoteAsset,
		Amount:       signal.Amount,
		Price:        signal.Price,
		Value:        signal.Amount * signal.Price,
		Status:       models.ExecutionPending,
		Metadata:     signal.Metadata,
	}

	// TODO: Execute actual trade via swap service
	// For now, mark as executed (simulation mode)
	execution.Status = models.ExecutionExecuted
	execution.TxHash = "simulated"

	// Calculate P&L if closing position
	if signal.Action == models.TradeSell || signal.Action == models.TradeExit {
		execution.ProfitLoss = s.calculateProfitLoss(strategy, signal)
		execution.ProfitPct = (execution.ProfitLoss / execution.Value) * 100
	}

	s.mu.Lock()
	s.executions = append(s.executions, execution)
	strategy.TotalTrades++
	strategy.TotalProfit += execution.ProfitLoss

	// Update performance metrics
	perf := s.performance[strategy.ID]
	perf.TotalTrades = strategy.TotalTrades
	if execution.ProfitLoss > 0 {
		perf.WinningTrades++
	} else if execution.ProfitLoss < 0 {
		perf.LosingTrades++
	}
	if perf.TotalTrades > 0 {
		perf.WinRate = float64(perf.WinningTrades) / float64(perf.TotalTrades) * 100
	}
	perf.TotalReturn = strategy.TotalProfit
	perf.LastUpdated = time.Now()
	s.mu.Unlock()
}

// calculateProfitLoss calculates P&L for a closing trade
func (s *Service) calculateProfitLoss(strategy *models.TradingStrategy, signal *models.StrategySignal) float64 {
	// Get previous buy executions for this strategy
	var totalBuyValue, totalBuyAmount float64

	s.mu.RLock()
	for i := len(s.executions) - 1; i >= 0; i-- {
		exec := s.executions[i]
		if exec.StrategyID == strategy.ID && exec.Action == models.TradeBuy {
			totalBuyValue += exec.Value
			totalBuyAmount += exec.Amount
		}
	}
	s.mu.RUnlock()

	if totalBuyAmount == 0 {
		return 0
	}

	avgBuyPrice := totalBuyValue / totalBuyAmount
	sellValue := signal.Amount * signal.Price
	buyValue := signal.Amount * avgBuyPrice

	return sellValue - buyValue
}

// validateStrategyParams validates parameters for each strategy type
func (s *Service) validateStrategyParams(strategyType models.StrategyType, params map[string]interface{}) error {
	switch strategyType {
	case models.StrategyGridTrading:
		if _, ok := params["upper_price"]; !ok {
			return fmt.Errorf("grid trading requires upper_price parameter")
		}
		if _, ok := params["lower_price"]; !ok {
			return fmt.Errorf("grid trading requires lower_price parameter")
		}
		if _, ok := params["num_grids"]; !ok {
			return fmt.Errorf("grid trading requires num_grids parameter")
		}
	case models.StrategyDCA:
		if _, ok := params["amount_per_order"]; !ok {
			return fmt.Errorf("DCA requires amount_per_order parameter")
		}
		if _, ok := params["interval_hours"]; !ok {
			return fmt.Errorf("DCA requires interval_hours parameter")
		}
	case models.StrategyMeanReversion:
		if _, ok := params["lookback_periods"]; !ok {
			return fmt.Errorf("mean reversion requires lookback_periods parameter")
		}
		if _, ok := params["std_dev_threshold"]; !ok {
			return fmt.Errorf("mean reversion requires std_dev_threshold parameter")
		}
	case models.StrategyMomentum:
		if _, ok := params["short_ma_periods"]; !ok {
			return fmt.Errorf("momentum requires short_ma_periods parameter")
		}
		if _, ok := params["long_ma_periods"]; !ok {
			return fmt.Errorf("momentum requires long_ma_periods parameter")
		}
	case models.StrategyScalping:
		// RSI period is optional (defaults to 14)
		// Stochastic parameters are optional with defaults:
		// - stochastic_k_period: 14
		// - stochastic_d_period: 3
		// - stochastic_overbought: 80
		// - stochastic_oversold: 20
		if kPeriod, ok := params["stochastic_k_period"]; ok {
			if kp, ok := kPeriod.(float64); !ok || kp < 5 || kp > 50 {
				return fmt.Errorf("stochastic_k_period must be a number between 5 and 50")
			}
		}
		if dPeriod, ok := params["stochastic_d_period"]; ok {
			if dp, ok := dPeriod.(float64); !ok || dp < 1 || dp > 10 {
				return fmt.Errorf("stochastic_d_period must be a number between 1 and 10")
			}
		}
	}
	return nil
}

// getStrategyInterval returns the execution interval for a strategy type
func getStrategyInterval(strategyType models.StrategyType) time.Duration {
	switch strategyType {
	case models.StrategyScalping:
		return 30 * time.Second
	case models.StrategyArbitrage:
		return 5 * time.Second
	case models.StrategyGridTrading:
		return 1 * time.Minute
	case models.StrategyMeanReversion, models.StrategyMomentum:
		return 5 * time.Minute
	case models.StrategyDCA:
		return 1 * time.Hour
	case models.StrategyBreakout:
		return 15 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// Helper functions
func generateStrategyID() string {
	return fmt.Sprintf("strategy_%d", time.Now().UnixNano())
}

func generateExecutionID() string {
	return fmt.Sprintf("exec_%d", time.Now().UnixNano())
}

// bollingerBands calculates Bollinger Bands for mean reversion
func bollingerBands(prices []float64, periods int, stdDevMultiplier float64) (upper, middle, lower float64) {
	if len(prices) < periods {
		return 0, 0, 0
	}

	// Calculate SMA (middle band)
	sum := 0.0
	for i := len(prices) - periods; i < len(prices); i++ {
		sum += prices[i]
	}
	middle = sum / float64(periods)

	// Calculate standard deviation
	varianceSum := 0.0
	for i := len(prices) - periods; i < len(prices); i++ {
		diff := prices[i] - middle
		varianceSum += diff * diff
	}
	stdDev := math.Sqrt(varianceSum / float64(periods))

	upper = middle + (stdDev * stdDevMultiplier)
	lower = middle - (stdDev * stdDevMultiplier)

	return upper, middle, lower
}

// simpleMovingAverage calculates SMA
func simpleMovingAverage(prices []float64, periods int) float64 {
	if len(prices) < periods {
		return 0
	}

	sum := 0.0
	for i := len(prices) - periods; i < len(prices); i++ {
		sum += prices[i]
	}

	return sum / float64(periods)
}

// rsi calculates Relative Strength Index
func rsi(prices []float64, periods int) float64 {
	if len(prices) < periods+1 {
		return 50 // Neutral
	}

	gains := 0.0
	losses := 0.0

	for i := len(prices) - periods; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}

	avgGain := gains / float64(periods)
	avgLoss := losses / float64(periods)

	if avgLoss == 0 {
		return 100
	}

	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// stochastic calculates Stochastic Oscillator %K and %D lines
func stochastic(prices []float64, kPeriod, dPeriod int) (k, d float64) {
	if len(prices) < kPeriod+dPeriod {
		return 50, 50 // Neutral values
	}

	// Calculate %K values
	kValues := make([]float64, dPeriod)
	for i := 0; i < dPeriod; i++ {
		startIdx := len(prices) - kPeriod - dPeriod + i
		endIdx := len(prices) - dPeriod + i

		// Find lowest low and highest high in the kPeriod window
		lowest := prices[startIdx]
		highest := prices[startIdx]
		currentClose := prices[endIdx]

		for j := startIdx; j <= endIdx; j++ {
			if prices[j] < lowest {
				lowest = prices[j]
			}
			if prices[j] > highest {
				highest = prices[j]
			}
		}

		// Calculate %K for this period
		range_ := highest - lowest
		if range_ == 0 {
			kValues[i] = 50
		} else {
			kValues[i] = ((currentClose - lowest) / range_) * 100
		}
	}

	// %K is the last calculated value
	k = kValues[len(kValues)-1]

	// %D is the SMA of %K values
	dSum := 0.0
	for _, val := range kValues {
		dSum += val
	}
	d = dSum / float64(dPeriod)

	return k, d
}
