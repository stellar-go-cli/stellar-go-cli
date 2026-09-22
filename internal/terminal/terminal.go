package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/integrations"
	"github.com/stellar-go-cli/stellar-go-cli/internal/wallet"
)

// Bloomberg-style colors
var (
	colorOrange   = lipgloss.Color("#FF6B00")
	colorGold     = lipgloss.Color("#FFB000")
	colorGreen    = lipgloss.Color("#00FF41")
	colorRed      = lipgloss.Color("#FF4136")
	colorWhite    = lipgloss.Color("#FFFFFF")
	colorGray     = lipgloss.Color("#666666")
	colorBlack    = lipgloss.Color("#0A0A0A")
	colorDarkGray = lipgloss.Color("#1A1A1A")
)

// Styles - Enhanced for better UX
var (
	headerStyle = lipgloss.NewStyle().
			Background(colorOrange).
			Foreground(colorBlack).
			Bold(true).
			Padding(0, 1).
			MarginBottom(1)

	// Enhanced panel styles with better borders
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorGray).
			Background(colorDarkGray).
			Padding(0, 1)

	panelStyleActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorOrange).
				Background(colorDarkGray).
				Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true).
			MarginBottom(1)

	// Compact styles for dense data
	compactLabelStyle = lipgloss.NewStyle().
				Foreground(colorGray).
				MarginTop(0).
				MarginBottom(0)

	compactValueStyle = lipgloss.NewStyle().
				Foreground(colorWhite).
				Bold(true).
				MarginTop(0).
				MarginBottom(0)

	// Ticker styles
	tickerUpStyle = lipgloss.NewStyle().
			Foreground(colorGreen).
			Bold(true)

	tickerDownStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	tickerNeutralStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	tickerHeaderStyle = lipgloss.NewStyle().
				Foreground(colorGray).
				Bold(true)

	// Section divider
	dividerStyle = lipgloss.NewStyle().
			Foreground(colorGray)
)

// Key bindings
type keyMap struct {
	Quit   key.Binding
	Help   key.Binding
	Reload key.Binding
	Tab    key.Binding
	Up     key.Binding
	Down   key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?", "h"),
		key.WithHelp("?", "toggle help"),
	),
	Reload: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reload data"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch panel"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑", "scroll up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓", "scroll down"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit, k.Help, k.Reload, k.Up, k.Down}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Quit, k.Help, k.Reload, k.Tab, k.Up, k.Down},
	}
}

// Ticker data types
type TickerData struct {
	Symbol    string
	Price     float64
	Change24h float64
	Volume    float64
	Updated   time.Time
}

type WalletData struct {
	Address      string
	Network      string
	Balances     map[string]float64
	Transactions []wallet.TransactionInfo
	Updated      time.Time
}

type NewsArticle struct {
	Title       string
	URL         string
	Summary     string
	Source      string
	PublishedAt time.Time
	Relevance   float64
}

// Model for bubbletea
type Model struct {
	cfg         *config.Config
	keys        keyMap
	help        help.Model
	showHelp    bool
	width       int
	height      int
	activePanel int

	// Data
	tickers    []TickerData
	wallet     WalletData
	news       []NewsArticle
	walletErr  string
	newsClient *integrations.FinnhubClient

	// Viewport for scrolling in small views
	viewport    viewport.Model
	useViewport bool

	// Refresh
	lastRefresh time.Time
	tickChan    chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
}

// Initialize the terminal model
func NewModel(cfg *config.Config) Model {
	ctx, cancel := context.WithCancel(context.Background())
	vp := viewport.New(80, 24)
	vp.SetContent("")
	m := Model{
		cfg:         cfg,
		keys:        keys,
		help:        help.New(),
		tickChan:    make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		news:        []NewsArticle{},
		wallet:      WalletData{Balances: make(map[string]float64)},
		newsClient:  integrations.NewFinnhubClient(cfg.Integrations.FinnhubAPIKey),
		viewport:    vp,
		useViewport: false,
	}
	// Initialize with data immediately
	m.loadTickers()
	return m
}

