package integrations

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/models"

	mpCrypto "github.com/stellar-go-cli/stellar-go-cli/pkg/crypto"
)

// ─────────────────────────────────────────────
// OA — Orchestrated Agreement Score
// ─────────────────────────────────────────────

// Note: OA implementation removed - keeping structure for future use

// ─────────────────────────────────────────────
// Alpha Vantage — News & Market Data
// ─────────────────────────────────────────────

// NewsArticle represents a news article from Alpha Vantage
type NewsArticle struct {
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"publishedAt"`
	Relevance   float64   `json:"relevance"`
}

// AlphaVantageClient handles Alpha Vantage API interactions
type AlphaVantageClient struct {
	apiKey     string
	httpClient *http.Client
	mcpClient  *AlphaVantageMCPClient
	useMCP     bool
}

// NewAlphaVantageClient creates a new Alpha Vantage client
func NewAlphaVantageClient(apiKey string) *AlphaVantageClient {
	return &AlphaVantageClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		mcpClient: NewAlphaVantageMCPClient(apiKey),
		useMCP:    false, // Use REST API as primary (MCP experimental)
	}
}

// AssetToNewsTopics maps wallet assets to relevant news topics
func (av *AlphaVantageClient) AssetToNewsTopics(assets []string) []string {
	topics := make(map[string]bool)

	for _, asset := range assets {
		switch strings.ToUpper(asset) {
		case "XLM":
			topics["stellar"] = true
			topics["cryptocurrency"] = true
			topics["blockchain"] = true
		case "USDC", "USDT":
			topics["stablecoin"] = true
			topics["defi"] = true
			topics["cryptocurrency"] = true
		case "BTC", "Bitcoin":
			topics["bitcoin"] = true
			topics["cryptocurrency"] = true
		case "ETH", "Ethereum":
			topics["ethereum"] = true
			topics["cryptocurrency"] = true
			topics["defi"] = true
		default:
			// For other tokens, include general crypto news
			topics["cryptocurrency"] = true
			topics["defi"] = true
		}
	}

	var result []string
	for topic := range topics {
		result = append(result, topic)
	}
	return result
}

// FetchNews fetches relevant news articles for the given assets
func (av *AlphaVantageClient) FetchNews(assets []string, limit int) ([]NewsArticle, error) {
	if av.apiKey == "" {
		// Return mock news if no API key is configured
		return av.getMockNews(assets, limit), nil
	}

	// Use MCP client if enabled
	if av.useMCP && av.mcpClient != nil {
		return av.fetchNewsMCP(assets, limit)
	}

	// Use REST API (original implementation)
	return av.fetchNewsREST(assets, limit)
}

// fetchNewsREST fetches news using the REST API
func (av *AlphaVantageClient) fetchNewsREST(assets []string, limit int) ([]NewsArticle, error) {
	topics := av.AssetToNewsTopics(assets)
	var allArticles []NewsArticle

	// Alpha Vantage News API endpoint
	baseURL := "https://www.alphavantage.co/query"

	for _, topic := range topics {
		// Construct API request
		url := fmt.Sprintf("%s?function=NEWS_SENTIMENT&keywords=%s&apikey=%s",
			baseURL, topic, av.apiKey)

		resp, err := av.httpClient.Get(url)
		if err != nil {
			continue // Skip this topic on error
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue // Skip this topic on HTTP error
		}

		var apiResponse struct {
			Feed []struct {
				Title                 string `json:"title"`
				URL                   string `json:"url"`
				Summary               string `json:"summary"`
				Source                string `json:"source"`
				TimePublished         string `json:"time_published"`
				OverallSentimentScore string `json:"overall_sentiment_score"`
				TickerSentiment       []struct {
					Ticker    string `json:"ticker"`
					Relevance string `json:"relevance"`
				} `json:"ticker_sentiment"`
			} `json:"feed"`
			Information string `json:"Information"`
			Error       string `json:"Error"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			continue // Skip this topic on parse error
		}

		// Check for API errors or information messages
		if apiResponse.Error != "" {
			fmt.Printf("Alpha Vantage API Error: %s\n", apiResponse.Error)
			continue
		}
		if apiResponse.Information != "" {
			// Check if this is a rate limit message
			if strings.Contains(apiResponse.Information, "premium plans") || strings.Contains(apiResponse.Information, "rate limit") {
				fmt.Printf("Alpha Vantage API rate limit reached. Using fallback news.\n")
				// Return mock news when rate limited
				return av.getMockNews(assets, limit), nil
			} else {
				fmt.Printf("Alpha Vantage API Info: %s\n", apiResponse.Information)
				continue
			}
		}

		// Convert API response to NewsArticle
		for _, item := range apiResponse.Feed {
			// Parse time_published format (YYYYMMDDTHHMMSS)
			publishedTime, err := time.Parse("20060102T150405", item.TimePublished)
			if err != nil {
				publishedTime = time.Now() // Fallback to current time
			}

			// Calculate relevance based on asset mentions
			relevance := av.calculateRelevance(item.TickerSentiment, assets)

			article := NewsArticle{
				Title:       item.Title,
				URL:         item.URL,
				Summary:     av.truncateSummary(item.Summary),
				Source:      item.Source,
				PublishedAt: publishedTime,
				Relevance:   relevance,
			}

			allArticles = append(allArticles, article)
		}

		// Add small delay between API calls to be respectful
		time.Sleep(100 * time.Millisecond)
	}

	// Sort by relevance and published time, then limit
	av.sortArticles(allArticles)
	if len(allArticles) > limit {
		allArticles = allArticles[:limit]
	}

	return allArticles, nil
}

