// Package gguf provides GGUF model export and quantization for llama.cpp integration
package gguf

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// QuantizationType represents different GGUF quantization levels
type QuantizationType string

const (
	// No quantization (FP16)
	QuantFP16 QuantizationType = "f16"
	// Q4_0 - Fastest, lowest quality
	QuantQ4_0 QuantizationType = "q4_0"
	// Q4_1 - Better quality than Q4_0
	QuantQ4_1 QuantizationType = "q4_1"
	// Q4_K - Medium quality
	QuantQ4_K QuantizationType = "q4_K"
	// Q4_K_S - Small, fast
	QuantQ4_K_S QuantizationType = "q4_K_S"
	// Q4_K_M - Medium, balanced (recommended)
	QuantQ4_K_M QuantizationType = "q4_K_M"
	// Q5_0 - Better quality than Q4
	QuantQ5_0 QuantizationType = "q5_0"
	// Q5_1 - Best Q5 quality
	QuantQ5_1 QuantizationType = "q5_1"
	// Q5_K - High quality
	QuantQ5_K QuantizationType = "q5_K"
	// Q5_K_S - Small Q5
	QuantQ5_K_S QuantizationType = "q5_K_S"
	// Q5_K_M - Medium Q5 (recommended)
	QuantQ5_K_M QuantizationType = "q5_K_M"
	// Q6_K - Very high quality
	QuantQ6_K QuantizationType = "q6_K"
	// Q8_0 - Near-FP32 quality
	QuantQ8_0 QuantizationType = "q8_0"
)

// QuantizationInfo describes a quantization type
type QuantizationInfo struct {
	Type        QuantizationType
	Description string
	SizeRatio   float64 // Relative to FP16
	Quality     string  // low, medium, high, very-high
}

// QuantizationTypes returns all available quantization types
func QuantizationTypes() []QuantizationInfo {
	return []QuantizationInfo{
		{QuantFP16, "FP16 (no quantization)", 1.0, "very-high"},
		{QuantQ4_0, "Q4_0 - Legacy, fast, low quality", 0.25, "low"},
		{QuantQ4_1, "Q4_1 - Legacy, medium quality", 0.28, "medium"},
		{QuantQ4_K, "Q4_K - Modern K-quants", 0.25, "medium"},
		{QuantQ4_K_S, "Q4_K_S - Small, fast", 0.22, "medium"},
		{QuantQ4_K_M, "Q4_K_M - Balanced (recommended)", 0.28, "medium-high"},
		{QuantQ5_0, "Q5_0 - Legacy Q5", 0.31, "high"},
		{QuantQ5_1, "Q5_1 - Best legacy Q5", 0.33, "high"},
		{QuantQ5_K, "Q5_K - Modern K-quants", 0.31, "high"},
		{QuantQ5_K_S, "Q5_K_S - Small Q5", 0.28, "high"},
		{QuantQ5_K_M, "Q5_K_M - Recommended quality", 0.34, "very-high"},
		{QuantQ6_K, "Q6_K - Very high quality", 0.40, "very-high"},
		{QuantQ8_0, "Q8_0 - Near-FP32", 0.50, "very-high"},
	}
}

// RecommendedQuantizations returns the recommended quantization types
func RecommendedQuantizations() []QuantizationType {
	return []QuantizationType{
		QuantQ4_K_M, // Best balance
		QuantQ5_K_M, // Higher quality
		QuantQ8_0,   // Maximum quality
	}
}

// Exporter handles GGUF model export and quantization
type Exporter struct {
	llamaCppPath   string
	workDir        string
	convertScript  string
	quantizeBinary string
}

// NewExporter creates a new GGUF exporter
// It looks for tools in llamaCppPath and llamaCppPath/build/bin
func NewExporter(llamaCppPath, workDir string) *Exporter {
	// Look for tools in standard locations
	convertScript := filepath.Join(llamaCppPath, "convert_hf_to_gguf.py")
	quantizeBinary := filepath.Join(llamaCppPath, "llama-quantize")

	// If quantize not found, try build/bin subdirectory
	if _, err := os.Stat(quantizeBinary); os.IsNotExist(err) {
		quantizeBinary = filepath.Join(llamaCppPath, "build", "bin", "llama-quantize")
	}

	return &Exporter{
		llamaCppPath:   llamaCppPath,
		workDir:        workDir,
		convertScript:  convertScript,
		quantizeBinary: quantizeBinary,
	}
}

// ExportOptions contains options for GGUF export
type ExportOptions struct {
	BaseModelPath    string           // Path to base model (FP16)
	AdapterPath      string           // Path to LoRA adapter
	OutputPath       string           // Output GGUF file path
	Quantization     QuantizationType // Quantization level
	ContextSize      int              // Context size for model metadata
	OllamaCompatible bool             // Generate Ollama Modelfile
}

// MergeAndExport merges a LoRA adapter with base model and exports to GGUF
func (e *Exporter) MergeAndExport(opts ExportOptions) error {
	// Step 1: Merge LoRA adapter with base model
	mergedPath, err := e.mergeAdapter(opts.BaseModelPath, opts.AdapterPath)
	if err != nil {
		return fmt.Errorf("merge adapter: %w", err)
	}

	// Step 2: Convert to GGUF format
	ggufPath, err := e.convertToGGUF(mergedPath, opts.OutputPath)
	if err != nil {
		return fmt.Errorf("convert to GGUF: %w", err)
	}

	// Step 3: Quantize if requested
	if opts.Quantization != QuantFP16 {
		quantizedPath, err := e.quantize(ggufPath, opts.OutputPath, opts.Quantization)
		if err != nil {
			return fmt.Errorf("quantize model: %w", err)
		}
		ggufPath = quantizedPath
	}

	// Step 4: Generate Ollama Modelfile if requested
	if opts.OllamaCompatible {
		if err := e.generateModelfile(ggufPath, opts); err != nil {
			return fmt.Errorf("generate Modelfile: %w", err)
		}
	}

	return nil
}

