package exchange

import (
	"fmt"
	"strconv"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/swap"
)

// StellarExchange implements the Exchange interface for Stellar network
type StellarExchange struct {
	name    string
	network models.Network
	swapSvc *swap.Service
	connected bool
}

// NewStellarExchange creates a new Stellar exchange instance
func NewStellarExchange(network models.Network) *StellarExchange {
	return &StellarExchange{
		name:    "stellar",
		network: network,
		swapSvc: swap.NewService(network),
		connected: false,
	}
}

// Name returns the exchange name
func (s *StellarExchange) Name() string {
	return s.name
}

// Type returns the exchange type
func (s *StellarExchange) Type() ExchangeType {
	return ExchangeTypeStellar
}

// Connect initializes the exchange connection
func (s *StellarExchange) Connect() error {
	s.connected = true
	return nil
}

// IsConnected returns whether the exchange is connected
func (s *StellarExchange) IsConnected() bool {
	return s.connected
}

// Disconnect closes the exchange connection
func (s *StellarExchange) Disconnect() error {
	s.connected = false
	return nil
}

// FetchTicker retrieves ticker data for a symbol pair on Stellar
// For Stellar, we use the swap service to get current prices
func (s *StellarExchange) FetchTicker(symbol string) (*Ticker, error) {
	// Parse symbol (e.g., "XLM/USDC")
	base, quote, err := parseSymbol(symbol)
	if err != nil {
		return nil, err
	}

	// Get quote for 1 unit of base asset
	req := models.SwapRequest{
		SourceAsset: base,
		DestAsset:   quote,
		Amount:      "1",
		SwapType:    models.SwapStrictSend,
	}

	quoteResult, err := s.swapSvc.GetQuote(req)
	if err != nil {
		return nil, &ExchangeError{
			Exchange: s.name,
			Op:       "FetchTicker",
			Code:     "QUOTE_FAILED",
			Message:  err.Error(),
		}
	}

	lastPrice, _ := strconv.ParseFloat(quoteResult.ExpectedAmount, 64)

	return &Ticker{
		Symbol:    symbol,
		Bid:       lastPrice * 0.999, // Simulate bid/ask spread
		Ask:       lastPrice * 1.001,
		Last:      lastPrice,
		Timestamp: time.Now(),
	}, nil
}

// FetchOHLCV retrieves OHLCV data
// Note: Stellar doesn't provide native OHLCV, so we return limited data
func (s *StellarExchange) FetchOHLCV(symbol, timeframe string, limit int) ([]Candle, error) {
	// For Stellar, we can only provide current price as a single candle
	// In a full implementation, this would use a price history service
	ticker, err := s.FetchTicker(symbol)
	if err != nil {
		return nil, err
	}

	return []Candle{{
		Timestamp: time.Now(),
		Open:      ticker.Last,
		High:      ticker.Last,
		Low:       ticker.Last,
		Close:     ticker.Last,
		Volume:    0,
	}}, nil
}

// FetchOrderBook retrieves the order book
// Stellar uses path payments, not traditional order books
func (s *StellarExchange) FetchOrderBook(symbol string, limit int) (*OrderBook, error) {
	base, quote, err := parseSymbol(symbol)
	if err != nil {
		return nil, err
	}

	// Get paths which serve as our "order book"
	req := models.SwapRequest{
		SourceAsset: base,
		DestAsset:   quote,
		Amount:      "1",
		SwapType:    models.SwapStrictSend,
	}

	quoteResult, err := s.swapSvc.GetQuote(req)
	if err != nil {
		return nil, &ExchangeError{
			Exchange: s.name,
			Op:       "FetchOrderBook",
			Code:     "QUOTE_FAILED",
			Message:  err.Error(),
		}
	}

	price, _ := strconv.ParseFloat(quoteResult.ExpectedAmount, 64)

	// Return a simplified order book based on the best path
	return &OrderBook{
		Symbol: symbol,
		Bids: []OrderBookEntry{{
			Price:  price * 0.999,
			Amount: 1000, // Simulated depth
		}},
		Asks: []OrderBookEntry{{
			Price:  price * 1.001,
			Amount: 1000,
		}},
		Timestamp: time.Now(),
	}, nil
}