// tickMsg is sent when we should refresh data
type tickMsg time.Time

// tickCmd returns a command that ticks every 30 seconds for market data refresh
func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		func() tea.Msg {
			// Defer initial data load to avoid blocking UI startup
			time.Sleep(100 * time.Millisecond)
			return tickMsg(time.Now())
		},
		tickCmd(),
	)
}

func (m *Model) loadWalletData() {
	m.walletErr = ""
	svc := wallet.NewService()
	acc, err := svc.GetActiveWallet()
	if err != nil {
		m.walletErr = fmt.Sprintf("No active wallet: %v", err)
		m.wallet = WalletData{Balances: make(map[string]float64), Transactions: []wallet.TransactionInfo{}}
		return
	}

	m.wallet.Address = acc.Address
	m.wallet.Network = string(acc.Network)
	m.wallet.Balances = make(map[string]float64)

	// Try to fetch live balance from network
	if acc.Funded {
		updatedAcc, err := svc.UpdateWalletBalance(acc.Address)
		if err == nil && updatedAcc != nil {
			acc = updatedAcc
		}
	}

	// Parse balance
	if acc.Funded && acc.Balance != "" {
		var bal float64
		if _, err := fmt.Sscanf(acc.Balance, "%f", &bal); err == nil {
			m.wallet.Balances["XLM"] = bal
		}
	}

	// Fetch assets
	if acc.Funded {
		assets, _ := svc.GetAccountAssets(acc.Address, acc.Network) //nolint:errcheck // empty list on error is acceptable
		importantAssets := map[string]bool{
			"USDC": true, "USDT": true, "yXLM": true,
			"AQUA": true, "SHX": true, "XRP": true, "BTC": true, "ETH": true,
			"EURT": true,
		}
		for _, asset := range assets {
			if asset.Code == "XLM" {
				continue
			}
			if importantAssets[asset.Code] {
				var bal float64
				if _, err := fmt.Sscanf(asset.Balance, "%f", &bal); err == nil {
					m.wallet.Balances[asset.Code] = bal
				}
			}
		}
	}

	// Fetch recent transactions
	if acc.Funded {
		txs, _ := svc.GetTransactions(acc.Address, acc.Network, 5) //nolint:errcheck // empty list on error is acceptable
		m.wallet.Transactions = txs
	} else {
		m.wallet.Transactions = []wallet.TransactionInfo{}
	}

	m.wallet.Updated = time.Now()
}

func (m *Model) loadTickers() {
	m.tickers = fetchLivePrices()
}

