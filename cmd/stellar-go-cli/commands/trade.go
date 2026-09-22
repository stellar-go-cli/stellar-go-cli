package commands

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/trading"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

func newTradeCmd(cfg *config.Config) *Command {
	cmd := &Command{
		Name:  "trade",
		Short: "Automated trading strategies",
		Long:  "Create, manage, and execute automated trading strategies including arbitrage, mean reversion, momentum, grid trading, DCA, and scalping.",
		cfg:   cfg,
	}
	cmd.addSub(newTradeStrategyCmd(cfg))
	cmd.addSub(newTradeListCmd(cfg))
	cmd.addSub(newTradeStartCmd(cfg))
	cmd.addSub(newTradeStopCmd(cfg))
	cmd.addSub(newTradeStatusCmd(cfg))
	cmd.addSub(newTradePerformanceCmd(cfg))
	cmd.addSub(newTradeHistoryCmd(cfg))
	cmd.Run = func(c *Command, args []string) error {
		c.printHelp()
		return nil
	}
	return cmd
}

// ─── trade strategy ─────────────────────────────

func newTradeStrategyCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("strategy", flag.ContinueOnError)
	strategyType := fs.String("type", "", "Strategy type: arbitrage, mean_reversion, momentum, grid_trading, dca, breakout, scalping")
	name := fs.String("name", "", "Strategy name")
	baseAsset := fs.String("base", "XLM", "Base asset (e.g., XLM)")
	quoteAsset := fs.String("quote", "USDC", "Quote asset (e.g., USDC)")
	network := fs.String("network", cfg.Network, "Network: stellar-testnet | stellar-mainnet")
	// Risk parameters
	maxPosition := fs.Float64("max-position", 100, "Maximum position size")
	maxDailyLoss := fs.Float64("max-daily-loss", 50, "Maximum daily loss")
	stopLoss := fs.Float64("stop-loss", 2.0, "Stop loss percentage")
	takeProfit := fs.Float64("take-profit", 3.0, "Take profit percentage")
	// Strategy-specific parameters
	params := fs.String("params", "", "Strategy parameters as key=value,key=value")

	return &Command{
		Name:  "strategy",
		Short: "Create a new trading strategy",
		Long: "Create and configure an automated trading strategy. Examples:\n\n" +
			"Grid Trading:\n" +
			"  mozartpay trade strategy --type grid_trading --name \"XLM Grid\" --params \"upper_price=0.12,lower_price=0.10,num_grids=10\"\n\n" +
			"Mean Reversion:\n" +
			"  mozartpay trade strategy --type mean_reversion --name \"Bollinger Bounce\" --params \"lookback_periods=20,std_dev_threshold=2.0\"\n\n" +
			"DCA:\n" +
			"  mozartpay trade strategy --type dca --name \"Daily DCA\" --params \"amount_per_order=10,interval_hours=24\"\n\n" +
			"Momentum:\n" +
			"  mozartpay trade strategy --type momentum --name \"MA Cross\" --params \"short_ma_periods=10,long_ma_periods=30\"\n\n" +
			"Scalping:\n" +
			"  mozartpay trade strategy --type scalping --name \"RSI Scalp\" --params \"rsi_period=14\"",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Create Trading Strategy")

			if *strategyType == "" {
				return fmt.Errorf("--type is required (arbitrage, mean_reversion, momentum, grid_trading, dca, breakout, scalping)")
			}
			if *name == "" {
				return fmt.Errorf("--name is required")
			}

			// Validate strategy type
			var strategyTypeVal models.StrategyType
			switch *strategyType {
			case "arbitrage":
				strategyTypeVal = models.StrategyArbitrage
			case "mean_reversion":
				strategyTypeVal = models.StrategyMeanReversion
			case "momentum":
				strategyTypeVal = models.StrategyMomentum
			case "grid_trading":
				strategyTypeVal = models.StrategyGridTrading
			case "dca":
				strategyTypeVal = models.StrategyDCA
			case "breakout":
				strategyTypeVal = models.StrategyBreakout
			case "scalping":
				strategyTypeVal = models.StrategyScalping
			default:
				return fmt.Errorf("invalid strategy type: %s", *strategyType)
			}

			// Parse parameters
			paramsMap := make(map[string]interface{})
			if *params != "" {
				pairs := strings.Split(*params, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						key := strings.TrimSpace(kv[0])
						val := strings.TrimSpace(kv[1])
						// Try to parse as float first
						if f, err := strconv.ParseFloat(val, 64); err == nil {
							paramsMap[key] = f
						} else {
							paramsMap[key] = val
						}
					}
				}
			}

			// Create risk limits
			riskLimits := models.RiskLimits{
				MaxPositionSize:   *maxPosition,
				MaxDailyLoss:      *maxDailyLoss,
				StopLossPercent:   *stopLoss,
				TakeProfitPercent: *takeProfit,
				MaxOpenTrades:     3,
			}

			// Create trading service
			net := models.Network(*network)
			if net == "" {
				net = models.NetworkStellarMainnet
			}
			svc := trading.NewService(net)

			// Create strategy
			spin := ui.NewSpinner("Creating strategy...")
			spin.Start()

			strategy, err := svc.CreateStrategy(*name, strategyTypeVal, *baseAsset, *quoteAsset, paramsMap, riskLimits)
			if err != nil {
				spin.Stop(false, err.Error())
				return err
			}
			spin.Stop(true, "Strategy created")

			ui.SectionLabel("Strategy Details")
			ui.KV("ID", strategy.ID)
			ui.KV("Name", strategy.Name)
			ui.KV("Type", string(strategy.Type))
			ui.KV("Pair", fmt.Sprintf("%s/%s", strategy.BaseAsset, strategy.QuoteAsset))
			ui.KV("Network", string(strategy.Network))
			ui.KV("Status", string(strategy.Status))

			ui.SectionLabel("Risk Limits")
			ui.KV("Max Position", fmt.Sprintf("%.2f", strategy.RiskLimits.MaxPositionSize))
			ui.KV("Max Daily Loss", fmt.Sprintf("%.2f XLM", strategy.RiskLimits.MaxDailyLoss))
			ui.KV("Stop Loss", fmt.Sprintf("%.1f%%", strategy.RiskLimits.StopLossPercent))
			ui.KV("Take Profit", fmt.Sprintf("%.1f%%", strategy.RiskLimits.TakeProfitPercent))

			fmt.Println()
			ui.Info(fmt.Sprintf("Start the strategy with: mozartpay trade start %s", strategy.ID))

			return nil
		},
	}
}