// CreateOrder creates a new order (path payment on Stellar)
func (s *StellarExchange) CreateOrder(symbol string, side OrderSide, orderType OrderType, amount, price float64) (*Order, error) {
	base, quote, err := parseSymbol(symbol)
	if err != nil {
		return nil, err
	}

	// Map order side to Stellar assets
	var sourceAsset, destAsset string
	var swapAmount string

	if side == OrderSideBuy {
		// Buying base asset with quote asset
		sourceAsset = quote
		destAsset = base
		swapAmount = fmt.Sprintf("%.7f", amount*price)
	} else {
		// Selling base asset for quote asset
		sourceAsset = base
		destAsset = quote
		swapAmount = fmt.Sprintf("%.7f", amount)
	}

	req := models.SwapRequest{
		SourceAsset: sourceAsset,
		DestAsset:   destAsset,
		Amount:      swapAmount,
		SwapType:    models.SwapStrictSend,
	}

	quoteResult, err := s.swapSvc.GetQuote(req)
	if err != nil {
		return nil, &ExchangeError{
			Exchange: s.name,
			Op:       "CreateOrder",
			Code:     "QUOTE_FAILED",
			Message:  err.Error(),
		}
	}

	// For limit orders, check if price is acceptable
	if orderType == OrderTypeLimit {
		expectedPrice, _ := strconv.ParseFloat(quoteResult.ExpectedAmount, 64)
		actualAmount, _ := strconv.ParseFloat(swapAmount, 64)
		actualPrice := expectedPrice / actualAmount

		if side == OrderSideBuy && actualPrice > price {
			return nil, &ExchangeError{
				Exchange: s.name,
				Op:       "CreateOrder",
				Code:     "PRICE_TOO_HIGH",
				Message:  fmt.Sprintf("Best available price (%.7f) exceeds limit price (%.7f)", actualPrice, price),
			}
		}
		if side == OrderSideSell && actualPrice < price {
			return nil, &ExchangeError{
				Exchange: s.name,
				Op:       "CreateOrder",
				Code:     "PRICE_TOO_LOW",
				Message:  fmt.Sprintf("Best available price (%.7f) below limit price (%.7f)", actualPrice, price),
			}
		}
	}

	// Execute the swap
	payment, err := s.swapSvc.ExecuteSwap(quoteResult, 1.0, "")
	if err != nil {
		return nil, &ExchangeError{
			Exchange: s.name,
			Op:       "CreateOrder",
			Code:     "EXECUTION_FAILED",
			Message:  err.Error(),
		}
	}

	return &Order{
		ID:        payment.TxHash,
		Symbol:    symbol,
		Side:      side,
		Type:      orderType,
		Amount:    amount,
		Price:     price,
		Status:    OrderStatusClosed,
		Filled:    amount,
		Remaining: 0,
		Timestamp: payment.CreatedAt,
	}, nil
}

// CancelOrder cancels an existing order
// For Stellar, we can't really cancel path payments (they're instant)
func (s *StellarExchange) CancelOrder(orderID string) error {
	return &ExchangeError{
		Exchange: s.name,
		Op:       "CancelOrder",
		Code:     "NOT_SUPPORTED",
		Message:  "Cannot cancel Stellar path payments - they execute immediately",
	}
}

// FetchOrder retrieves order details
func (s *StellarExchange) FetchOrder(orderID string) (*Order, error) {
	// Would need to query Horizon for transaction details
	// For now, return not found
	return nil, &ExchangeError{
		Exchange: s.name,
		Op:       "FetchOrder",
		Code:     "NOT_IMPLEMENTED",
		Message:  "Order lookup not yet implemented for Stellar",
	}
}

// FetchOpenOrders retrieves open orders
// Stellar path payments are instant, so there are no open orders
func (s *StellarExchange) FetchOpenOrders(symbol string) ([]Order, error) {
	return []Order{}, nil
}

// FetchBalance retrieves account balance
func (s *StellarExchange) FetchBalance() (map[string]Balance, error) {
	// This would need to load the Stellar account and parse balances
	// For now, return empty map
	return map[string]Balance{}, nil
}

// SupportsSymbol checks if a symbol is supported
func (s *StellarExchange) SupportsSymbol(symbol string) bool {
	// Parse and check if both assets are supported
	base, quote, err := parseSymbol(symbol)
	if err != nil {
		return false
	}

	// Check if assets are in our known asset list
	supportedAssets := map[string]bool{
		"XLM":  true,
		"USDC": true,
		"EURC": true,
		"yXLM": true,
		"XRF":  true,
	}

	return supportedAssets[base] && supportedAssets[quote]
}

// GetSymbols returns all available symbol pairs
func (s *StellarExchange) GetSymbols() []string {
	assets := []string{"XLM", "USDC", "EURC", "yXLM", "XRF"}
	symbols := []string{}

	for i, base := range assets {
		for j, quote := range assets {
			if i != j {
				symbols = append(symbols, fmt.Sprintf("%s/%s", base, quote))
			}
		}
	}

	return symbols
}

// parseSymbol parses a symbol string like "XLM/USDC" into base and quote assets
func parseSymbol(symbol string) (base, quote string, err error) {
	// Simple parsing - expects "BASE/QUOTE" format
	for i := 0; i < len(symbol); i++ {
		if symbol[i] == '/' {
			return symbol[:i], symbol[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid symbol format: %s (expected BASE/QUOTE)", symbol)
}
