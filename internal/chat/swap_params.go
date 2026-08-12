package chat

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ogtechnologies/mozartpay/internal/models"
	"github.com/ogtechnologies/mozartpay/internal/swap"
)

// swapAssetAliases maps common user aliases to canonical asset codes
var swapAssetAliases = map[string]string{
	"USD":  "USDC",
	"EUR":  "EURC",
	"EURO": "EURC",
}

var swapPairPattern = regexp.MustCompile(`(?i)([a-z0-9]+)\s+(?:to|for|into)\s+([a-z0-9]+)`)

// supportedSwapAssets returns the set of swappable asset codes for the network
func supportedSwapAssets(net models.Network) map[string]bool {
	assets := swap.TestnetAssets
	if net == models.NetworkStellarMainnet {
		assets = swap.MainnetAssets
	}
	supported := make(map[string]bool, len(assets))
	for code := range assets {
		supported[code] = true
	}
	return supported
}

// supportedSwapAssetList returns the sorted list of swappable asset codes
func supportedSwapAssetList(net models.Network) []string {
	assets := swap.TestnetAssets
	if net == models.NetworkStellarMainnet {
		assets = swap.MainnetAssets
	}
	list := make([]string, 0, len(assets))
	for code := range assets {
		list = append(list, code)
	}
	sort.Strings(list)
	return list
}

// canonicalSwapAsset resolves a single token to a supported canonical asset code
func canonicalSwapAsset(code string, supported map[string]bool) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if alias, ok := swapAssetAliases[code]; ok {
		code = alias
	}
	if supported[code] {
		return code, true
	}
	return "", false
}

// splitAssetPair extracts a (from, to) asset pair from free text like "xlm to usd"
func splitAssetPair(input string, supported map[string]bool) (string, string, bool) {
	m := swapPairPattern.FindStringSubmatch(input)
	if len(m) != 3 {
		return "", "", false
	}
	from, okFrom := canonicalSwapAsset(m[1], supported)
	to, okTo := canonicalSwapAsset(m[2], supported)
	if !okFrom || !okTo {
		return "", "", false
	}
	return from, to, true
}

// extractAmountToken finds the first numeric token in free text
func extractAmountToken(input string) string {
	for _, tok := range strings.Fields(input) {
		if _, err := strconv.ParseFloat(tok, 64); err == nil {
			return tok
		}
	}
	return ""
}

// normalizeSwapParams cleans up free-text answers collected conversationally.
// Answers like "xlm to usd" or "50 xlm to usd" can carry multiple params.
func normalizeSwapParams(from, to, amount string, supported map[string]bool) (string, string, string) {
	rawCombined := strings.TrimSpace(from + " " + to)

	if f, t, ok := splitAssetPair(from, supported); ok {
		from = f
		if to == "" {
			to = t
		}
	}
	if _, t, ok := splitAssetPair(to, supported); ok {
		to = t
	}
	if code, ok := canonicalSwapAsset(from, supported); ok {
		from = code
	}
	if code, ok := canonicalSwapAsset(to, supported); ok {
		to = code
	}
	if amount == "" {
		amount = extractAmountToken(rawCombined)
	}
	return from, to, amount
}
