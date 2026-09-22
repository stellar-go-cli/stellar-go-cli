package integrations

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FinnhubClient handles Finnhub API interactions for news
type FinnhubClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// FinnhubNewsItem represents a single news article from Finnhub API
type FinnhubNewsItem struct {
	Category string `json:"category"`
	Datetime int64  `json:"datetime"`
	Headline string `json:"headline"`
	ID       int    `json:"id"`
	Image    string `json:"image"`
	Related  string `json:"related"`
	Source   string `json:"source"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

// NewFinnhubClient creates a new Finnhub API client
func NewFinnhubClient(apiKey string) *FinnhubClient {
	return &FinnhubClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://finnhub.io/api/v1",
	}
}

// AssetToSymbol maps wallet assets to Finnhub-compatible symbols
func (fc *FinnhubClient) AssetToSymbol(asset string) string {
	switch strings.ToUpper(asset) {
	case "XLM":
		return "XLM"
	case "USDC":
		return "USDC"
	case "BTC", "BITCOIN":
		return "BTC"
	case "ETH", "ETHEREUM":
		return "ETH"
	case "SOL", "SOLANA":
		return "SOL"
	case "ADA", "CARDANO":
		return "ADA"
	case "DOT", "POLKADOT":
		return "DOT"
	case "LINK", "CHAINLINK":
		return "LINK"
	default:
		// Return the asset as-is for unknown assets
		return strings.ToUpper(asset)
	}
}

// FetchNews fetches news articles for the given assets
func (fc *FinnhubClient) FetchNews(assets []string, limit int) ([]NewsArticle, error) {
	if fc.apiKey == "" {
		// Return mock news if no API key is configured
		return fc.getMockNews(assets, limit), nil
	}

	var allArticles []NewsArticle

	// Calculate date range (last 7 days to now)
	to := time.Now().Format("2006-01-02")
	from := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	// Fetch news for each asset
	for _, asset := range assets {
		symbol := fc.AssetToSymbol(asset)

		// Construct API request
		url := fmt.Sprintf("%s/company-news?symbol=%s&from=%s&to=%s&token=%s",
			fc.baseURL, symbol, from, to, fc.apiKey)

		resp, err := fc.httpClient.Get(url)
		if err != nil {
			continue // Skip this asset on error
		}
		defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort close

		if resp.StatusCode != http.StatusOK {
			continue // Skip this asset on HTTP error
		}

		var newsItems []FinnhubNewsItem
		if err := json.NewDecoder(resp.Body).Decode(&newsItems); err != nil {
			continue // Skip on parse error
		}

		// Convert Finnhub news items to NewsArticle
		for _, item := range newsItems {
			article := NewsArticle{
				Title:       item.Headline,
				URL:         item.URL,
				Summary:     fc.truncateSummary(item.Summary),
				Source:      item.Source,
				PublishedAt: time.Unix(item.Datetime, 0),
				Relevance:   fc.calculateRelevance(item, assets),
			}
			allArticles = append(allArticles, article)
		}

		// Add small delay between API calls to respect rate limits
		time.Sleep(100 * time.Millisecond)
	}

	// Sort by relevance and published time, then limit
	fc.sortArticles(allArticles)
	if len(allArticles) > limit {
		allArticles = allArticles[:limit]
	}

	return allArticles, nil
}

// calculateRelevance determines how relevant a news article is to the assets
func (fc *FinnhubClient) calculateRelevance(item FinnhubNewsItem, assets []string) float64 {
	// Base relevance
	relevance := 0.5

	// Check if any asset is mentioned in the headline or summary
	headlineLower := strings.ToLower(item.Headline)
	summaryLower := strings.ToLower(item.Summary)

	for _, asset := range assets {
		assetLower := strings.ToLower(asset)
		symbol := strings.ToLower(fc.AssetToSymbol(asset))

		// High relevance if mentioned in headline
		if strings.Contains(headlineLower, assetLower) || strings.Contains(headlineLower, symbol) {
			relevance += 0.3
		}

		// Medium relevance if mentioned in summary
		if strings.Contains(summaryLower, assetLower) || strings.Contains(summaryLower, symbol) {
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
func (fc *FinnhubClient) truncateSummary(summary string) string {
	if len(summary) > 150 {
		return summary[:147] + "..."
	}
	return summary
}

// sortArticles sorts articles by relevance (descending) then by time (descending)
func (fc *FinnhubClient) sortArticles(articles []NewsArticle) {
	for i := 0; i < len(articles)-1; i++ {
		for j := i + 1; j < len(articles); j++ {
			if articles[i].Relevance < articles[j].Relevance ||
				(articles[i].Relevance == articles[j].Relevance && articles[i].PublishedAt.Before(articles[j].PublishedAt)) {
				articles[i], articles[j] = articles[j], articles[i]
			}
		}
	}
}

// GetMockNews exposes mock news generation for fallback
func (fc *FinnhubClient) GetMockNews(assets []string, limit int) []NewsArticle {
	return fc.getMockNews(assets, limit)
}

// getMockNews provides mock news when API key is not available or rate limited
func (fc *FinnhubClient) getMockNews(assets []string, limit int) []NewsArticle {
	mockArticles := []NewsArticle{
		{
			Title:       "Crypto Markets Show Strong Recovery After Recent Dip",
			URL:         "https://finnhub.io/news",
			Summary:     "Major cryptocurrencies including BTC and ETH are showing signs of recovery...",
			Source:      "Finnhub",
			PublishedAt: time.Now().Add(-1 * time.Hour),
			Relevance:   0.9,
		},
		{
			Title:       "Stellar Network Processes Record Transaction Volume",
			URL:         "https://finnhub.io/news",
			Summary:     "The Stellar network has achieved a new milestone in daily transaction processing...",
			Source:      "Finnhub",
			PublishedAt: time.Now().Add(-3 * time.Hour),
			Relevance:   0.85,
		},
		{
			Title:       "Stablecoin Adoption Accelerates in DeFi Markets",
			URL:         "https://finnhub.io/news",
			Summary:     "USDC and other stablecoins see increased usage across decentralized finance protocols...",
			Source:      "Finnhub",
			PublishedAt: time.Now().Add(-5 * time.Hour),
			Relevance:   0.8,
		},
		{
			Title:       "Institutional Interest in Digital Assets Continues to Grow",
			URL:         "https://finnhub.io/news",
			Summary:     "New data shows increasing institutional investment in cryptocurrency markets...",
			Source:      "Finnhub",
			PublishedAt: time.Now().Add(-7 * time.Hour),
			Relevance:   0.75,
		},
		{
			Title:       "Cross-Chain Interoperability Solutions Gain Traction",
			URL:         "https://finnhub.io/news",
			Summary:     "New protocols aim to bridge different blockchain networks more efficiently...",
			Source:      "Finnhub",
			PublishedAt: time.Now().Add(-9 * time.Hour),
			Relevance:   0.7,
		},
	}

	if len(mockArticles) > limit {
		mockArticles = mockArticles[:limit]
	}

	return mockArticles
}

// IsConfigured returns whether the client has an API key configured
func (fc *FinnhubClient) IsConfigured() bool {
	return fc.apiKey != ""
}