// fetchLivePrices gets live crypto prices from CoinGecko and EUR/USD from Frankfurter
// Always returns exactly 5 tickers: XLM, BTC, ETH, USDC, EUR
func fetchLivePrices() []TickerData {
	client := &http.Client{Timeout: 5 * time.Second}
	now := time.Now()

	// Initialize with fallback values (ensures we always have data)
	tickers := map[string]TickerData{
		"XLM/USD":  {Symbol: "XLM/USD", Price: 0.1660, Change24h: 0.0, Updated: now},
		"BTC/USD":  {Symbol: "BTC/USD", Price: 67500.0, Change24h: 0.0, Updated: now},
		"ETH/USD":  {Symbol: "ETH/USD", Price: 2100.0, Change24h: 0.0, Updated: now},
		"USDC/USD": {Symbol: "USDC/USD", Price: 1.00, Change24h: 0.01, Updated: now},
		"EUR/USD":  {Symbol: "EUR/USD", Price: 1.15, Change24h: 0.0, Updated: now},
	}

	// Fetch crypto prices from CoinGecko
	cryptoURL := "https://api.coingecko.com/api/v3/simple/price?ids=stellar,bitcoin,ethereum&vs_currencies=usd&include_24hr_change=true"
	if resp, err := client.Get(cryptoURL); err == nil {
		defer resp.Body.Close() //nolint:errcheck // best-effort close
		var data map[string]struct {
			Usd          float64 `json:"usd"`
			Usd24hChange float64 `json:"usd_24h_change"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if stellar, ok := data["stellar"]; ok {
				tickers["XLM/USD"] = TickerData{Symbol: "XLM/USD", Price: stellar.Usd, Change24h: stellar.Usd24hChange, Updated: now}
			}
			if btc, ok := data["bitcoin"]; ok {
				tickers["BTC/USD"] = TickerData{Symbol: "BTC/USD", Price: btc.Usd, Change24h: btc.Usd24hChange, Updated: now}
			}
			if eth, ok := data["ethereum"]; ok {
				tickers["ETH/USD"] = TickerData{Symbol: "ETH/USD", Price: eth.Usd, Change24h: eth.Usd24hChange, Updated: now}
			}
		}
	}

	// Small delay between API calls to be nice to the servers
	time.Sleep(100 * time.Millisecond)

	// Fetch EUR/USD from Frankfurter (ECB rates)
	if resp, err := client.Get("https://api.frankfurter.app/latest?from=EUR&to=USD"); err == nil {
		defer resp.Body.Close() //nolint:errcheck // best-effort close
		var data struct {
			Rates map[string]float64 `json:"rates"`
			Date  string             `json:"date"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
			if rate, ok := data.Rates["USD"]; ok {
				tickers["EUR/USD"] = TickerData{Symbol: "EUR/USD", Price: rate, Change24h: 0.0, Updated: now}
			}
		}
	}

	// Return in consistent order: XLM, BTC, ETH, USDC, EUR
	result := []TickerData{
		tickers["XLM/USD"],
		tickers["BTC/USD"],
		tickers["ETH/USD"],
		tickers["USDC/USD"],
		tickers["EUR/USD"],
	}

	return result
}

