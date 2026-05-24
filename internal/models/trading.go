package models

import (
	"time"
)

// Trading Strategy Types
// ─────────────────────────────────────────────

type StrategyType string

const (
	StrategyArbitrage      StrategyType = "arbitrage"
	StrategyMeanReversion  StrategyType = "mean_reversion"
	StrategyMomentum       StrategyType = "momentum"
	StrategyGridTrading    StrategyType = "grid_trading"
	StrategyDCA            StrategyType = "dca"
	StrategyBreakout       StrategyType = "breakout"
	StrategyScalping       StrategyType = "scalping"
)

// StrategyStatus represents the current state of a trading strategy
type StrategyStatus string

const (
	StrategyActive    StrategyStatus = "active"
	StrategyPaused    StrategyStatus = "paused"
	StrategyStopped   StrategyStatus = "stopped"
	StrategyError     StrategyStatus = "error"
)

// TradingStrategy represents a configured trading strategy
type TradingStrategy struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        StrategyType           `json:"type"`
	Description string                 `json:"description"`
	Status      StrategyStatus         `json:"status"`
	Network     Network                `json:"network"`
	BaseAsset   string                 `json:"baseAsset"`
	QuoteAsset  string                 `json:"quoteAsset"`
	Parameters  map[string]interface{} `json:"parameters"`
	RiskLimits  RiskLimits             `json:"riskLimits"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	LastRunAt   *time.Time             `json:"lastRunAt,omitempty"`
	TotalTrades int                    `json:"totalTrades"`
	TotalProfit float64                `json:"totalProfit"`
	ActiveSince *time.Time             `json:"activeSince,omitempty"`
}

// RiskLimits defines risk management parameters
type RiskLimits struct {
	MaxPositionSize   float64 `json:"maxPositionSize"`   // Maximum position in base asset
	MaxDailyLoss      float64 `json:"maxDailyLoss"`      // Maximum daily loss in quote asset
	MaxDrawdown       float64 `json:"maxDrawdown"`       // Maximum drawdown percentage
	StopLossPercent   float64 `json:"stopLossPercent"`   // Stop loss percentage
	TakeProfitPercent float64 `json:"takeProfitPercent"` // Take profit percentage
	MaxOpenTrades     int     `json:"maxOpenTrades"`     // Maximum concurrent trades
}

// DefaultRiskLimits returns conservative risk limits
func DefaultRiskLimits() RiskLimits {
	return RiskLimits{
		MaxPositionSize:   100.0,  // 100 units of base asset
		MaxDailyLoss:      50.0,   // 50 XLM max daily loss
		MaxDrawdown:       10.0,   // 10% max drawdown
		StopLossPercent:   2.0,    // 2% stop loss
		TakeProfitPercent: 3.0,    // 3% take profit
		MaxOpenTrades:     3,      // Max 3 concurrent trades
	}
}

// StrategyExecution represents a single strategy execution/trade
type StrategyExecution struct {
	ID           string         `json:"id"`
	StrategyID   string         `json:"strategyId"`
	StrategyType StrategyType   `json:"strategyType"`
	Timestamp    time.Time      `json:"timestamp"`
	Action       TradeAction    `json:"action"`
	BaseAsset    string         `json:"baseAsset"`
	QuoteAsset   string         `json:"quoteAsset"`
	Amount       float64        `json:"amount"`
	Price        float64        `json:"price"`
	Value        float64        `json:"value"`
	ProfitLoss   float64        `json:"profitLoss"`
	ProfitPct    float64        `json:"profitPct"`
	TxHash       string         `json:"txHash,omitempty"`
	Status       ExecutionStatus `json:"status"`
	Error        string         `json:"error,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type TradeAction string

const (
	TradeBuy   TradeAction = "buy"
	TradeSell  TradeAction = "sell"
	TradeHold  TradeAction = "hold"
	TradeEnter TradeAction = "enter"
	TradeExit  TradeAction = "exit"
)

type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "pending"
	ExecutionExecuted  ExecutionStatus = "executed"
	ExecutionFailed  ExecutionStatus = "failed"
	ExecutionSkipped ExecutionStatus = "skipped"
)

// StrategyPerformance tracks strategy performance metrics
type StrategyPerformance struct {
	StrategyID        string    `json:"strategyId"`
	TotalTrades       int       `json:"totalTrades"`
	WinningTrades     int       `json:"winningTrades"`
	LosingTrades      int       `json:"losingTrades"`
	WinRate           float64   `json:"winRate"`
	AvgProfit         float64   `json:"avgProfit"`
	AvgLoss           float64   `json:"avgLoss"`
	ProfitFactor      float64   `json:"profitFactor"`
	SharpeRatio       float64   `json:"sharpeRatio"`
	MaxDrawdown       float64   `json:"maxDrawdown"`
	TotalReturn       float64   `json:"totalReturn"`
	DailyReturns      []float64 `json:"dailyReturns"`
	LastUpdated       time.Time `json:"lastUpdated"`
}

// MarketData represents price and order book data
type MarketData struct {
	BaseAsset     string    `json:"baseAsset"`
	QuoteAsset    string    `json:"quoteAsset"`
	Price         float64   `json:"price"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	Spread        float64   `json:"spread"`
	SpreadPct     float64   `json:"spreadPct"`
	Volume24h     float64   `json:"volume24h"`
	Change24h     float64   `json:"change24h"`
	ChangePct24h  float64   `json:"changePct24h"`
	High24h       float64   `json:"high24h"`
	Low24h        float64   `json:"low24h"`
	Timestamp     time.Time `json:"timestamp"`
	OrderBookDepth int      `json:"orderBookDepth"`
}

// PriceHistory represents historical price data
type PriceHistory struct {
	BaseAsset  string      `json:"baseAsset"`
	QuoteAsset string      `json:"quoteAsset"`
	Interval   string      `json:"interval"` // 1m, 5m, 15m, 1h, 4h, 1d
	Candles    []Candle    `json:"candles"`
}

type Candle struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// StrategySignal represents a trading signal from a strategy
type StrategySignal struct {
	StrategyID   string                 `json:"strategyId"`
	Timestamp    time.Time              `json:"timestamp"`
	Action       TradeAction            `json:"action"`
	Confidence   float64                `json:"confidence"` // 0.0 to 1.0
	Reason       string                 `json:"reason"`
	Price        float64                `json:"price"`
	Amount       float64                `json:"amount"`
	StopLoss     float64                `json:"stopLoss,omitempty"`
	TakeProfit   float64                `json:"takeProfit,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// GridLevel represents a single level in a grid trading strategy
type GridLevel struct {
	Level        int     `json:"level"`
	Price        float64 `json:"price"`
	BuyTriggered bool    `json:"buyTriggered"`
	SellTriggered bool   `json:"sellTriggered"`
	Executed     bool    `json:"executed"`
}

// GridState tracks the state of a grid trading strategy
type GridState struct {
	Levels       []GridLevel `json:"levels"`
	UpperPrice   float64     `json:"upperPrice"`
	LowerPrice   float64     `json:"lowerPrice"`
	GridSpacing  float64     `json:"gridSpacing"`
	NumGrids     int         `json:"numGrids"`
	CurrentLevel int         `json:"currentLevel"`
}
