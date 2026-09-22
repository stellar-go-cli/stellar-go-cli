// Package training provides format conversion for different LLM training formats
package training

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FormatType represents the output format for training data
type FormatType string

const (
	FormatAlpaca   FormatType = "alpaca"
	FormatChatML   FormatType = "chatml"
	FormatLlama2   FormatType = "llama2"
	FormatOpenAI   FormatType = "openai"
	FormatShareGPT FormatType = "sharegpt"
)

// Formatter converts training examples to various formats
type Formatter struct {
	format    FormatType
	systemMsg string
}

// NewFormatter creates a new formatter with the specified format
func NewFormatter(format FormatType, systemMsg string) *Formatter {
	return &Formatter{
		format:    format,
		systemMsg: systemMsg,
	}
}

// Format converts a training example to the target format
func (f *Formatter) Format(ex TrainingExample) (string, error) {
	switch f.format {
	case FormatAlpaca:
		return f.toAlpaca(ex)
	case FormatChatML:
		return f.toChatML(ex)
	case FormatLlama2:
		return f.toLlama2(ex)
	case FormatOpenAI:
		return f.toOpenAI(ex)
	case FormatShareGPT:
		return f.toShareGPT(ex)
	default:
		return f.toAlpaca(ex)
	}
}

// toAlpaca formats as Alpaca instruction format
func (f *Formatter) toAlpaca(ex TrainingExample) (string, error) {
	output := map[string]interface{}{
		"instruction": ex.Instruction,
		"input":       ex.Input,
		"output":      ex.Output,
	}
	if ex.System != "" {
		output["system"] = ex.System
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal alpaca format: %w", err)
	}
	return string(data), nil
}

// toChatML formats as ChatML format (used by many modern models)
func (f *Formatter) toChatML(ex TrainingExample) (string, error) {
	messages := make([]map[string]string, 0)

	system := ex.System
	if system == "" {
		system = f.systemMsg
	}
	if system != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": system,
		})
	}

	content := ex.Instruction
	if ex.Input != "" {
		content += "\n\nInput: " + ex.Input
	}

	messages = append(messages,
		map[string]string{"role": "user", "content": content},
		map[string]string{"role": "assistant", "content": ex.Output},
	)

	output := map[string]interface{}{
		"messages": messages,
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal chatml format: %w", err)
	}
	return string(data), nil
}

// toLlama2 formats as LLaMA-2 chat format
func (f *Formatter) toLlama2(ex TrainingExample) (string, error) {
	var b strings.Builder

	// System prompt
	system := ex.System
	if system == "" {
		system = f.systemMsg
	}
	if system != "" {
		fmt.Fprintf(&b, "<<SYS>>\n%s\n<</SYS>>\n\n", system)
	}

	// User message
	content := ex.Instruction
	if ex.Input != "" {
		content += "\n\n" + ex.Input
	}
	fmt.Fprintf(&b, "[INST] %s [/INST]", content)

	// Assistant response
	fmt.Fprintf(&b, " %s", ex.Output)

	output := map[string]string{
		"text": b.String(),
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal llama2 format: %w", err)
	}
	return string(data), nil
}

// toOpenAI formats as OpenAI chat completions format
func (f *Formatter) toOpenAI(ex TrainingExample) (string, error) {
	messages := make([]map[string]string, 0)

	system := ex.System
	if system == "" {
		system = f.systemMsg
	}
	if system != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": system,
		})
	}

	content := ex.Instruction
	if ex.Input != "" {
		content += "\n\n" + ex.Input
	}

	messages = append(messages,
		map[string]string{"role": "user", "content": content},
		map[string]string{"role": "assistant", "content": ex.Output},
	)

	output := map[string]interface{}{
		"messages": messages,
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal openai format: %w", err)
	}
	return string(data), nil
}

// toShareGPT formats as ShareGPT format
func (f *Formatter) toShareGPT(ex TrainingExample) (string, error) {
	conversations := make([]map[string]string, 0)

	system := ex.System
	if system == "" {
		system = f.systemMsg
	}
	if system != "" {
		conversations = append(conversations, map[string]string{
			"from":  "system",
			"value": system,
		})
	}

	content := ex.Instruction
	if ex.Input != "" {
		content += "\n\n" + ex.Input
	}

	conversations = append(conversations,
		map[string]string{"from": "human", "value": content},
		map[string]string{"from": "gpt", "value": ex.Output},
	)

	output := map[string]interface{}{
		"conversations": conversations,
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("marshal sharegpt format: %w", err)
	}
	return string(data), nil
}

// FormatDataset converts an entire dataset to the target format
func (f *Formatter) FormatDataset(ds *Dataset) ([]string, error) {
	formatted := make([]string, 0, len(ds.Examples))

	for _, ex := range ds.Examples {
		formattedEx, err := f.Format(ex)
		if err != nil {
			return nil, fmt.Errorf("format example: %w", err)
		}
		formatted = append(formatted, formattedEx)
	}

	return formatted, nil
}

// SaveFormattedDataset saves the formatted dataset to a file
func (f *Formatter) SaveFormattedDataset(ds *Dataset, path string) error {
	formatted, err := f.FormatDataset(ds)
	if err != nil {
		return fmt.Errorf("format dataset: %w", err)
	}

	file, err := createFile(path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // best-effort close

	for _, line := range formatted {
		if _, err := file.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("write formatted line: %w", err)
		}
	}

	return nil
}

// createFile is a helper to create a file
func createFile(path string) (*fileWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	return &fileWriter{f}, nil
}

// fileWriter wraps a file for writing
type fileWriter struct {
	f *os.File
}

// WriteString writes a string to the file
func (fw *fileWriter) WriteString(s string) (int, error) {
	return fw.f.WriteString(s)
}

// Close closes the file
func (fw *fileWriter) Close() error {
	return fw.f.Close()
}

// FormatTypeFromString converts a string to FormatType
func FormatTypeFromString(s string) FormatType {
	switch strings.ToLower(s) {
	case "chatml", "chat-ml":
		return FormatChatML
	case "llama2", "llama-2":
		return FormatLlama2
	case "openai":
		return FormatOpenAI
	case "sharegpt", "share-gpt":
		return FormatShareGPT
	case "alpaca":
		return FormatAlpaca
	default:
		return FormatAlpaca
	}
}

// SupportedFormats returns a list of supported format types
func SupportedFormats() []FormatType {
	return []FormatType{
		FormatAlpaca,
		FormatChatML,
		FormatLlama2,
		FormatOpenAI,
		FormatShareGPT,
	}
}

// FormatDescription returns a description of the format
func FormatDescription(format FormatType) string {
	descriptions := map[FormatType]string{
		FormatAlpaca:   "Stanford Alpaca format with instruction, input, output fields",
		FormatChatML:   "ChatML format with role-based messages (system, user, assistant)",
		FormatLlama2:   "Meta LLaMA-2 chat format with <<SYS>> and [INST] tags",
		FormatOpenAI:   "OpenAI chat completions API format",
		FormatShareGPT: "ShareGPT conversation format",
	}
	if desc, ok := descriptions[format]; ok {
		return desc
	}
	return "Unknown format"
}
