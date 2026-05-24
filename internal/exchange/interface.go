package exchange

import (
	"fmt"
	"time"
)

// ExchangeType represents the type of exchange
type ExchangeType string

const (
	ExchangeTypeStellar ExchangeType = "stellar"
)

// OrderSide represents buy or sell
type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

// OrderType represents the type of order
type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
)

// OrderStatus represents the status of an order
type OrderStatus string

const (
	OrderStatusOpen     OrderStatus = "open"
	OrderStatusClosed   OrderStatus = "closed"
	OrderStatusCanceled OrderStatus = "canceled"
	OrderStatusPending  OrderStatus = "pending"
	OrderStatusRejected OrderStatus = "rejected"
)

// Ticker represents market ticker data
type Ticker struct {
	Symbol    string    `json:"symbol"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Last      float64   `json:"last"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Volume    float64   `json:"volume"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"changePct"`
	Timestamp time.Time `json:"timestamp"`
}

// Candle represents OHLCV data
type Candle struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
}

// OrderBook represents the order book
type OrderBook struct {
	Symbol    string           `json:"symbol"`
	Bids      []OrderBookEntry `json:"bids"`
	Asks      []OrderBookEntry `json:"asks"`
	Timestamp time.Time        `json:"timestamp"`
}

// OrderBookEntry represents a single entry in the order book
type OrderBookEntry struct {
	Price  float64 `json:"price"`
	Amount float64 `json:"amount"`
}

// Order represents a trading order
type Order struct {
	ID        string      `json:"id"`
	Symbol    string      `json:"symbol"`
	Side      OrderSide   `json:"side"`
	Type      OrderType   `json:"type"`
	Amount    float64     `json:"amount"`
	Price     float64     `json:"price,omitempty"`
	Status    OrderStatus `json:"status"`
	Filled    float64     `json:"filled"`
	Remaining float64     `json:"remaining"`
	Cost      float64     `json:"cost"`
	Fee       float64     `json:"fee"`
	Timestamp time.Time   `json:"timestamp"`
}

// Balance represents account balance for a specific asset
type Balance struct {
	Asset string  `json:"asset"`
	Free  float64 `json:"free"`
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
}

// Exchange defines the unified interface for all exchanges
type Exchange interface {
	// Market Data
	FetchTicker(symbol string) (*Ticker, error)
	FetchOHLCV(symbol, timeframe string, limit int) ([]Candle, error)
	FetchOrderBook(symbol string, limit int) (*OrderBook, error)

	// Trading
	CreateOrder(symbol string, side OrderSide, orderType OrderType, amount, price float64) (*Order, error)
	CancelOrder(orderID string) error
	FetchOrder(orderID string) (*Order, error)
	FetchOpenOrders(symbol string) ([]Order, error)

	// Account
	FetchBalance() (map[string]Balance, error)

	// Metadata
	Name() string
	Type() ExchangeType
	SupportsSymbol(symbol string) bool
	GetSymbols() []string

	// Connection
	Connect() error
	IsConnected() bool
	Disconnect() error
}

// ExchangeError represents an error from an exchange
type ExchangeError struct {
	Exchange string `json:"exchange"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Op       string `json:"operation"`
}

func (e *ExchangeError) Error() string {
	return fmt.Sprintf("%s error in %s: %s (code: %s)", e.Exchange, e.Op, e.Message, e.Code)
}

// NewExchangeError creates a new exchange error
func NewExchangeError(exchange, op, code, message string) *ExchangeError {
	return &ExchangeError{
		Exchange: exchange,
		Op:       op,
		Code:     code,
		Message:  message,
	}
}
