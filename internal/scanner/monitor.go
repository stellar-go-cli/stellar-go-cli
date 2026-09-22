package scanner

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/swap"
)

// MonitorConfig holds configuration for continuous monitoring
type MonitorConfig struct {
	Interval     time.Duration
	MetricsPort  int
	Network      string
	TestAmount   string
	MinProfit    float64
	ScanAllPairs bool
	AutoExecute  bool // Prompt to execute profitable opportunities
}

// MonitorService handles continuous arbitrage monitoring
type MonitorService struct {
	config MonitorConfig
	svc    *Service
}

// NewMonitorService creates a new monitor service
func NewMonitorService(config MonitorConfig) *MonitorService {
	return &MonitorService{
		config: config,
		svc:    NewService(models.Network(config.Network), config.MinProfit, config.TestAmount),
	}
}

// Run starts continuous monitoring
func (m *MonitorService) Run(ctx context.Context) error {
	// Start metrics server (already listening internally)
	server, serverErr, err := m.svc.StartMetricsServerWithChannel(m.config.MetricsPort)
	if err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	// Start monitoring loop
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	fmt.Printf("🔍 Starting continuous arbitrage monitoring every %v\n", m.config.Interval)
	fmt.Printf("📊 Metrics at http://localhost:%d/metrics\n", m.config.MetricsPort)
	fmt.Printf("🔄 Press Ctrl+C to stop\n\n")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n🛑 Shutting down monitor...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(shutdownCtx)
			return ctx.Err()

		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("metrics server error: %w", err)
			}

		case <-ticker.C:
			profitable := m.performScan()
			if profitable != nil && m.config.AutoExecute {
				m.promptAndExecute(profitable)
			}
		}
	}
}

func (m *MonitorService) performScan() *ScanResult {
	start := time.Now()

	results, err := m.svc.ScanAll()
	if err != nil {
		fmt.Printf("❌ Scan failed: %v\n", err)
		return nil
	}

	profitable := 0
	var bestOpportunity *ScanResult
	for i, r := range results {
		if r.IsProfitable {
			profitable++
			if bestOpportunity == nil || r.NetProfitXLM > bestOpportunity.NetProfitXLM {
				bestOpportunity = &results[i]
			}
		}
	}

	duration := time.Since(start)
	fmt.Printf("[%s] Scan complete: %d pairs, %d profitable, took %v\n",
		time.Now().Format("15:04:05"), len(results), profitable, duration)

	// Log profitable opportunities
	for _, r := range results {
		if r.IsProfitable {
			fmt.Printf("💰 %s: +%.7f XLM (%.3f%%)\n", r.PairName, r.NetProfitXLM, r.SpreadPercent)
		}
	}

	return bestOpportunity
}

func (m *MonitorService) promptAndExecute(result *ScanResult) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("  🚨 PROFITABLE ARBITRAGE DETECTED\n")
	fmt.Printf("  Pair: %s\n", result.PairName)
	fmt.Printf("  Expected Profit: +%.7f XLM (%.3f%%)\n", result.NetProfitXLM, result.SpreadPercent)
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Stop the ticker temporarily by using stdin prompt
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Execute this arbitrage? [y/N]: ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("⏭️  Skipped execution")
		return
	}

	// Initialize swap service
	swapSvc := swap.NewService(models.Network(m.config.Network))
	_, keyErr := swapSvc.LoadStellarKeypair()
	if keyErr != nil {
		fmt.Printf("❌ Cannot execute: no Stellar keypair configured\n")
		return
	}

	// Execute Leg A
	fmt.Printf("\n📤 Executing Leg A: %s → %s\n", result.BaseAsset, result.QuoteAsset)
	reqA := models.SwapRequest{
		SourceAsset: result.BaseAsset,
		DestAsset:   result.QuoteAsset,
		Amount:      result.TestAmount,
		SwapType:    models.SwapStrictSend,
		MaxSlippage: 1.0,
	}

	quoteA, err := swapSvc.GetQuote(reqA)
	if err != nil {
		fmt.Printf("❌ Leg A quote failed: %v\n", err)
		return
	}

	paymentA, err := swapSvc.ExecuteSwap(quoteA, 1.0, "")
	if err != nil {
		fmt.Printf("❌ Leg A execution failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Leg A complete - received %s %s\n", quoteA.ExpectedAmount, result.QuoteAsset)
	fmt.Printf("   TX: %s\n", paymentA.TxHash)

	// Execute Leg B
	fmt.Printf("\n📤 Executing Leg B: %s → %s\n", result.QuoteAsset, result.BaseAsset)
	reqB := models.SwapRequest{
		SourceAsset: result.QuoteAsset,
		DestAsset:   result.BaseAsset,
		Amount:      quoteA.ExpectedAmount,
		SwapType:    models.SwapStrictSend,
		MaxSlippage: 1.0,
	}

	quoteB, err := swapSvc.GetQuote(reqB)
	if err != nil {
		fmt.Printf("❌ Leg B quote failed: %v\n", err)
		return
	}

	paymentB, err := swapSvc.ExecuteSwap(quoteB, 1.0, "")
	if err != nil {
		fmt.Printf("❌ Leg B execution failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Leg B complete - received %s %s\n", quoteB.ExpectedAmount, result.BaseAsset)
	fmt.Printf("   TX: %s\n", paymentB.TxHash)

	// Calculate actual profit
	finalAmount, _ := strconv.ParseFloat(quoteB.ExpectedAmount, 64)
	startAmount, _ := strconv.ParseFloat(result.TestAmount, 64)
	actualProfit := finalAmount - startAmount

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	if actualProfit > 0 {
		fmt.Printf("  ✅ ARBITRAGE COMPLETE - PROFIT: +%.7f %s\n", actualProfit, result.BaseAsset)
	} else {
		fmt.Printf("  ⚠️  ARBITRAGE COMPLETE - LOSS: %.7f %s\n", actualProfit, result.BaseAsset)
	}
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
}
