// Package chat provides an interactive chat interface for MozartPay CLI
package chat

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ogtechnologies/mozartpay/internal/config"
)

// LLMIntentRecognizer uses local LLM for intent classification
type LLMIntentRecognizer struct {
	llm           *LLMClient
	promptBuilder *PromptBuilder
	fallback      *IntentRecognizer
	config        config.LLMConfig
}

// NewLLMIntentRecognizer creates a new LLM-based intent recognizer
func NewLLMIntentRecognizer(cfg config.LLMConfig) *LLMIntentRecognizer {
	return &LLMIntentRecognizer{
		llm:           NewLLMClient(cfg),
		promptBuilder: NewPromptBuilder(),
		fallback:      NewIntentRecognizer(),
		config:        cfg,
	}
}

// IsAvailable checks if the LLM recognizer can be used
func (r *LLMIntentRecognizer) IsAvailable() bool {
	return r.llm.IsAvailable()
}

// UnderstandIntent analyzes a message and returns the recognized intent using LLM
func (r *LLMIntentRecognizer) UnderstandIntent(message string, state *State) *Intent {
	// Check if LLM is available and enabled
	if !r.IsAvailable() {
		if r.config.FallbackToRules {
			return r.fallback.UnderstandIntent(message, state)
		}
		return &Intent{Name: "unknown", Params: map[string]interface{}{}}
	}

	// Try LLM-based classification with timeout
	intent, err := r.classifyWithLLM(message, state)
	if err != nil {
		// Fallback to rule-based on error
		if r.config.FallbackToRules {
			return r.fallback.UnderstandIntent(message, state)
		}
		return &Intent{Name: "unknown", Params: map[string]interface{}{}}
	}

	// Check confidence threshold
	if intent.Confidence < 0.5 {
		// Low confidence, fallback to rules
		if r.config.FallbackToRules {
			return r.fallback.UnderstandIntent(message, state)
		}
	}

	// Extract parameters for the classified intent
	params, err := r.extractParameters(intent.Intent, message, state)
	if err != nil {
		// If parameter extraction fails, try fallback rules
		if r.config.FallbackToRules {
			fallbackIntent := r.fallback.UnderstandIntent(message, state)
			if fallbackIntent.Name != "unknown" {
				return fallbackIntent
			}
		}
	}

	return &Intent{
		Name:   intent.Intent,
		Params: params,
	}
}

// classifyWithLLM uses the LLM to classify intent
func (r *LLMIntentRecognizer) classifyWithLLM(message string, state *State) (*IntentResult, error) {
	prompt := r.promptBuilder.IntentClassificationPrompt(message)

	// Set timeout for intent classification (faster for UX)
	timeout := 500 * time.Millisecond

	output, err := r.llm.PredictWithTimeout(prompt, timeout)
	if err != nil {
		return nil, fmt.Errorf("LLM prediction failed: %w", err)
	}

	// Parse the JSON response
	var result IntentResult
	if err := r.llm.PredictJSON(prompt, &result); err != nil {
		// Try direct parsing if PredictJSON fails
		output = extractJSON(output)
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return nil, fmt.Errorf("failed to parse LLM output: %w", err)
		}
	}

	return &result, nil
}

// extractParameters uses LLM to extract parameters for the given intent
func (r *LLMIntentRecognizer) extractParameters(intentName string, message string, state *State) (map[string]interface{}, error) {
	// For simple intents without parameters, return empty
	switch intentName {
	case "greeting", "help", "quit", "cancel_operation", "wallet_list",
		"wallet_show", "wallet_balance", "wallet_assets", "show_network",
		"system_status", "system_health", "memory_list":
		return map[string]interface{}{}, nil
	}

	prompt := r.promptBuilder.ParameterExtractionPrompt(intentName, message, state)

	// Use longer timeout for parameter extraction
	timeout := 1 * time.Second

	output, err := r.llm.PredictWithTimeout(prompt, timeout)
	if err != nil {
		// Try fallback parameter extraction
		return r.fallbackExtractParameters(intentName, message), nil
	}

	// Parse the JSON response
	var result map[string]interface{}
	output = extractJSON(output)
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// Try fallback extraction on parse error
		return r.fallbackExtractParameters(intentName, message), nil
	}

	return result, nil
}

// fallbackExtractParameters uses rule-based extraction when LLM fails
func (r *LLMIntentRecognizer) fallbackExtractParameters(intentName string, message string) map[string]interface{} {
	// Use the fallback recognizer's extraction logic
	fallbackIntent := r.fallback.UnderstandIntent(message, &State{})
	if fallbackIntent.Name == intentName {
		return fallbackIntent.Params
	}
	return map[string]interface{}{}
}

// Close cleans up resources
func (r *LLMIntentRecognizer) Close() error {
	return r.llm.Close()
}

// extractJSON helper to find JSON in text
func extractJSON(output string) string {
	start := -1
	end := -1
	depth := 0

	for i, ch := range output {
		switch ch {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start != -1 {
				end = i + 1
				return output[start:end]
			}
		}
	}

	return output
}
