// Package training provides synthetic data generation for fine-tuning
package training

import (
	"fmt"
	"math/rand"
	"strings"
)

// SyntheticGenerator generates augmented training data from existing patterns
type SyntheticGenerator struct {
	rng *rand.Rand
}

// NewSyntheticGenerator creates a new synthetic data generator
func NewSyntheticGenerator(seed int64) *SyntheticGenerator {
	return &SyntheticGenerator{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// IntentPattern defines patterns for generating synthetic intent examples
type IntentPattern struct {
	Intent     string
	Category   string
	Templates  []string
	Variations map[string][]string
	Parameters map[string][]string
}

// Common intent patterns for MozartPay
var DefaultIntentPatterns = []IntentPattern{
	{
		Intent:   "greeting",
		Category: "system",
		Templates: []string{
			"{greeting}",
			"{greeting}, how are you",
			"{greeting} there",
		},
		Variations: map[string][]string{
			"greeting": {"hello", "hi", "hey", "good morning", "good afternoon", "good evening",
				"greetings", "what's up", "yo", "hiya"},
		},
	},
	{
		Intent:   "help",
		Category: "system",
		Templates: []string{
			"{help}",
			"can you {help}",
			"I need {help}",
		},
		Variations: map[string][]string{
			"help": {"help", "show me what you can do", "what are your commands",
				"how do I use this", "what can you do", "list commands"},
		},
	},
	{
		Intent:   "wallet_balance",
		Category: "wallet",
		Templates: []string{
			"{check} my balance",
			"{check} balance",
			"what's my {balance}",
			"show me my {balance}",
			"how much {xlm} do I have",
		},
		Variations: map[string][]string{
			"check":   {"check", "what's", "show me", "tell me", "view"},
			"balance": {"balance", "account balance", "wallet balance"},
			"xlm":     {"XLM", "Lumens", "Stellar"},
		},
	},
	{
		Intent:   "pay_send",
		Category: "payment",
		Templates: []string{
			"{send} {amount} {asset} to {address}",
			"{pay} {address} {amount} {asset}",
			"{transfer} {amount} {asset} to {address}",
			"{send} {address} {amount} {asset}",
		},
		Parameters: map[string][]string{
			"send":     {"send", "pay", "transfer", "give"},
			"pay":      {"pay", "send", "transfer"},
			"transfer": {"transfer", "send", "move"},
			"amount":   {"10", "100", "1000", "50", "500", "0.5", "5"},
			"asset":    {"XLM", "USDC", "EURC", "BTC", "ETH"},
			"address":  {"GABC123...", "GXYZ789...", "recipient address", "their wallet"},
		},
	},
	{
		Intent:   "swap_quote",
		Category: "swap",
		Templates: []string{
			"{swap} {amount} {from} {to} {dest}",
			"{exchange} {amount} {from} for {dest}",
			"{convert} {amount} {from} {to} {dest}",
			"what's the rate for {amount} {from} {to} {dest}",
		},
		Parameters: map[string][]string{
			"swap":     {"swap", "exchange", "convert", "trade"},
			"exchange": {"exchange", "swap", "convert"},
			"convert":  {"convert", "swap", "exchange"},
			"amount":   {"100", "50", "1000", "10", "500"},
			"from":     {"XLM", "USDC"},
			"to":       {"to", "for", "into"},
			"dest":     {"USDC", "EURC", "XLM", "BTC"},
		},
	},
	{
		Intent:   "create_trustline",
		Category: "wallet",
		Templates: []string{
			"{add} trustline for {asset}",
			"{trust} {asset}",
			"{enable} {asset}",
			"{create} trustline to {asset}",
		},
		Parameters: map[string][]string{
			"add":    {"add", "create", "set up"},
			"trust":  {"trust", "accept", "enable"},
			"enable": {"enable", "add", "activate"},
			"create": {"create", "add", "set up"},
			"asset":  {"USDC", "EURC", "BTC", "ETH", "yXLM"},
		},
	},
	{
		Intent:   "triangular_scan",
		Category: "arbitrage",
		Templates: []string{
			"{scan} for {arbitrage}",
			"{find} {triangular} {opportunities}",
			"{check} for {arbitrage} {opportunities}",
			"{run} {triangular} {scan}",
		},
		Parameters: map[string][]string{
			"scan":          {"scan", "look", "search", "check"},
			"find":          {"find", "look for", "search for", "get"},
			"check":         {"check", "scan", "look"},
			"run":           {"run", "execute", "start", "do"},
			"arbitrage":     {"arbitrage", "arbitrage opportunities", "trading opportunities"},
			"triangular":    {"triangular", "triangle", "three-way"},
			"opportunities": {"opportunities", "trades", "deals", "chances"},
		},
	},
}

// GenerateIntentExamples creates synthetic examples for intent classification
func (g *SyntheticGenerator) GenerateIntentExamples(pattern IntentPattern, count int) []TrainingExample {
	examples := make([]TrainingExample, 0, count)

	for i := 0; i < count; i++ {
		message := g.generateFromPattern(pattern)
		confidence := 0.85 + g.rng.Float64()*0.14 // 0.85-0.99 confidence

		ex := TrainingExample{
			Instruction: "Classify the user intent for this MozartPay CLI command. Respond with ONLY a JSON object: {\"intent\": \"NAME\", \"confidence\": 0.0-1.0}",
			Input:       message,
			Output:      fmt.Sprintf(`{"intent": "%s", "confidence": %.2f}`, pattern.Intent, confidence),
			System:      "You are an intent classifier for MozartPay, a cryptocurrency payment CLI.",
		}
		examples = append(examples, ex)
	}

	return examples
}

// generateFromPattern generates a message from a pattern template
func (g *SyntheticGenerator) generateFromPattern(pattern IntentPattern) string {
	if len(pattern.Templates) == 0 {
		return ""
	}

	template := pattern.Templates[g.rng.Intn(len(pattern.Templates))]
	result := template

	// Replace variations
	for key, variations := range pattern.Variations {
		placeholder := "{" + key + "}"
		if strings.Contains(result, placeholder) {
			replacement := variations[g.rng.Intn(len(variations))]
			result = strings.Replace(result, placeholder, replacement, -1)
		}
	}

	// Replace parameters
	for key, values := range pattern.Parameters {
		placeholder := "{" + key + "}"
		if strings.Contains(result, placeholder) {
			replacement := values[g.rng.Intn(len(values))]
			result = strings.Replace(result, placeholder, replacement, -1)
		}
	}

	return result
}

// AugmentWithParaphrasing creates paraphrased variations of existing examples
func (g *SyntheticGenerator) AugmentWithParaphrasing(examples []TrainingExample, multiplier int) []TrainingExample {
	augmented := make([]TrainingExample, 0, len(examples)*multiplier)

	paraphrasePatterns := map[string][]string{
		"show":    {"display", "view", "see", "list", "show me"},
		"send":    {"transfer", "pay", "give", "move", "send"},
		"check":   {"check", "look at", "view", "see", "get"},
		"my":      {"my", "the", ""},
		"balance": {"balance", "amount", "holdings", "funds"},
	}

	for _, ex := range examples {
		augmented = append(augmented, ex) // Keep original

		for i := 0; i < multiplier-1; i++ {
			paraphrased := ex
			input := ex.Input

			// Apply random word substitutions
			for word, replacements := range paraphrasePatterns {
				if strings.Contains(input, word) && g.rng.Float32() > 0.5 {
					replacement := replacements[g.rng.Intn(len(replacements))]
					input = strings.Replace(input, word, replacement, 1)
				}
			}

			paraphrased.Input = input
			augmented = append(augmented, paraphrased)
		}
	}

	return augmented
}

// AddNoiseToExamples introduces minor variations to prevent overfitting
func (g *SyntheticGenerator) AddNoiseToExamples(examples []TrainingExample, noiseLevel float64) []TrainingExample {
	noisy := make([]TrainingExample, len(examples))
	copy(noisy, examples)

	noisePatterns := []struct {
		find    string
		replace string
	}{
		{"USDC", "usdc"},
		{"XLM", "xlm"},
		{"100", "100.0"},
		{"to ", "to: "},
		{"  ", " "},
	}

	for i := range noisy {
		if g.rng.Float64() < noiseLevel {
			noise := noisePatterns[g.rng.Intn(len(noisePatterns))]
			noisy[i].Input = strings.Replace(noisy[i].Input, noise.find, noise.replace, 1)
		}
	}

	return noisy
}

// GenerateParameterExamples creates synthetic examples for parameter extraction
func (g *SyntheticGenerator) GenerateParameterExamples(intent string, count int) []TrainingExample {
	examples := make([]TrainingExample, 0, count)

	parameterPatterns := map[string][]map[string]interface{}{
		"pay_send": {
			{"destination": "GABC123DEF456", "amount": "100", "asset": "XLM"},
			{"destination": "GXYZ789ABC012", "amount": "50.5", "asset": "USDC"},
			{"destination": "recipient address", "amount": "1000", "asset": "EURC"},
		},
		"swap_quote": {
			{"from": "XLM", "to": "USDC", "amount": "100"},
			{"from": "USDC", "to": "EURC", "amount": "500"},
			{"from": "XLM", "to": "BTC", "amount": "1000"},
		},
		"create_trustline": {
			{"code": "USDC", "issuer": nil},
			{"code": "EURC", "issuer": nil},
			{"code": "BTC", "issuer": "GABC123..."},
		},
	}

	patterns, ok := parameterPatterns[intent]
	if !ok {
		return examples
	}

	for i := 0; i < count; i++ {
		params := patterns[g.rng.Intn(len(patterns))]
		message := g.generateMessageFromParams(intent, params)

		output, _ := jsonMarshal(params)

		ex := TrainingExample{
			Instruction: fmt.Sprintf("Extract parameters from the message for intent '%s'. Respond with JSON.", intent),
			Input:       message,
			Output:      string(output),
			System:      "You are a parameter extractor for MozartPay, a cryptocurrency payment CLI.",
		}
		examples = append(examples, ex)
	}

	return examples
}

// generateMessageFromParams generates a natural language message from parameters
func (g *SyntheticGenerator) generateMessageFromParams(intent string, params map[string]interface{}) string {
	switch intent {
	case "pay_send":
		amount, _ := params["amount"].(string)
		asset, _ := params["asset"].(string)
		dest, _ := params["destination"].(string)
		templates := []string{
			fmt.Sprintf("send %s %s to %s", amount, asset, dest),
			fmt.Sprintf("pay %s %s %s", dest, amount, asset),
			fmt.Sprintf("transfer %s %s to %s", amount, asset, dest),
		}
		return templates[g.rng.Intn(len(templates))]
	case "swap_quote":
		amount, _ := params["amount"].(string)
		from, _ := params["from"].(string)
		to, _ := params["to"].(string)
		templates := []string{
			fmt.Sprintf("swap %s %s to %s", amount, from, to),
			fmt.Sprintf("exchange %s %s for %s", amount, from, to),
			fmt.Sprintf("convert %s %s to %s", amount, from, to),
		}
		return templates[g.rng.Intn(len(templates))]
	case "create_trustline":
		code, _ := params["code"].(string)
		templates := []string{
			fmt.Sprintf("add trustline for %s", code),
			fmt.Sprintf("trust %s", code),
			fmt.Sprintf("enable %s", code),
		}
		return templates[g.rng.Intn(len(templates))]
	}
	return ""
}

// jsonMarshal is a helper to marshal params
func jsonMarshal(v interface{}) ([]byte, error) {
	// Simple implementation - in real code use encoding/json
	if m, ok := v.(map[string]interface{}); ok {
		parts := make([]string, 0, len(m))
		for k, val := range m {
			if val == nil {
				parts = append(parts, fmt.Sprintf("\"%s\": null", k))
			} else if s, ok := val.(string); ok {
				parts = append(parts, fmt.Sprintf("\"%s\": \"%s\"", k, s))
			}
		}
		return []byte("{" + strings.Join(parts, ", ") + "}"), nil
	}
	return nil, fmt.Errorf("unsupported type")
}

// GenerateFullDataset creates a complete training dataset with all intents
func (g *SyntheticGenerator) GenerateFullDataset(intentCount, paramCount int) *Dataset {
	builder := NewDatasetBuilder("mozartpay_synthetic", "1.0.0", "Synthetic training data for MozartPay intent classification and parameter extraction")

	// Generate intent classification examples
	for _, pattern := range DefaultIntentPatterns {
		examples := g.GenerateIntentExamples(pattern, intentCount)
		for _, ex := range examples {
			builder.examples = append(builder.examples, ex)
		}
	}

	// Generate parameter extraction examples
	paramIntents := []string{"pay_send", "swap_quote", "create_trustline"}
	for _, intent := range paramIntents {
		examples := g.GenerateParameterExamples(intent, paramCount)
		for _, ex := range examples {
			builder.examples = append(builder.examples, ex)
		}
	}

	// Apply augmentation
	builder.examples = g.AugmentWithParaphrasing(builder.examples, 2)
	builder.examples = g.AddNoiseToExamples(builder.examples, 0.1)

	return builder.Build()
}