// mergeAdapter merges a LoRA adapter into the base model
func (e *Exporter) mergeAdapter(basePath, adapterPath string) (string, error) {
	mergedDir := filepath.Join(e.workDir, "merged")
	if err := os.MkdirAll(mergedDir, 0755); err != nil {
		return "", fmt.Errorf("create merged directory: %w", err)
	}

	// Use Python script to merge adapter with peft
	script := fmt.Sprintf(`
import torch
from peft import PeftModel
from transformers import AutoModelForCausalLM, AutoTokenizer

base_model = AutoModelForCausalLM.from_pretrained("%s", torch_dtype=torch.float16)
tokenizer = AutoTokenizer.from_pretrained("%s")

model = PeftModel.from_pretrained(base_model, "%s")
model = model.merge_and_unload()

model.save_pretrained("%s")
tokenizer.save_pretrained("%s")
`, basePath, basePath, adapterPath, mergedDir, mergedDir)

	scriptPath := filepath.Join(e.workDir, "merge_adapter.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("write merge script: %w", err)
	}

	cmd := exec.Command("python3", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run merge script: %w\nOutput: %s", err, output)
	}

	return mergedDir, nil
}

// convertToGGUF converts a HuggingFace model to GGUF format
func (e *Exporter) convertToGGUF(modelPath, outputPath string) (string, error) {
	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Run llama.cpp convert script
	cmd := exec.Command(
		"python3",
		e.convertScript,
		modelPath,
		"--outfile", outputPath,
		"--outtype", "f16", // Always convert to FP16 first
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("convert to GGUF: %w\nOutput: %s", err, output)
	}

	return outputPath, nil
}

// quantize applies quantization to a GGUF model
func (e *Exporter) quantize(inputPath, outputPath string, quantType QuantizationType) (string, error) {
	// Generate output filename with quantization suffix
	quantizedPath := e.addQuantSuffix(outputPath, quantType)

	cmd := exec.Command(
		e.quantizeBinary,
		inputPath,
		quantizedPath,
		string(quantType),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("quantize model: %w\nOutput: %s", err, output)
	}

	return quantizedPath, nil
}

// generateModelfile creates an Ollama Modelfile
func (e *Exporter) generateModelfile(ggufPath string, opts ExportOptions) error {
	modelfilePath := filepath.Join(filepath.Dir(ggufPath), "Modelfile")

	systemPrompt := `You are Stellar Go CLI, a cryptocurrency payment CLI assistant. 
You help users with wallet management, payments, swaps, and other blockchain operations.
Always respond concisely and accurately. For payment intents, extract parameters precisely.`

	modelfile := fmt.Sprintf(`FROM %s

SYSTEM """%s"""

PARAMETER temperature 0.1
PARAMETER top_p 0.9
PARAMETER top_k 40
PARAMETER repeat_penalty 1.1

# Context size
PARAMETER num_ctx %d

# Stop sequences
STOP "<|im_end|>"
STOP "<|endoftext|>"
`, filepath.Base(ggufPath), systemPrompt, opts.ContextSize)

	return os.WriteFile(modelfilePath, []byte(modelfile), 0644)
}

// addQuantSuffix adds quantization type to filename
func (e *Exporter) addQuantSuffix(path string, quantType QuantizationType) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s-%s%s", base, quantType, ext)
}

// VerifyTools checks if required llama.cpp tools are available
func (e *Exporter) VerifyTools() error {
	tools := []struct {
		name string
		path string
	}{
		{"convert_hf_to_gguf.py", e.convertScript},
		{"llama-quantize", e.quantizeBinary},
	}

	for _, tool := range tools {
		if _, err := os.Stat(tool.path); os.IsNotExist(err) {
			return fmt.Errorf("required tool not found: %s at %s", tool.name, tool.path)
		}
	}

	return nil
}

// GetModelSize estimates the size of a quantized model
func GetModelSize(baseSizeBytes int64, quantType QuantizationType) int64 {
	for _, info := range QuantizationTypes() {
		if info.Type == quantType {
			return int64(float64(baseSizeBytes) * info.SizeRatio)
		}
	}
	return baseSizeBytes
}

// ExportConfig provides a structured way to configure exports
type ExportConfig struct {
	JobID            string
	BaseModel        string
	AdapterPath      string
	OutputDir        string
	Quantizations    []QuantizationType
	ContextSize      int
	OllamaCompatible bool
	KeepIntermediate bool
}

// BatchExport exports multiple quantization variants
func (e *Exporter) BatchExport(config ExportConfig) (map[QuantizationType]string, error) {
	results := make(map[QuantizationType]string)

	for _, quantType := range config.Quantizations {
		opts := ExportOptions{
			BaseModelPath:    config.BaseModel,
			AdapterPath:      config.AdapterPath,
			OutputPath:       filepath.Join(config.OutputDir, fmt.Sprintf("%s.gguf", config.JobID)),
			Quantization:     quantType,
			ContextSize:      config.ContextSize,
			OllamaCompatible: config.OllamaCompatible && quantType == config.Quantizations[0],
		}

		if err := e.MergeAndExport(opts); err != nil {
			return results, fmt.Errorf("export %s: %w", quantType, err)
		}

		results[quantType] = opts.OutputPath
	}

	return results, nil
}
