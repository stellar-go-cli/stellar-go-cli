package scanner

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SpreadPercent tracks the round-trip arbitrage spread percentage for each pair
	SpreadPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_spread_percent",
			Help: "Round-trip arbitrage spread percentage",
		},
		[]string{"pair", "network"},
	)

	// ProfitXLM tracks the estimated profit in XLM for each pair
	ProfitXLM = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_profit_xlm",
			Help: "Estimated profit in XLM after fees",
		},
		[]string{"pair", "network"},
	)

	// PathsChecked tracks the number of paths checked per pair
	PathsChecked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stellar_arbitrage_paths_checked_total",
			Help: "Total number of paths checked",
		},
		[]string{"pair", "network"},
	)

	// OpportunitiesFound tracks profitable opportunities
	OpportunitiesFound = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "stellar_arbitrage_opportunities_total",
			Help: "Total profitable opportunities found",
		},
		[]string{"pair", "network"},
	)

	// LastCheckTimestamp tracks when each pair was last checked
	LastCheckTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_last_check_timestamp",
			Help: "Unix timestamp of last check",
		},
		[]string{"pair", "network"},
	)

	// ScanDuration tracks how long each scan takes
	ScanDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "stellar_arbitrage_scan_duration_seconds",
			Help:    "Time spent scanning each pair",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"pair", "network"},
	)

	// LegAQuote tracks the first leg quote amount
	LegAQuote = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_leg_a_quote",
			Help: "Intermediate amount received in leg A",
		},
		[]string{"pair", "network", "intermediate_asset"},
	)

	// LegBQuote tracks the second leg quote amount
	LegBQuote = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_leg_b_quote",
			Help: "Final amount received in leg B",
		},
		[]string{"pair", "network", "base_asset"},
	)

	// IsProfitable indicates if the current spread is profitable
	IsProfitable = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "stellar_arbitrage_is_profitable",
			Help: "1 if spread is profitable, 0 otherwise",
		},
		[]string{"pair", "network"},
	)
)
