// Package chat provides an interactive chat interface for MozartPay CLI
package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
)

// LLMClient provides an interface for local LLM inference
type LLMClient struct {
	config      config.LLMConfig
	modelLoaded bool
	lastErr     error
}

// NewLLMClient creates a new LLM client with the given configuration
func NewLLMClient(cfg config.LLMConfig) *LLMClient {
	return &LLMClient{
		config:      cfg,
		modelLoaded: false,
	}
}

// IsAvailable checks if the LLM is properly configured and available
func (l *LLMClient) IsAvailable() bool {
	if !l.config.Enabled {
		return false
	}

	// Check Ollama configuration first
	if l.config.OllamaURL != "" && l.config.OllamaModel != "" {
		return l.isOllamaAvailable()
	}

	// Fall back to llama-cli model file check
	if l.config.ModelPath == "" {
		return false
	}

	// Expand home directory if needed
	modelPath := l.config.ModelPath
	if strings.HasPrefix(modelPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		modelPath = filepath.Join(home, modelPath[2:])
	}

	_, err := os.Stat(modelPath)
	return err == nil
}

// isOllamaAvailable checks if Ollama server is running and model exists
func (l *LLMClient) isOllamaAvailable() bool {
	url := l.config.OllamaURL
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}

	// Check if Ollama is running
	resp, err := http.Get(url + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	// Check if the specified model exists
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	for _, model := range result.Models {
		if model.Name == l.config.OllamaModel {
			return true
		}
	}

	return false
}

// Predict generates a completion for the given prompt using llama.cpp or Ollama
func (l *LLMClient) Predict(prompt string) (string, error) {
	if !l.IsAvailable() {
		return "", fmt.Errorf("LLM not available: check configuration and ensure Ollama is running or model file exists")
	}

	// Use Ollama if configured
	if l.config.OllamaURL != "" && l.config.OllamaModel != "" {
		return l.predictOllama(prompt)
	}

	// Fall back to llama-cli
	return l.predictLlamaCLI(prompt)
}

// predictOllama calls Ollama's HTTP API
func (l *LLMClient) predictOllama(prompt string) (string, error) {
	url := l.config.OllamaURL
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":  l.config.OllamaModel,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": l.config.Temperature,
			"num_ctx":     l.config.ContextSize,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make HTTP request
	resp, err := http.Post(url+"/api/generate", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	// Parse response
	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return strings.TrimSpace(result.Response), nil
}

// predictLlamaCLI uses llama-cli subprocess
func (l *LLMClient) predictLlamaCLI(prompt string) (string, error) {
	// Expand model path
	modelPath := l.config.ModelPath
	if strings.HasPrefix(modelPath, "~/") {
		home, _ := os.UserHomeDir()
		modelPath = filepath.Join(home, modelPath[2:])
	}

	// Check for llama-cli binary in common locations
	llamaCLI := l.findLlamaCLI()
	if llamaCLI == "" {
		return "", fmt.Errorf("llama-cli not found in PATH. Please install llama.cpp or add it to PATH")
	}

	// Build arguments for llama-cli
	args := []string{
		"-m", modelPath,
		"-p", prompt,
		"-n", "512", // max tokens to generate
		"--temp", fmt.Sprintf("%.2f", l.config.Temperature),
		"-c", fmt.Sprintf("%d", l.config.ContextSize),
		"-t", fmt.Sprintf("%d", l.config.Threads),
		"--no-display-prompt", // Don't echo the prompt
		"-s", "42",            // Seed for reproducibility
	}

	// Execute llama-cli
	cmd := exec.Command(llamaCLI, args...)
	cmd.Stderr = &bytes.Buffer{} // Capture stderr

	output, err := cmd.Output()
	if err != nil {
		stderr := cmd.Stderr.(*bytes.Buffer).String()
		return "", fmt.Errorf("llama-cli execution failed: %w (stderr: %s)", err, stderr)
	}

	return strings.TrimSpace(string(output)), nil
}

// PredictJSON generates a completion and parses it as JSON
func (l *LLMClient) PredictJSON(prompt string, v interface{}) error {
	output, err := l.Predict(prompt)
	if err != nil {
		return err
	}

	// Try to extract JSON from the output (in case there's extra text)
	output = l.extractJSON(output)

	return json.Unmarshal([]byte(output), v)
}

// extractJSON tries to find a JSON object in the output
func (l *LLMClient) extractJSON(output string) string {
	// Look for JSON object between braces
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")

	if start != -1 && end != -1 && end > start {
		return output[start : end+1]
	}

	return output
}

// findLlamaCLI searches for the llama-cli binary in common locations
func (l *LLMClient) findLlamaCLI() string {
	// Check PATH first
	if path, err := exec.LookPath("llama-cli"); err == nil {
		return path
	}

	// Common locations
	commonPaths := []string{
		"/usr/local/bin/llama-cli",
		"/opt/llama.cpp/llama-cli",
		"/usr/bin/llama-cli",
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		commonPaths = append(commonPaths,
			filepath.Join(home, "llama.cpp/llama-cli"),
			filepath.Join(home, "bin/llama-cli"),
		)
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// PredictWithTimeout generates a completion with a timeout
func (l *LLMClient) PredictWithTimeout(prompt string, timeout time.Duration) (string, error) {
	if !l.IsAvailable() {
		return "", fmt.Errorf("LLM not available")
	}

	// Use context with timeout
	type result struct {
		output string
		err    error
	}

	done := make(chan result, 1)
	go func() {
		output, err := l.Predict(prompt)
		done <- result{output, err}
	}()

	select {
	case res := <-done:
		return res.output, res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("LLM prediction timed out after %v", timeout)
	}
}

// Close cleans up any resources (no-op for subprocess-based client)
func (l *LLMClient) Close() error {
	return nil
}