// calculateRelevance calculates how relevant an article is to the wallet assets
func (av *AlphaVantageClient) calculateRelevance(tickerSentiments []struct {
	Ticker    string `json:"ticker"`
	Relevance string `json:"relevance"`
}, assets []string) float64 {
	totalRelevance := 0.0
	count := 0

	for _, ts := range tickerSentiments {
		for _, asset := range assets {
			if strings.Contains(strings.ToUpper(ts.Ticker), strings.ToUpper(asset)) {
				if relevance, err := parseRelevance(ts.Relevance); err == nil {
					totalRelevance += relevance
					count++
				}
			}
		}
	}

	if count == 0 {
		return 0.5 // Default relevance
	}
	return totalRelevance / float64(count)
}

// parseRelevance parses the relevance score from string
func parseRelevance(relevanceStr string) (float64, error) {
	var relevance float64
	_, err := fmt.Sscanf(relevanceStr, "%f", &relevance)
	return relevance, err
}

// fetchNewsMCP fetches news using the MCP client
func (av *AlphaVantageClient) fetchNewsMCP(assets []string, limit int) ([]NewsArticle, error) {
	// Map assets to ticker symbols for Alpha Vantage
	var tickers []string
	for _, asset := range assets {
		switch strings.ToUpper(asset) {
		case "XLM":
			tickers = append(tickers, "XLM")
		case "USDC":
			tickers = append(tickers, "USDC")
		case "BTC", "BITCOIN":
			tickers = append(tickers, "BTC")
		case "ETH", "ETHEREUM":
			tickers = append(tickers, "ETH")
		default:
			// Include the asset as-is if not specifically mapped
			tickers = append(tickers, strings.ToUpper(asset))
		}
	}

	// Join tickers with comma for Alpha Vantage API
	tickerStr := strings.Join(tickers, ",")
	if tickerStr == "" {
		tickerStr = "CRYPTO" // Default fallback
	}

	articles, err := av.mcpClient.FetchNewsMCP(tickerStr, limit)
	if err != nil {
		fmt.Printf("MCP fetch failed: %v. Using fallback news.\n", err)
		return av.getMockNews(assets, limit), nil
	}

	// If no articles returned, use fallback
	if len(articles) == 0 {
		return av.getMockNews(assets, limit), nil
	}

	// Update relevance based on asset matching
	for i := range articles {
		articles[i].Relevance = av.calculateArticleRelevance(articles[i], assets)
	}

	// Sort by relevance
	av.sortArticles(articles)

	return articles, nil
}

