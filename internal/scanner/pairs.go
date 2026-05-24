package scanner

import "github.com/ogtechnologies/mozartpay/internal/models"

// AssetPair represents a liquid trading pair for arbitrage scanning
type AssetPair struct {
	Name        string
	BaseAsset   string // e.g., "XLM"
	QuoteAsset  string // e.g., "USDC"
	BaseIssuer  string // Empty for XLM
	QuoteIssuer string // Asset issuer address
	Network     models.Network
	TestAmount  string // Default amount for testing
}

// Mainnet liquid pairs - verified working on Stellar mainnet
var MainnetLiquidPairs = []AssetPair{
	{
		Name:        "USDC_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "USDC",
		BaseIssuer:  "",
		QuoteIssuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "10",
	},
	{
		Name:        "yXLM_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "yXLM",
		BaseIssuer:  "",
		QuoteIssuer: "GARDNV3Q7YGT4AKSDF25LT32YSCCW4EV22Y2TV3I2PU2MMXJTEDL5T55",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "10",
	},
	{
		Name:        "XRF_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "XRF",
		BaseIssuer:  "",
		QuoteIssuer: "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "10",
	},
	{
		Name:        "XRF_USDC",
		BaseAsset:   "USDC",
		QuoteAsset:  "XRF",
		BaseIssuer:  "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		QuoteIssuer: "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "5",
	},
	// Reverse pairs for bid/ask arbitrage
	{
		Name:        "USDC_XLM_reverse",
		BaseAsset:   "USDC",
		QuoteAsset:  "XLM",
		BaseIssuer:  "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		QuoteIssuer: "",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "5",
	},
	{
		Name:        "yXLM_XLM_reverse",
		BaseAsset:   "yXLM",
		QuoteAsset:  "XLM",
		BaseIssuer:  "GARDNV3Q7YGT4AKSDF25LT32YSCCW4EV22Y2TV3I2PU2MMXJTEDL5T55",
		QuoteIssuer: "",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "5",
	},
	{
		Name:        "XRF_XLM_reverse",
		BaseAsset:   "XRF",
		QuoteAsset:  "XLM",
		BaseIssuer:  "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		QuoteIssuer: "",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "50",
	},
	{
		Name:        "XRF_USDC_reverse",
		BaseAsset:   "XRF",
		QuoteAsset:  "USDC",
		BaseIssuer:  "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		QuoteIssuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
		Network:     models.NetworkStellarMainnet,
		TestAmount:  "50",
	},
}

// Testnet liquid pairs (using testnet issuers)
var TestnetLiquidPairs = []AssetPair{
	{
		Name:        "USDC_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "USDC",
		BaseIssuer:  "",
		QuoteIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "10",
	},
	{
		Name:        "EURC_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "EURC",
		BaseIssuer:  "",
		QuoteIssuer: "GAKMOVSF35IPK5HTDN4B3ITIR5R4AX6PZFAXNPJFDHNIUQKDT5O6G2E",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "10",
	},
	{
		Name:        "XRF_XLM",
		BaseAsset:   "XLM",
		QuoteAsset:  "XRF",
		BaseIssuer:  "",
		QuoteIssuer: "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "10",
	},
	{
		Name:        "XRF_USDC",
		BaseAsset:   "USDC",
		QuoteAsset:  "XRF",
		BaseIssuer:  "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		QuoteIssuer: "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "5",
	},
	// Reverse pairs for bid/ask arbitrage
	{
		Name:        "XRF_XLM_reverse",
		BaseAsset:   "XRF",
		QuoteAsset:  "XLM",
		BaseIssuer:  "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		QuoteIssuer: "",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "50",
	},
	{
		Name:        "XRF_USDC_reverse",
		BaseAsset:   "XRF",
		QuoteAsset:  "USDC",
		BaseIssuer:  "GCHI6I3X62UDMMIWZGPCRHBLYOUCC4EJM22IP5GA6CGO6UP3DBMFCNHF",
		QuoteIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
		Network:     models.NetworkStellarTestnet,
		TestAmount:  "50",
	},
}

// GetPairsForNetwork returns the appropriate pairs for a given network
func GetPairsForNetwork(network models.Network) []AssetPair {
	switch network {
	case models.NetworkStellarMainnet:
		return MainnetLiquidPairs
	case models.NetworkStellarTestnet:
		return TestnetLiquidPairs
	default:
		return MainnetLiquidPairs
	}
}

// GetPairByName returns a specific pair by name
func GetPairByName(network models.Network, name string) (AssetPair, bool) {
	pairs := GetPairsForNetwork(network)
	for _, p := range pairs {
		if p.Name == name {
			return p, true
		}
	}
	return AssetPair{}, false
}
