package chat

import (
	"testing"

	"github.com/ogtechnologies/mozartpay/internal/config"
)

func TestLLMClientIsAvailable(t *testing.T) {
	// Test with no model path
	cfg := config.LLMConfig{
		Enabled:   true,
		ModelPath: "",
	}
	client := NewLLMClient(cfg)
	if client.IsAvailable() {
		t.Error("Expected IsAvailable() to return false when model path is empty")
	}

	// Test with non-existent model path
	cfg.ModelPath = "/nonexistent/model.gguf"
	client = NewLLMClient(cfg)
	if client.IsAvailable() {
		t.Error("Expected IsAvailable() to return false for non-existent model")
	}
}

func TestLLMClientWithDisabledConfig(t *testing.T) {
	cfg := config.LLMConfig{
		Enabled:   false,
		ModelPath: "/some/path/model.gguf",
	}
	client := NewLLMClient(cfg)
	if client.IsAvailable() {
		t.Error("Expected IsAvailable() to return false when LLM is disabled")
	}
}

func TestPromptBuilderIntentClassification(t *testing.T) {
	builder := NewPromptBuilder()
	prompt := builder.IntentClassificationPrompt("show my wallet balance")

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}

	// Check that the prompt contains expected elements
	if !contains(prompt, "intent classifier") {
		t.Error("Prompt should mention intent classifier")
	}
	if !contains(prompt, "wallet_balance") {
		t.Error("Prompt should mention wallet_balance intent")
	}
}

func TestPromptBuilderParameterExtraction(t *testing.T) {
	builder := NewPromptBuilder()
	state := NewState(config.DefaultConfig())
	prompt := builder.ParameterExtractionPrompt("swap_quote", "swap 100 XLM to USDC", state)

	if prompt == "" {
		t.Error("Expected non-empty prompt")
	}

	if !contains(prompt, "from") || !contains(prompt, "to") {
		t.Error("Prompt should mention parameter names")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`{"intent": "greeting", "confidence": 0.95}`, `{"intent": "greeting", "confidence": 0.95}`},
		{`Some text before {"key": "value"} after`, `{"key": "value"}`},
		{"No JSON here", "No JSON here"},
		{`{"nested": {"key": "value"}}`, `{"nested": {"key": "value"}}`},
	}

	for _, test := range tests {
		result := extractJSON(test.input)
		if result != test.expected {
			t.Errorf("extractJSON(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLLMIntentRecognizerCreation(t *testing.T) {
	cfg := config.LLMConfig{
		Enabled:         false,
		FallbackToRules: true,
	}

	recognizer := NewLLMIntentRecognizer(cfg)
	if recognizer == nil {
		t.Error("Expected recognizer to be created")
	}

	// When disabled, should not be available
	if recognizer.IsAvailable() {
		t.Error("Expected recognizer to not be available when disabled")
	}
}

func TestLLMIntentRecognizerWithFallback(t *testing.T) {
	cfg := config.LLMConfig{
		Enabled:         false,
		FallbackToRules: true,
	}

	recognizer := NewLLMIntentRecognizer(cfg)
	state := NewState(config.DefaultConfig())

	// Test that fallback works for a simple greeting
	intent := recognizer.UnderstandIntent("hello", state)
	if intent.Name != "greeting" {
		t.Errorf("Expected 'greeting' intent, got '%s'", intent.Name)
	}
}

func TestIntentRecognizerInterface(t *testing.T) {
	// Test that both recognizers implement the interface
	var _ IntentRecognizerInterface = NewIntentRecognizer()
	var _ IntentRecognizerInterface = NewLLMIntentRecognizer(config.LLMConfig{})
}