// calculateArticleRelevance calculates relevance based on article content and assets
func (av *AlphaVantageClient) calculateArticleRelevance(article NewsArticle, assets []string) float64 {
	titleUpper := strings.ToUpper(article.Title)
	summaryUpper := strings.ToUpper(article.Summary)

	relevance := 0.5 // Base relevance

	for _, asset := range assets {
		assetUpper := strings.ToUpper(asset)

		// High relevance if asset is in title
		if strings.Contains(titleUpper, assetUpper) {
			relevance += 0.3
		}

		// Medium relevance if asset is in summary
		if strings.Contains(summaryUpper, assetUpper) {
			relevance += 0.2
		}
	}

	// Cap at 1.0
	if relevance > 1.0 {
		relevance = 1.0
	}

	return relevance
}

// truncateSummary limits summary length for display
func (av *AlphaVantageClient) truncateSummary(summary string) string {
	if len(summary) > 150 {
		return summary[:147] + "..."
	}
	return summary
}

// sortArticles sorts articles by relevance and publication time
func (av *AlphaVantageClient) sortArticles(articles []NewsArticle) {
	// Sort by relevance (descending) then by time (descending)
	for i := 0; i < len(articles)-1; i++ {
		for j := i + 1; j < len(articles); j++ {
			if articles[i].Relevance < articles[j].Relevance ||
				(articles[i].Relevance == articles[j].Relevance && articles[i].PublishedAt.Before(articles[j].PublishedAt)) {
				articles[i], articles[j] = articles[j], articles[i]
			}
		}
	}
}

// getMockNews provides mock news when API key is not available
func (av *AlphaVantageClient) getMockNews(assets []string, limit int) []NewsArticle {
	mockArticles := []NewsArticle{
		{
			Title:       "Stellar Development Foundation Announces New Partnership",
			URL:         "https://stellar.org/blog",
			Summary:     "SDF partners with major financial institution to expand XLM adoption...",
			Source:      "Stellar Blog",
			PublishedAt: time.Now().Add(-2 * time.Hour),
			Relevance:   0.9,
		},
		{
			Title:       "USDC Stablecoin Market Cap Reaches New High",
			URL:         "https://circle.com/blog",
			Summary:     "Circle's USDC continues to dominate the stablecoin market with record usage...",
			Source:      "Circle Blog",
			PublishedAt: time.Now().Add(-4 * time.Hour),
			Relevance:   0.8,
		},
		{
			Title:       "DeFi Protocols See Increased Activity on Stellar Network",
			URL:         "https://defipulse.com/blog",
			Summary:     "Decentralized finance applications on Stellar show significant growth...",
			Source:      "DeFi Pulse",
			PublishedAt: time.Now().Add(-6 * time.Hour),
			Relevance:   0.7,
		},
		{
			Title:       "Cryptocurrency Market Shows Positive Sentiment",
			URL:         "https://coindesk.com",
			Summary:     "Overall crypto market sentiment improves as institutional adoption continues...",
			Source:      "CoinDesk",
			PublishedAt: time.Now().Add(-8 * time.Hour),
			Relevance:   0.6,
		},
		{
			Title:       "Blockchain Technology Adoption Accelerates in 2024",
			URL:         "https://techcrunch.com",
			Summary:     "Enterprise blockchain adoption accelerates as more companies implement DLT solutions...",
			Source:      "TechCrunch",
			PublishedAt: time.Now().Add(-12 * time.Hour),
			Relevance:   0.5,
		},
	}

	if len(mockArticles) > limit {
		mockArticles = mockArticles[:limit]
	}

	return mockArticles
}

// ─────────────────────────────────────────────
// StellarCarbon — Carbon Credits
// ─────────────────────────────────────────────

type StellarCarbonClient struct{}

func NewStellarCarbonClient() *StellarCarbonClient {
	return &StellarCarbonClient{}
}