// ─── trade list ───────────────────────────────

func newTradeListCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	showAll := fs.Bool("all", false, "Show all strategies including stopped")

	return &Command{
		Name:  "list",
		Short: "List trading strategies",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			ui.Header("Trading Strategies")

			svc := trading.NewService(models.NetworkStellarMainnet)
			strategies := svc.GetAllStrategies()

			if len(strategies) == 0 {
				ui.Info("No strategies configured")
				ui.Info("Create one with: mozartpay trade strategy --type <type> --name <name>")
				return nil
			}

			t := ui.NewTable("ID", "Name", "Type", "Pair", "Status", "Trades", "Profit")
			for _, s := range strategies {
				if !*showAll && s.Status == models.StrategyStopped {
					continue
				}
				shortID := s.ID
				if len(shortID) > 12 {
					shortID = shortID[:12] + "..."
				}
				profitStr := fmt.Sprintf("%.4f", s.TotalProfit)
				statusColor := ui.Reset
				switch s.Status {
				case models.StrategyActive:
					statusColor = ui.BrightGreen
				case models.StrategyPaused:
					statusColor = ui.BrightYellow
				case models.StrategyError:
					statusColor = ui.Red
				}
				t.AddRow(
					shortID,
					s.Name,
					string(s.Type),
					fmt.Sprintf("%s/%s", s.BaseAsset, s.QuoteAsset),
					statusColor+string(s.Status)+ui.Reset,
					fmt.Sprintf("%d", s.TotalTrades),
					profitStr,
				)
			}
			t.Print()

			fmt.Println()
			ui.Info("Use 'mozartpay trade status <id>' for detailed strategy info")

			return nil
		},
	}
}

// ─── trade start ──────────────────────────────

func newTradeStartCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "start",
		Short: "Start a trading strategy",
		Long:  "Activate a trading strategy to begin automated trading",
		Run: func(c *Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("strategy ID required")
			}

			strategyID := args[0]

			ui.Header("Start Trading Strategy")

			svc := trading.NewService(models.NetworkStellarMainnet)

			strategy, err := svc.GetStrategy(strategyID)
			if err != nil {
				return err
			}

			ui.Info(fmt.Sprintf("Starting %s (%s)...", strategy.Name, strategy.Type))

			if err := svc.StartStrategy(strategyID); err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Success(fmt.Sprintf("Strategy '%s' started successfully", strategy.Name))
			ui.Info(fmt.Sprintf("Monitoring %s/%s on %s", strategy.BaseAsset, strategy.QuoteAsset, strategy.Network))

			return nil
		},
	}
}

// ─── trade stop ───────────────────────────────

func newTradeStopCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "stop",
		Short: "Stop a trading strategy",
		Long:  "Deactivate a trading strategy",
		Run: func(c *Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("strategy ID required")
			}

			strategyID := args[0]

			ui.Header("Stop Trading Strategy")

			svc := trading.NewService(models.NetworkStellarMainnet)

			strategy, err := svc.GetStrategy(strategyID)
			if err != nil {
				return err
			}

			if err := svc.StopStrategy(strategyID); err != nil {
				ui.Error(err.Error())
				return err
			}

			ui.Success(fmt.Sprintf("Strategy '%s' stopped", strategy.Name))
			ui.Info(fmt.Sprintf("Total trades: %d | Total profit: %.4f XLM", strategy.TotalTrades, strategy.TotalProfit))

			return nil
		},
	}
}

// ─── trade status ─────────────────────────────

func newTradeStatusCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "status",
		Short: "Show detailed strategy status",
		Run: func(c *Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("strategy ID required")
			}

			strategyID := args[0]

			svc := trading.NewService(models.NetworkStellarMainnet)

			strategy, err := svc.GetStrategy(strategyID)
			if err != nil {
				return err
			}

			ui.Header(fmt.Sprintf("Strategy: %s", strategy.Name))

			ui.SectionLabel("General")
			ui.KV("ID", strategy.ID)
			ui.KV("Type", string(strategy.Type))
			ui.KV("Pair", fmt.Sprintf("%s/%s", strategy.BaseAsset, strategy.QuoteAsset))
			ui.KV("Network", string(strategy.Network))

			statusColor := ui.Reset
			switch strategy.Status {
			case models.StrategyActive:
				statusColor = ui.BrightGreen
			case models.StrategyPaused:
				statusColor = ui.BrightYellow
			case models.StrategyError:
				statusColor = ui.Red
			}
			ui.KVColor("Status", string(strategy.Status), statusColor)

			if strategy.LastRunAt != nil {
				ui.KV("Last Run", strategy.LastRunAt.Format("15:04:05"))
			}

			ui.SectionLabel("Performance")
			ui.KV("Total Trades", fmt.Sprintf("%d", strategy.TotalTrades))
			ui.KV("Total Profit", fmt.Sprintf("%.4f XLM", strategy.TotalProfit))

			// Get detailed performance
			perf, err := svc.GetPerformance(strategyID)
			if err == nil && perf.TotalTrades > 0 {
				ui.KV("Win Rate", fmt.Sprintf("%.1f%%", perf.WinRate))
				ui.KV("Profit Factor", fmt.Sprintf("%.2f", perf.ProfitFactor))
			}

			ui.SectionLabel("Risk Limits")
			ui.KV("Max Position", fmt.Sprintf("%.2f", strategy.RiskLimits.MaxPositionSize))
			ui.KV("Max Daily Loss", fmt.Sprintf("%.2f XLM", strategy.RiskLimits.MaxDailyLoss))
			ui.KV("Stop Loss", fmt.Sprintf("%.1f%%", strategy.RiskLimits.StopLossPercent))
			ui.KV("Take Profit", fmt.Sprintf("%.1f%%", strategy.RiskLimits.TakeProfitPercent))

			return nil
		},
	}
}

// ─── trade performance ────────────────────────