func (m *Model) loadNews() {
	// Clear existing news
	m.news = []NewsArticle{}

	// Get wallet assets to fetch relevant news
	if len(m.wallet.Balances) == 0 {
		fmt.Fprintf(os.Stderr, "[DEBUG] No wallet balances\n")
		return // No wallet assets, keep empty news
	}

	// Extract asset names from wallet balances
	var assets []string
	for asset := range m.wallet.Balances {
		assets = append(assets, asset)
	}

	if len(assets) == 0 {
		fmt.Fprintf(os.Stderr, "[DEBUG] No assets extracted\n")
		return // No assets to fetch news for
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Fetching news for assets: %v\n", assets)
	fmt.Fprintf(os.Stderr, "[DEBUG] API Key configured: %v\n", m.newsClient.IsConfigured())

	// Fetch news using Finnhub client - limit to 3 articles
	articles, err := m.newsClient.FetchNews(assets, 3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] FetchNews error: %v\n", err)
		// Fall back to mock news on error
		articles = m.newsClient.GetMockNews(assets, 3)
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] Got %d articles\n", len(articles))

	// If API returned no articles, use mock news
	if len(articles) == 0 {
		fmt.Fprintf(os.Stderr, "[DEBUG] API returned 0 articles, using mock news\n")
		articles = m.newsClient.GetMockNews(assets, 3)
	}

	// Convert integrations.NewsArticle to terminal.NewsArticle
	for _, article := range articles {
		m.news = append(m.news, NewsArticle{
			Title:       article.Title,
			URL:         article.URL,
			Summary:     article.Summary,
			Source:      article.Source,
			PublishedAt: article.PublishedAt,
			Relevance:   article.Relevance,
		})
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.cancel()
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
		case key.Matches(msg, m.keys.Reload):
			m.loadWalletData()
			m.loadTickers()
			m.loadNews()
			m.lastRefresh = time.Now()
			return m, nil
		case key.Matches(msg, m.keys.Tab):
			m.activePanel = (m.activePanel + 1) % 3
		case key.Matches(msg, m.keys.Up):
			if m.useViewport {
				m.viewport.ScrollUp(1)
			}
			return m, nil
		case key.Matches(msg, m.keys.Down):
			if m.useViewport {
				m.viewport.ScrollDown(1)
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		// Update viewport dimensions - use viewport in single column mode
		if m.width < 104 { // < 100 cols + margins
			m.useViewport = true
			headerHeight := 1
			stripHeight := 1
			contentHeight := m.height - headerHeight - stripHeight - 1
			if m.showHelp {
				contentHeight -= 4
			}
			m.viewport.Width = m.width - 4
			m.viewport.Height = contentHeight
		} else {
			m.useViewport = false
		}

	case tickMsg:
		if m.lastRefresh.IsZero() {
			// First load - load everything
			m.loadWalletData()
			m.loadTickers()
			m.loadNews()
			m.lastRefresh = time.Now()
		} else {
			// Subsequent ticks - only refresh market data (lightweight)
			m.loadTickers()
		}
		// Return tickCmd to schedule next tick
		return m, tickCmd()
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Use viewport for scrolling in small views
	if m.useViewport {
		return m.renderScrollableView()
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	// Header takes 1 line, bottom strip takes 1 line, spacing takes 1 line
	headerHeight := 1
	stripHeight := 1
	spacing := 1

	contentHeight := m.height - headerHeight - stripHeight - spacing
	if m.showHelp {
		contentHeight -= 4 // Help takes 4 lines
	}

	// Panels row - use available height
	panels := m.renderPanels(contentHeight)
	sections = append(sections, panels)

	// Help or ticker strip
	if m.showHelp {
		sections = append(sections, m.help.View(m.keys))
	} else {
		sections = append(sections, m.renderTickerStrip())
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderScrollableView() string {
	// Build all content for viewport
	var content []string

	content = append(content, m.renderHeader())
	content = append(content, "")

	// All panels stacked for scrolling
	availableWidth := m.width - 4
	panelHeight := 20 // Fixed height for each panel in scrollable mode

	tickerContent := m.renderTickerPanel(availableWidth, panelHeight)
	content = append(content, tickerContent)
	content = append(content, "")

	walletContent := m.renderWalletPanel(availableWidth, panelHeight)
	content = append(content, walletContent)
	content = append(content, "")

	newsContent := m.renderNewsPanel(availableWidth, panelHeight)
	content = append(content, newsContent)

	fullContent := lipgloss.JoinVertical(lipgloss.Left, content...)
	m.viewport.SetContent(fullContent)

	return lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render(" STELLAR TERMINAL "),
		m.viewport.View(),
		m.renderTickerStrip(),
	)
}

func (m Model) renderHeader() string {
	timeStr := time.Now().Format("15:04:05")
	status := "● LIVE"

	left := headerStyle.Render(" STELLAR TERMINAL ")
	center := lipgloss.NewStyle().
		Foreground(colorGray).
		Render(fmt.Sprintf("v0.1.0-mvp | Last update: %s", m.lastRefresh.Format("15:04:05")))
	right := lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true).
		Render(fmt.Sprintf("%s %s", status, timeStr))

	// Calculate spacing, ensuring it never goes negative
	spaceCount := m.width - lipgloss.Width(left) - lipgloss.Width(right) - lipgloss.Width(center)
	if spaceCount < 0 {
		spaceCount = 0
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Render(lipgloss.JoinHorizontal(lipgloss.Center,
			left,
			strings.Repeat(" ", spaceCount),
			center,
			right,
		))
}

func (m Model) renderPanels(height int) string {
	// Calculate available width accounting for borders and spacing
	availableWidth := m.width - 4 // Account for outer margins

	// Determine layout based on terminal width
	// < 100 cols: single column
	// 100-150 cols: 2 columns (tickers+wallet | news)
	// > 150 cols: 3 columns
	var layout string
	switch {
	case availableWidth < 100:
		layout = "single"
	case availableWidth < 150:
		layout = "two"
	default:
		layout = "three"
	}

	switch layout {
	case "single":
		return m.renderSingleColumn(height, availableWidth)
	case "two":
		return m.renderTwoColumn(height, availableWidth)
	default:
		return m.renderThreeColumn(height, availableWidth)
	}
}

func (m Model) renderThreeColumn(height, availableWidth int) string {
	// Three equal columns with 2-char spacing
	panelWidth := (availableWidth - 4) / 3

	// Ticker panel
	tickerContent := m.renderTickerPanel(panelWidth, height)
	tickerPanel := m.getPanelStyle(0).
		Width(panelWidth).
		Height(height).
		Render(tickerContent)

	// Wallet panel
	walletContent := m.renderWalletPanel(panelWidth, height)
	walletPanel := m.getPanelStyle(1).
		Width(panelWidth).
		Height(height).
		Render(walletContent)

	// News panel
	newsContent := m.renderNewsPanel(panelWidth, height)
	newsPanel := m.getPanelStyle(2).
		Width(panelWidth).
		Height(height).
		Render(newsContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, tickerPanel, "  ", walletPanel, "  ", newsPanel)
}

func (m Model) renderTwoColumn(height, availableWidth int) string {
	// Left: tickers+wallet (60%), Right: news (40%)
	leftWidth := (availableWidth - 2) * 6 / 10
	rightWidth := availableWidth - 2 - leftWidth

	// Left side - stacked
	leftHeight := height / 2
	tickerContent := m.renderTickerPanel(leftWidth, leftHeight)
	tickerPanel := m.getPanelStyle(0).
		Width(leftWidth).
		Height(leftHeight).
		Render(tickerContent)

	walletContent := m.renderWalletPanel(leftWidth, leftHeight)
	walletPanel := m.getPanelStyle(1).
		Width(leftWidth).
		Height(height - leftHeight).
		Render(walletContent)

	leftStack := lipgloss.JoinVertical(lipgloss.Left, tickerPanel, walletPanel)

	// Right side - news full height
	newsContent := m.renderNewsPanel(rightWidth, height)
	newsPanel := m.getPanelStyle(2).
		Width(rightWidth).
		Height(height).
		Render(newsContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftStack, "  ", newsPanel)
}

func (m Model) renderSingleColumn(height, availableWidth int) string {
	// Stack all panels vertically
	panelHeight := (height - 2) / 3

	tickerContent := m.renderTickerPanel(availableWidth, panelHeight)
	tickerPanel := m.getPanelStyle(0).
		Width(availableWidth).
		Height(panelHeight).
		Render(tickerContent)

	walletContent := m.renderWalletPanel(availableWidth, panelHeight)
	walletPanel := m.getPanelStyle(1).
		Width(availableWidth).
		Height(panelHeight).
		Render(walletContent)

	newsHeight := height - 2*panelHeight - 2
	newsContent := m.renderNewsPanel(availableWidth, newsHeight)
	newsPanel := m.getPanelStyle(2).
		Width(availableWidth).
		Height(newsHeight).
		Render(newsContent)

	return lipgloss.JoinVertical(lipgloss.Left, tickerPanel, walletPanel, newsPanel)
}

func (m Model) getPanelStyle(index int) lipgloss.Style {
	if m.activePanel == index {
		return panelStyleActive
	}
	return panelStyle
}

func (m Model) renderTickerPanel(width, height int) string {
	var lines []string
	lines = append(lines, panelTitleStyle.Render("📊 MARKET DATA"))
	lines = append(lines, "")

	// Compact table-like layout
	colWidth := width - 4
	priceWidth := 12
	changeWidth := 10
	symbolWidth := colWidth - priceWidth - changeWidth - 3

	// Header row (compact)
	if width > 40 {
		header := fmt.Sprintf("%-*s %*s %*s",
			symbolWidth, "SYMBOL",
			priceWidth, "PRICE",
			changeWidth, "24H")
		lines = append(lines, tickerHeaderStyle.Render(header))
		lines = append(lines, dividerStyle.Render(strings.Repeat("─", colWidth)))
	}

	for _, ticker := range m.tickers {
		style := tickerNeutralStyle
		arrow := "─"
		if ticker.Change24h > 0 {
			style = tickerUpStyle
			arrow = "▲"
		} else if ticker.Change24h < 0 {
			style = tickerDownStyle
			arrow = "▼"
		}

		var line string
		if width > 50 {
			// Full format
			line = fmt.Sprintf("%-*s %*.4f %s%*.2f%%",
				symbolWidth, ticker.Symbol,
				priceWidth-1, ticker.Price,
				arrow,
				changeWidth-2, ticker.Change24h)
		} else {
			// Compact format for narrow panels
			line = fmt.Sprintf("%-8s %.4f %+.1f%%",
				ticker.Symbol,
				ticker.Price,
				ticker.Change24h)
		}
		lines = append(lines, style.Render(line))
	}

	return lipgloss.NewStyle().
		Height(height - 2).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderWalletPanel(width, height int) string {
	var lines []string
	lines = append(lines, panelTitleStyle.Render("💼 WALLET & ACTIVITY"))

	if m.walletErr != "" {
		lines = append(lines, "")
		lines = append(lines, compactLabelStyle.Render(m.walletErr))
		lines = append(lines, "")
		lines = append(lines, compactLabelStyle.Render("Use 'wallet connect' to connect"))
	} else if m.wallet.Address == "" {
		lines = append(lines, "")
		lines = append(lines, compactLabelStyle.Render("No active wallet"))
		lines = append(lines, "")
		lines = append(lines, compactLabelStyle.Render("Use 'wallet connect' to connect"))
	} else {
		// Compact layout
		addr := m.wallet.Address
		maxAddrLen := width - 10
		if len(addr) > maxAddrLen && maxAddrLen > 15 {
			addr = addr[:maxAddrLen/2-2] + "..." + addr[len(addr)-maxAddrLen/2+2:]
		}

		lines = append(lines, "")
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left,
			compactLabelStyle.Render("Address: "),
			compactValueStyle.Render(addr)))
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left,
			compactLabelStyle.Render("Network: "),
			compactValueStyle.Render(m.wallet.Network)))

		// Sort assets alphabetically for consistent display order
		var assets []string
		for asset := range m.wallet.Balances {
			assets = append(assets, asset)
		}
		sort.Strings(assets)

		if len(assets) > 0 {
			lines = append(lines, "")
			lines = append(lines, panelTitleStyle.Render("Balances"))

			// Calculate column widths
			colWidth := width - 4
			balWidth := 12
			assetWidth := colWidth - balWidth - 1

			for _, asset := range assets {
				balance := m.wallet.Balances[asset]
				line := fmt.Sprintf("%-*s %*.4f",
					assetWidth, asset,
					balWidth-1, balance)
				lines = append(lines, compactValueStyle.Render(line))
			}
		}

		// Show recent transactions
		if len(m.wallet.Transactions) > 0 {
			lines = append(lines, "")
			lines = append(lines, panelTitleStyle.Render("Recent Transactions"))

			maxTx := 3
			if len(m.wallet.Transactions) < maxTx {
				maxTx = len(m.wallet.Transactions)
			}

			for i := 0; i < maxTx; i++ {
				tx := m.wallet.Transactions[i]
				statusColor := colorGreen
				if tx.Status == "Failed" {
					statusColor = colorRed
				}
				statusStyle := lipgloss.NewStyle().Foreground(statusColor)

				// Truncate hash if too narrow
				txHash := tx.Hash
				if width < 50 && len(txHash) > 8 {
					txHash = txHash[:8]
				}

				var txLine string
				if tx.Amount > 0 && tx.Asset != "" {
					txLine = fmt.Sprintf("%s %s %.2f %s %s",
						statusStyle.Render("●"),
						txHash,
						tx.Amount,
						tx.Asset,
						formatTimeAgo(tx.Timestamp))
				} else {
					txLine = fmt.Sprintf("%s %s %s %s",
						statusStyle.Render("●"),
						txHash,
						tx.Type,
						formatTimeAgo(tx.Timestamp))
				}
				lines = append(lines, compactValueStyle.Render(txLine))
			}
		}
	}

	return lipgloss.NewStyle().
		Height(height - 2).
		Render(strings.Join(lines, "\n"))
}
func (m Model) renderNewsPanel(width, height int) string {
	var lines []string
	lines = append(lines, panelTitleStyle.Render("📰 RECENT NEWS"))

	if len(m.news) == 0 {
		lines = append(lines, "")
		lines = append(lines, compactLabelStyle.Render("No news available"))
		lines = append(lines, "")
		if len(m.wallet.Balances) == 0 {
			lines = append(lines, compactLabelStyle.Render("Connect a wallet to see asset-relevant news"))
		} else {
			lines = append(lines, compactLabelStyle.Render("News will refresh every 5 minutes"))
		}
	} else {
		// Calculate max title length based on panel width
		maxTitleLen := width - 6
		maxSummaryLen := width - 4

		for _, article := range m.news {
			lines = append(lines, "")

			// Truncate title for display
			title := article.Title
			if len(title) > maxTitleLen && maxTitleLen > 10 {
				title = title[:maxTitleLen-3] + "..."
			}

			// Show source and time
			timeStr := formatTimeAgo(article.PublishedAt)
			sourceLine := fmt.Sprintf("%s • %s", article.Source, timeStr)

			// Show relevance indicator with color
			relevanceIndicator := "●"
			relevanceColor := colorGray
			if article.Relevance >= 0.8 {
				relevanceColor = colorGreen
			} else if article.Relevance >= 0.6 {
				relevanceColor = colorGold
			}

			relevanceStyle := lipgloss.NewStyle().Foreground(relevanceColor)
			lines = append(lines, relevanceStyle.Render(fmt.Sprintf("%s %s", relevanceIndicator, title)))

			if width > 40 {
				lines = append(lines, compactLabelStyle.Render(fmt.Sprintf("  %s", sourceLine)))

				// Show truncated summary on wider panels
				if article.Summary != "" && height > 20 {
					summary := article.Summary
					if len(summary) > maxSummaryLen {
						summary = summary[:maxSummaryLen-3] + "..."
					}
					lines = append(lines, compactLabelStyle.Render(fmt.Sprintf("  %s", summary)))
				}
			}
		}
	}

	return lipgloss.NewStyle().
		Height(height - 2).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderTickerStrip() string {
	var items []string

	for _, ticker := range m.tickers {
		style := tickerNeutralStyle
		if ticker.Change24h > 0 {
			style = tickerUpStyle
		} else if ticker.Change24h < 0 {
			style = tickerDownStyle
		}

		item := fmt.Sprintf("%s %.4f (%+.2f%%)", ticker.Symbol, ticker.Price, ticker.Change24h)
		items = append(items, style.Render(item))
	}

	return lipgloss.NewStyle().
		Background(colorDarkGray).
		Width(m.width).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Left, items...))
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return t.Format("Jan 02")
}

// truncateURL truncates a URL for display
// Run starts the terminal UI
func Run(cfg *config.Config) error {
	model := NewModel(cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running terminal: %v\n", err)
		return err
	}

	return nil
}
