// Package training provides data preparation and formatting for LLM fine-tuning
package training

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TrainingExample represents a single training example in Alpaca format
type TrainingExample struct {
	Instruction string `json:"instruction"`
	Input       string `json:"input,omitempty"`
	Output      string `json:"output"`
	System      string `json:"system,omitempty"`
}

// ChatMLMessage represents a message in ChatML format
type ChatMLMessage struct {
	Role    string `json:"role"` // system, user, assistant
	Content string `json:"content"`
}

// ChatMLExample represents a full conversation for training
type ChatMLExample struct {
	Messages []ChatMLMessage `json:"messages"`
}

// IntentTrainingData is specialized for intent classification training
type IntentTrainingData struct {
	Message    string  `json:"message"`
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	Category   string  `json:"category"` // wallet, payment, swap, system, etc.
}

// Dataset represents a collection of training examples
type Dataset struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	Description string            `json:"description"`
	Examples    []TrainingExample `json:"examples"`
	Metadata    DatasetMetadata   `json:"metadata"`
}

// DatasetMetadata contains statistics and info about the dataset
type DatasetMetadata struct {
	TotalExamples   int            `json:"total_examples"`
	IntentCounts    map[string]int `json:"intent_counts"`
	AvgInputLength  float64        `json:"avg_input_length"`
	AvgOutputLength float64        `json:"avg_output_length"`
	Categories      map[string]int `json:"categories"`
}

// DatasetBuilder helps construct training datasets
type DatasetBuilder struct {
	examples []TrainingExample
	name     string
	version  string
	desc     string
}

// NewDatasetBuilder creates a new dataset builder
func NewDatasetBuilder(name, version, description string) *DatasetBuilder {
	return &DatasetBuilder{
		examples: make([]TrainingExample, 0),
		name:     name,
		version:  version,
		desc:     description,
	}
}

// AddExample adds a training example to the dataset
func (b *DatasetBuilder) AddExample(instruction, input, output, system string) {
	b.examples = append(b.examples, TrainingExample{
		Instruction: instruction,
		Input:       input,
		Output:      output,
		System:      system,
	})
}

// AddIntentExample adds an example for intent classification
func (b *DatasetBuilder) AddIntentExample(message, intent string, confidence float64, system string) {
	instruction := "Classify the user intent for this Stellar Go CLI command. Respond with ONLY a JSON object: {\"intent\": \"NAME\", \"confidence\": 0.0-1.0}"
	output := fmt.Sprintf(`{"intent": "%s", "confidence": %.2f}`, intent, confidence)
	b.AddExample(instruction, message, output, system)
}

// AddParameterExample adds an example for parameter extraction
func (b *DatasetBuilder) AddParameterExample(intent, message string, params map[string]interface{}, system string) {
	instruction := fmt.Sprintf("Extract parameters from the message for intent '%s'. Respond with JSON.", intent)
	outputBytes, err := json.Marshal(params)
	if err != nil {
		return
	}
	b.AddExample(instruction, message, string(outputBytes), system)
}

// AddResponseExample adds an example for response generation
func (b *DatasetBuilder) AddResponseExample(intent string, params map[string]interface{}, result, expectedResponse, system string) {
	instruction := "Generate a helpful, conversational response based on the operation result."
	inputData := map[string]interface{}{
		"intent": intent,
		"params": params,
		"result": result,
	}
	inputBytes, err := json.Marshal(inputData)
	if err != nil {
		return
	}
	b.AddExample(instruction, string(inputBytes), expectedResponse, system)
}

// Build creates the final Dataset with metadata
func (b *DatasetBuilder) Build() *Dataset {
	ds := &Dataset{
		Name:        b.name,
		Version:     b.version,
		CreatedAt:   time.Now(),
		Description: b.desc,
		Examples:    b.examples,
		Metadata: DatasetMetadata{
			TotalExamples: len(b.examples),
			IntentCounts:  make(map[string]int),
			Categories:    make(map[string]int),
		},
	}

	// Calculate statistics
	var totalInputLen, totalOutputLen int
	for _, ex := range b.examples {
		totalInputLen += len(ex.Input)
		totalOutputLen += len(ex.Output)
	}

	if len(b.examples) > 0 {
		ds.Metadata.AvgInputLength = float64(totalInputLen) / float64(len(b.examples))
		ds.Metadata.AvgOutputLength = float64(totalOutputLen) / float64(len(b.examples))
	}

	return ds
}

// SaveToFile saves the dataset as JSONL
func (ds *Dataset) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create dataset file: %w", err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // best-effort close

	encoder := json.NewEncoder(file)
	for _, ex := range ds.Examples {
		if err := encoder.Encode(ex); err != nil {
			return fmt.Errorf("encode example: %w", err)
		}
	}

	return nil
}

// SaveMetadata saves dataset metadata separately
func (ds *Dataset) SaveMetadata(path string) error {
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadDataset loads a dataset from a JSONL file
func LoadDataset(path string) (*Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dataset file: %w", err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // best-effort close

	ds := &Dataset{
		Examples: make([]TrainingExample, 0),
	}

	decoder := json.NewDecoder(file)
	for decoder.More() {
		var ex TrainingExample
		if err := decoder.Decode(&ex); err != nil {
			return nil, fmt.Errorf("decode example: %w", err)
		}
		ds.Examples = append(ds.Examples, ex)
	}

	ds.Metadata.TotalExamples = len(ds.Examples)
	return ds, nil
}

// ToChatMLFormat converts Alpaca format to ChatML format
func (ex *TrainingExample) ToChatMLFormat() []ChatMLMessage {
	messages := make([]ChatMLMessage, 0)

	if ex.System != "" {
		messages = append(messages, ChatMLMessage{
			Role:    "system",
			Content: ex.System,
		})
	}

	content := ex.Instruction
	if ex.Input != "" {
		content += "\n\nInput: " + ex.Input
	}

	messages = append(messages,
		ChatMLMessage{Role: "user", Content: content},
		ChatMLMessage{Role: "assistant", Content: ex.Output},
	)

	return messages
}

// SplitDataset splits a dataset into train/validation/test sets
func (ds *Dataset) SplitDataset(trainRatio, valRatio float64) (*Dataset, *Dataset, *Dataset) {
	if trainRatio+valRatio > 1.0 {
		valRatio = 1.0 - trainRatio
	}

	total := len(ds.Examples)
	trainEnd := int(float64(total) * trainRatio)
	valEnd := trainEnd + int(float64(total)*valRatio)

	train := &Dataset{
		Name:        ds.Name + "_train",
		Version:     ds.Version,
		CreatedAt:   ds.CreatedAt,
		Description: ds.Description + " (training split)",
		Examples:    ds.Examples[:trainEnd],
		Metadata:    DatasetMetadata{TotalExamples: trainEnd},
	}

	val := &Dataset{
		Name:        ds.Name + "_val",
		Version:     ds.Version,
		CreatedAt:   ds.CreatedAt,
		Description: ds.Description + " (validation split)",
		Examples:    ds.Examples[trainEnd:valEnd],
		Metadata:    DatasetMetadata{TotalExamples: valEnd - trainEnd},
	}

	test := &Dataset{
		Name:        ds.Name + "_test",
		Version:     ds.Version,
		CreatedAt:   ds.CreatedAt,
		Description: ds.Description + " (test split)",
		Examples:    ds.Examples[valEnd:],
		Metadata:    DatasetMetadata{TotalExamples: total - valEnd},
	}

	return train, val, test
}