func newTradePerformanceCmd(cfg *config.Config) *Command {
	return &Command{
		Name:  "performance",
		Short: "Show strategy performance metrics",
		Run: func(c *Command, args []string) error {
			if len(args) < 1 {
				// Show summary of all strategies
				ui.Header("Trading Performance Summary")

				svc := trading.NewService(models.NetworkStellarMainnet)
				strategies := svc.GetAllStrategies()

				if len(strategies) == 0 {
					ui.Info("No strategies to show performance for")
					return nil
				}

				t := ui.NewTable("Name", "Type", "Trades", "Wins", "Losses", "Win Rate", "Total Return")
				for _, s := range strategies {
					perf, err := svc.GetPerformance(s.ID)
					if err != nil {
						continue
					}
					t.AddRow(
						s.Name,
						string(s.Type),
						fmt.Sprintf("%d", perf.TotalTrades),
						fmt.Sprintf("%d", perf.WinningTrades),
						fmt.Sprintf("%d", perf.LosingTrades),
						fmt.Sprintf("%.1f%%", perf.WinRate),
						fmt.Sprintf("%.4f XLM", perf.TotalReturn),
					)
				}
				t.Print()
				return nil
			}

			strategyID := args[0]

			svc := trading.NewService(models.NetworkStellarMainnet)

			strategy, err := svc.GetStrategy(strategyID)
			if err != nil {
				return err
			}

			perf, err := svc.GetPerformance(strategyID)
			if err != nil {
				return err
			}

			ui.Header(fmt.Sprintf("Performance: %s", strategy.Name))

			ui.SectionLabel("Trade Statistics")
			ui.KV("Total Trades", fmt.Sprintf("%d", perf.TotalTrades))
			ui.KV("Winning Trades", fmt.Sprintf("%d", perf.WinningTrades))
			ui.KV("Losing Trades", fmt.Sprintf("%d", perf.LosingTrades))
			ui.KVColor("Win Rate", fmt.Sprintf("%.1f%%", perf.WinRate), ui.BrightGreen)

			ui.SectionLabel("Returns")
			ui.KV("Average Profit", fmt.Sprintf("%.4f XLM", perf.AvgProfit))
			ui.KV("Average Loss", fmt.Sprintf("%.4f XLM", perf.AvgLoss))
			ui.KV("Profit Factor", fmt.Sprintf("%.2f", perf.ProfitFactor))
			ui.KVColor("Total Return", fmt.Sprintf("%.4f XLM", perf.TotalReturn), ui.BrightGreen)

			ui.SectionLabel("Risk Metrics")
			ui.KV("Sharpe Ratio", fmt.Sprintf("%.2f", perf.SharpeRatio))
			ui.KV("Max Drawdown", fmt.Sprintf("%.2f%%", perf.MaxDrawdown))

			return nil
		},
	}
}

// ─── trade history ────────────────────────────

func newTradeHistoryCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Number of executions to show")

	return &Command{
		Name:  "history",
		Short: "Show strategy execution history",
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("strategy ID required")
			}

			strategyID := args[0]

			svc := trading.NewService(models.NetworkStellarMainnet)

			strategy, err := svc.GetStrategy(strategyID)
			if err != nil {
				return err
			}

			executions := svc.GetExecutions(strategyID, *limit)

			ui.Header(fmt.Sprintf("Execution History: %s", strategy.Name))

			if len(executions) == 0 {
				ui.Info("No executions yet")
				return nil
			}

			t := ui.NewTable("Time", "Action", "Amount", "Price", "Value", "P&L", "Status")
			for _, e := range executions {
				plStr := "-"
				plColor := ui.Reset
				if e.ProfitLoss != 0 {
					plStr = fmt.Sprintf("%.4f", e.ProfitLoss)
					if e.ProfitLoss > 0 {
						plColor = ui.BrightGreen
					} else {
						plColor = ui.Red
					}
				}
				t.AddRow(
					e.Timestamp.Format("15:04:05"),
					string(e.Action),
					fmt.Sprintf("%.2f", e.Amount),
					fmt.Sprintf("%.6f", e.Price),
					fmt.Sprintf("%.4f", e.Value),
					plColor+plStr+ui.Reset,
					string(e.Status),
				)
			}
			t.Print()

			return nil
		},
	}
}