// IssueCredit tokenizes a carbon credit on Stellar
func (sc *StellarCarbonClient) IssueCredit(
	issuer string,
	amountTonnes float64,
	vintage int,
	standard string,
) (*models.CarbonCredit, error) {
	if amountTonnes <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if vintage < 2000 || vintage > time.Now().Year() {
		return nil, fmt.Errorf("invalid vintage year: %d", vintage)
	}

	credit := &models.CarbonCredit{
		Provider: "StellarCarbon",
		TokenID:  "SCT-" + mpCrypto.RandomHex(8),
		Amount:   amountTonnes,
		Vintage:  vintage,
		Standard: standard,
		Retired:  false,
		TxHash:   mpCrypto.StellarTxHash(),
	}
	return credit, nil
}

// RetireCredit retires (burns) a carbon credit
func (sc *StellarCarbonClient) RetireCredit(credit *models.CarbonCredit, beneficiary string) (*models.CarbonCredit, error) {
	if credit.Retired {
		return nil, fmt.Errorf("credit %s is already retired", credit.TokenID)
	}
	retired := *credit
	now := time.Now().UTC()
	retired.Retired = true
	retired.RetiredAt = &now
	retired.TxHash = mpCrypto.StellarTxHash()
	return &retired, nil
}

// ListCredits simulates fetching available credits
func (sc *StellarCarbonClient) ListCredits(ownerAddress string) ([]*models.CarbonCredit, error) {
	return []*models.CarbonCredit{
		{
			Provider: "StellarCarbon",
			TokenID:  "SCT-" + mpCrypto.RandomHex(8),
			Amount:   10.5,
			Vintage:  2023,
			Standard: "VCS",
			Retired:  false,
		},
		{
			Provider: "StellarCarbon",
			TokenID:  "SCT-" + mpCrypto.RandomHex(8),
			Amount:   5.25,
			Vintage:  2022,
			Standard: "Gold Standard",
			Retired:  false,
		},
	}, nil
}

// ─────────────────────────────────────────────
// Integration Registry
// ─────────────────────────────────────────────

type Integration struct {
	Name        string
	Enabled     bool
	Status      string
	Description string
	Protocol    string
}

func ListIntegrations() []Integration {
	return []Integration{
		{
			Name:        "StellarCarbon",
			Enabled:     true,
			Status:      "active",
			Description: "Carbon credit tokenisation & retirement on Stellar",
			Protocol:    "Stellar SEP-41",
		},
		{
			Name:        "x402",
			Enabled:     true,
			Status:      "active",
			Description: "HTTP 402 native pay-per-use micropayment protocol",
			Protocol:    "HTTP/2 + Stellar",
		},
		{
			Name:        "Tempo",
			Enabled:     true,
			Status:      "active",
			Description: "Global FX & remittance corridors via Stellar network",
			Protocol:    "Stellar + SEPA/SWIFT",
		},
		{
			Name:        "Alpha Vantage",
			Enabled:     true,
			Status:      "active",
			Description: "Market news & sentiment analysis for crypto assets",
			Protocol:    "REST API",
		},
		{
			Name:        "Finnhub",
			Enabled:     true,
			Status:      "active",
			Description: "Real-time market news and financial data",
			Protocol:    "REST API",
		},
		{
			Name:        "Tansu",
			Enabled:     true,
			Status:      "active",
			Description: "Decentralized project governance & versioning on Stellar",
			Protocol:    "Soroban RPC",
		},
	}
}

func PingIntegration(name string) (bool, int64) {
	start := time.Now()
	// Simulate latency
	latency := int64(20 + rand.Intn(80))
	_ = start
	return true, latency
}

// ─────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────

func scoreToGrade(score int) string {
	switch {
	case score >= 900:
		return "AAA"
	case score >= 800:
		return "AA"
	case score >= 700:
		return "A"
	case score >= 600:
		return "BBB"
	case score >= 500:
		return "BB"
	default:
		return "B"
	}
}

func gradeToRisk(grade string) string {
	switch grade {
	case "AAA", "AA":
		return "LOW"
	case "A", "BBB":
		return "MEDIUM"
	default:
		return "HIGH"
	}
}
