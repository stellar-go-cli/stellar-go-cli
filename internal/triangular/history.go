package triangular

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stellar-go-cli/stellar-go-cli/pkg/models"
)

// HistoryStore provides persistence for triangular arbitrage data
type HistoryStore struct {
	db *sql.DB
}

// quoteRate returns the output/input rate for a swap leg quote.
func quoteRate(q *models.SwapQuote) float64 {
	in, err := strconv.ParseFloat(q.Amount, 64)
	if err != nil || in == 0 {
		return 0
	}
	out, err := strconv.ParseFloat(q.ExpectedAmount, 64)
	if err != nil {
		return 0
	}
	return out / in
}

// NewHistoryStore creates/opens the SQLite database
func NewHistoryStore() (*HistoryStore, error) {
	// Create data directory in home
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dataDir := filepath.Join(home, ".stellar-go-cli")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "arbitrage.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &HistoryStore{db: db}
	if err := store.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables
func (h *HistoryStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS price_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		path TEXT NOT NULL,
		leg1_rate REAL,
		leg2_rate REAL,
		leg3_rate REAL,
		combined_rate REAL NOT NULL,
		deviation REAL NOT NULL,
		z_score REAL,
		volatility REAL,
		is_opportunity BOOLEAN DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_price_history_path ON price_history(path);
	CREATE INDEX IF NOT EXISTS idx_price_history_timestamp ON price_history(timestamp);

	CREATE TABLE IF NOT EXISTS arbitrage_trades (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		path TEXT NOT NULL,
		amount_xlm REAL,
		expected_profit_xlm REAL,
		actual_profit_xlm REAL,
		executed BOOLEAN DEFAULT 0,
		success BOOLEAN DEFAULT 0
	);
	`

	_, err := h.db.Exec(schema)
	return err
}

// RecordScan saves a triangular scan result to history
func (h *HistoryStore) RecordScan(result TriangularResult) error {
	// Calculate individual leg rates
	leg1Rate := 0.0
	leg2Rate := 0.0
	leg3Rate := 0.0

	// Rate = output / input for each leg
	// Leg 1: XLM -> USDC (rate = USDC/XLM)
	// Leg 2: USDC -> yXLM (rate = yXLM/USDC)
	// Leg 3: yXLM -> XLM (rate = XLM/yXLM)
	if result.Leg1Quote != nil {
		leg1Rate = quoteRate(result.Leg1Quote)
	}
	if result.Leg2Quote != nil {
		leg2Rate = quoteRate(result.Leg2Quote)
	}
	if result.Leg3Quote != nil {
		leg3Rate = quoteRate(result.Leg3Quote)
	}

	_, err := h.db.Exec(
		`INSERT INTO price_history 
		(path, leg1_rate, leg2_rate, leg3_rate, combined_rate, deviation, is_opportunity)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		result.Path.Name,
		leg1Rate,
		leg2Rate,
		leg3Rate,
		result.Path.CombinedRate,
		result.Path.Deviation,
		result.Path.IsOpportunity,
	)
	return err
}

// GetHistory retrieves historical data for a path
func (h *HistoryStore) GetHistory(path string, limit int) ([]HistoryRecord, error) {
	rows, err := h.db.Query(
		`SELECT timestamp, combined_rate, deviation, IFNULL(z_score, 0), IFNULL(volatility, 0), is_opportunity
		FROM price_history 
		WHERE path = ?
		ORDER BY timestamp DESC
		LIMIT ?`,
		path, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // cleanup

	var records []HistoryRecord
	for rows.Next() {
		var r HistoryRecord
		var ts string
		err := rows.Scan(&ts, &r.CombinedRate, &r.Deviation, &r.ZScore, &r.Volatility, &r.IsOpportunity)
		if err != nil {
			continue
		}
		r.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts) //nolint:errcheck // parse failure yields zero timestamp
		r.Path = path
		records = append(records, r)
	}

	return records, nil
}

// GetAllPaths returns distinct paths in history
func (h *HistoryStore) GetAllPaths() ([]string, error) {
	rows, err := h.db.Query("SELECT DISTINCT path FROM price_history ORDER BY path")
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // cleanup

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err == nil {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

// HistoryRecord represents a single historical data point
type HistoryRecord struct {
	Timestamp     time.Time
	Path          string
	CombinedRate  float64
	Deviation     float64
	ZScore        float64
	Volatility    float64
	IsOpportunity bool
}

// Close closes the database connection
func (h *HistoryStore) Close() error {
	return h.db.Close()
}
