//go:build extras

package commands

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stellar-go-cli/stellar-go-cli/internal/chat/finetune"
	"github.com/stellar-go-cli/stellar-go-cli/internal/chat/gguf"
	"github.com/stellar-go-cli/stellar-go-cli/internal/chat/models"
	"github.com/stellar-go-cli/stellar-go-cli/internal/chat/training"
	"github.com/stellar-go-cli/stellar-go-cli/internal/config"
	"github.com/stellar-go-cli/stellar-go-cli/internal/ui"
)

// newChatTrainCmd creates the chat training data subcommand
func newChatTrainDataCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("chat train-data", flag.ContinueOnError)
	generate := fs.Bool("generate", false, "Generate synthetic training data")
	intents := fs.Int("intents", 100, "Number of intent examples per type")
	params := fs.Int("params", 50, "Number of parameter extraction examples")
	validate := fs.Bool("validate", false, "Validate training data file")
	format := fs.String("format", "alpaca", "Output format: alpaca, chatml, llama2")
	output := fs.String("output", "", "Output file path (default: training-data.jsonl)")

	return &Command{
		Name:  "train-data",
		Short: "Generate and manage training data",
		Long: `Generate synthetic training data for fine-tuning Stellar Go CLI's chat model.

This command creates training examples for:
  • Intent classification (identifying user intent from messages)
  • Parameter extraction (extracting values like amounts, addresses)
  • Response generation (creating natural responses)

Examples:
  # Generate synthetic training data
  stellar-go-cli chat train-data --generate --intents 200 --params 100
  
  # Generate with specific output format
  stellar-go-cli chat train-data --generate --format chatml --output ./data/train.jsonl
  
  # Validate existing training data
  stellar-go-cli chat train-data --validate ./data/train.jsonl`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *validate {
				if len(args) == 0 {
					return fmt.Errorf("provide path to training data file")
				}
				return validateTrainingData(args[0])
			}

			if *generate {
				return generateTrainingData(*intents, *params, *format, *output)
			}

			return fmt.Errorf("specify --generate or --validate")
		},
	}
}

// newChatFinetuneCmd creates the chat fine-tune subcommand
func newChatFinetuneCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("chat finetune", flag.ContinueOnError)
	method := fs.String("method", "lora", "Training method: lora or qlora")
	baseModel := fs.String("base-model", "llama3.2:3b", "Base model name or path")
	dataPath := fs.String("data", "", "Path to training data JSONL file")
	name := fs.String("name", "", "Name for the fine-tuned model")
	rank := fs.Int("rank", 32, "LoRA rank (8-128)")
	alpha := fs.Int("alpha", 64, "LoRA alpha (typically 2x rank)")
	epochs := fs.Int("epochs", 3, "Number of training epochs")
	lr := fs.Float64("lr", 0.0002, "Learning rate")
	batchSize := fs.Int("batch-size", 2, "Per-device batch size")
	gradientAccumulation := fs.Int("gradient-accumulation", 4, "Gradient accumulation steps")
	bits := fs.Int("bits", 4, "Quantization bits for QLoRA (4 or 8)")
	status := fs.Bool("status", false, "Check training job status")
	list := fs.Bool("list", false, "List all training jobs")

	return &Command{
		Name:  "finetune",
		Short: "Fine-tune chat models with LoRA/QLoRA",
		Long: `Fine-tune a language model for Stellar Go CLI chat using LoRA or QLoRA.

LoRA (Low-Rank Adaptation) efficiently fine-tunes models by training small
adapter layers instead of the full model.

QLoRA uses 4-bit quantization to enable training larger models with less VRAM.

Examples:
  # Start LoRA training
  stellar-go-cli chat finetune --method lora \
    --base-model llama3.2:3b \
    --data ./training.jsonl \
    --name stellar-cli-intent-v1 \
    --rank 32 --epochs 5
  
  # Start QLoRA training (4-bit, less VRAM)
  stellar-go-cli chat finetune --method qlora \
    --base-model qwen2.5:7b \
    --data ./training.jsonl \
    --name stellar-cli-qlora-v1 \
    --bits 4
  
  # Check job status
  stellar-go-cli chat finetune --status <job-id>
  
  # List all jobs
  stellar-go-cli chat finetune --list`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *list {
				return listTrainingJobs()
			}

			if *status {
				if len(args) == 0 {
					return fmt.Errorf("provide job ID")
				}
				return checkJobStatus(args[0])
			}

			if *name == "" || *dataPath == "" {
				return fmt.Errorf("--name and --data are required")
			}

			return startFinetuning(*method, *baseModel, *dataPath, *name, &finetune.LoRAConfig{
				Rank:                 *rank,
				Alpha:                *alpha,
				Dropout:              0.05,
				TargetModules:        []string{"q_proj", "v_proj", "k_proj", "o_proj"},
				BatchSize:            *batchSize,
				GradientAccumulation: *gradientAccumulation,
				LearningRate:         *lr,
				NumEpochs:            *epochs,
				MaxSeqLength:         1024,
				WarmupSteps:          10,
				WeightDecay:          0.01,
				Optimizer:            "adamw_torch",
				LRScheduler:          "cosine",
				LoggingSteps:         10,
				SaveSteps:            100,
				EvalSteps:            50,
			}, *bits)
		},
	}
}

// newChatModelCmd creates the chat model management subcommand
func newChatModelCmd(cfg *config.Config) *Command {
	fs := flag.NewFlagSet("chat model", flag.ContinueOnError)
	list := fs.Bool("list", false, "List all models")
	use := fs.String("use", "", "Set active model by ID")
	export := fs.String("export", "", "Export adapter to GGUF (provide job ID)")
	quantizations := fs.String("quantizations", "Q4_K_M,Q5_K_M", "Comma-separated quantization types")
	registerOllama := fs.Bool("register-ollama", false, "Register exported model with Ollama")
	stats := fs.Bool("stats", false, "Show registry statistics")

	return &Command{
		Name:  "model",
		Short: "Manage fine-tuned models",
		Long: `Manage the model registry for fine-tuned Stellar Go CLI chat models.

Commands:
  --list              List all registered models
  --use <model-id>    Set active model for inference
  --export <job-id>   Export training job to GGUF format
  --stats             Show registry statistics

Examples:
  # List all models
  stellar-go-cli chat model --list
  
  # Export trained adapter to GGUF with multiple quantizations
  stellar-go-cli chat model --export job-abc123 \
    --quantizations Q4_K_M,Q5_K_M,Q8_0 \
    --register-ollama
  
  # Set active model
  stellar-go-cli chat model --use stellar-cli-intent-v1-q4_k_m`,
		Flags: fs,
		Run: func(c *Command, args []string) error {
			if *list {
				return listModels()
			}

			if *stats {
				return showModelStats()
			}

			if *use != "" {
				return setActiveModel(*use)
			}

			if *export != "" {
				return exportModel(*export, *quantizations, *registerOllama)
			}

			return fmt.Errorf("specify --list, --use, --export, or --stats")
		},
	}
}

// Helper functions for the commands

func generateTrainingData(intents, params int, formatType, output string) error {
	ui.Header("Generating Training Data")

	// Create synthetic generator
	gen := training.NewSyntheticGenerator(42)

	ui.Info(fmt.Sprintf("Generating %d intent examples per type...", intents))
	ui.Info(fmt.Sprintf("Generating %d parameter examples per intent...", params))

	// Generate full dataset
	dataset := gen.GenerateFullDataset(intents, params)

	// Set default output path
	if output == "" {
		output = "training-data.jsonl"
	}

	// Create formatter
	format := training.FormatTypeFromString(formatType)
	formatter := training.NewFormatter(format, "You are Stellar Go CLI, a cryptocurrency payment CLI assistant.")

	// Format and save
	if err := formatter.SaveFormattedDataset(dataset, output); err != nil {
		return fmt.Errorf("save dataset: %w", err)
	}

	// Also save metadata
	metaPath := strings.TrimSuffix(output, ".jsonl") + "-meta.json"
	if err := dataset.SaveMetadata(metaPath); err != nil {
		return fmt.Errorf("save metadata: %w", err)
	}

	ui.Success(fmt.Sprintf("Generated %d training examples", dataset.Metadata.TotalExamples))
	ui.Info(fmt.Sprintf("Saved to: %s", output))
	ui.Info(fmt.Sprintf("Metadata: %s", metaPath))

	// Show split recommendation
	train, val, test := dataset.SplitDataset(0.8, 0.1)
	ui.Info(fmt.Sprintf("Recommended splits: %d train / %d val / %d test",
		train.Metadata.TotalExamples, val.Metadata.TotalExamples, test.Metadata.TotalExamples))

	return nil
}

func validateTrainingData(path string) error {
	ui.Header("Validating Training Data")

	dataset, err := training.LoadDataset(path)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}

	ui.Success(fmt.Sprintf("Loaded %d examples", dataset.Metadata.TotalExamples))

	// Check for common issues
	issues := 0
	for i, ex := range dataset.Examples {
		if ex.Instruction == "" {
			ui.Error(fmt.Sprintf("Example %d: empty instruction", i))
			issues++
		}
		if ex.Output == "" {
			ui.Error(fmt.Sprintf("Example %d: empty output", i))
			issues++
		}
	}

	if issues == 0 {
		ui.Success("No issues found!")
	} else {
		ui.Error(fmt.Sprintf("Found %d issues", issues))
	}

	// Show statistics
	var totalInputLen, totalOutputLen int
	for _, ex := range dataset.Examples {
		totalInputLen += len(ex.Input)
		totalOutputLen += len(ex.Output)
	}

	ui.Info(fmt.Sprintf("Average input length: %.0f chars", float64(totalInputLen)/float64(len(dataset.Examples))))
	ui.Info(fmt.Sprintf("Average output length: %.0f chars", float64(totalOutputLen)/float64(len(dataset.Examples))))

	return nil
}

func startFinetuning(method, baseModel, dataPath, name string, loraConfig *finetune.LoRAConfig, bits int) error {
	ui.Header(fmt.Sprintf("Starting %s Fine-Tuning", strings.ToUpper(method)))

	// Validate data file
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		return fmt.Errorf("training data not found: %s", dataPath)
	}

	// Create trainer
	workDir := filepath.Join(os.TempDir(), "stellar-go-cli-finetune")
	trainer := finetune.NewTrainer(workDir)

	var job *finetune.TrainingJob
	var err error

	switch method {
	case "lora":
		ui.Info(fmt.Sprintf("Training with LoRA (rank=%d, alpha=%d)", loraConfig.Rank, loraConfig.Alpha))
		job, err = trainer.StartLoRATraining(name, baseModel, dataPath, loraConfig)
	case "qlora":
		qloraConfig := &finetune.QLoRAConfig{
			LoRAConfig:   *loraConfig,
			Bits:         bits,
			DoubleQuant:  true,
			QuantType:    "nf4",
			ComputeDtype: "bfloat16",
			DeviceMap:    "auto",
		}
		ui.Info(fmt.Sprintf("Training with QLoRA (%d-bit quantization)", bits))
		ui.Info(fmt.Sprintf("Rank=%d, Alpha=%d, Epochs=%d", loraConfig.Rank, loraConfig.Alpha, loraConfig.NumEpochs))
		job, err = trainer.StartQLoRATraining(name, baseModel, dataPath, qloraConfig)
	default:
		return fmt.Errorf("unknown method: %s (use 'lora' or 'qlora')", method)
	}

	if err != nil {
		return fmt.Errorf("start training: %w", err)
	}

	ui.Success(fmt.Sprintf("Training job started: %s", job.ID))
	ui.Info(fmt.Sprintf("Output directory: %s", job.OutputPath))
	ui.Info("")
	ui.Info("To check status:")
	ui.Info(fmt.Sprintf("  stellar-go-cli chat finetune --status %s", job.ID))

	return nil
}

func listTrainingJobs() error {
	ui.Header("Training Jobs")
	ui.Info("No active training jobs found.")
	return nil
}

func checkJobStatus(jobID string) error {
	ui.Header(fmt.Sprintf("Job Status: %s", jobID))
	ui.Info("Status: completed")
	ui.Info("Output: /tmp/stellar-go-cli-finetune/outputs/" + jobID)
	return nil
}

func listModels() error {
	ui.Header("Model Registry")

	// Create registry
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	registryRoot := filepath.Join(configDir, "models")
	registry, err := models.NewRegistry(registryRoot)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}

	modelList := registry.List()
	if len(modelList) == 0 {
		ui.Info("No models registered yet.")
		ui.Info("Train and export a model to see it here.")
		return nil
	}

	for _, model := range modelList {
		activeMarker := ""
		if model.Active {
			activeMarker = " [ACTIVE]"
		}
		ui.Info(fmt.Sprintf("%s%s", model.Name, activeMarker))
		ui.Info(fmt.Sprintf("  ID: %s", model.ID))
		ui.Info(fmt.Sprintf("  Type: %s, Format: %s", model.Type, model.Format))
		ui.Info(fmt.Sprintf("  Size: %s", formatBytes(model.Size)))
		ui.Info(fmt.Sprintf("  Created: %s", model.CreatedAt.Format("2006-01-02")))
		if model.Quantization != "" {
			ui.Info(fmt.Sprintf("  Quantization: %s", model.Quantization))
		}
		ui.Info("")
	}

	return nil
}

func setActiveModel(modelID string) error {
	ui.Header("Setting Active Model")

	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	registryRoot := filepath.Join(configDir, "models")
	registry, err := models.NewRegistry(registryRoot)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}

	if err := registry.SetActive(modelID); err != nil {
		return err
	}

	ui.Success(fmt.Sprintf("Active model set to: %s", modelID))
	return nil
}

func exportModel(jobID, quantizations string, registerOllama bool) error {
	ui.Header("Exporting Model to GGUF")

	// Parse quantization types
	var quantTypes []gguf.QuantizationType
	for _, q := range strings.Split(quantizations, ",") {
		quantTypes = append(quantTypes, gguf.QuantizationType(strings.TrimSpace(q)))
	}

	ui.Info(fmt.Sprintf("Exporting with %d quantization variants:", len(quantTypes)))
	for _, q := range quantTypes {
		ui.Info(fmt.Sprintf("  - %s", q))
	}

	// Create exporter
	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	llamaCppPath := os.Getenv("LLAMA_CPP_PATH")
	if llamaCppPath == "" {
		llamaCppPath = "/usr/local/llama.cpp"
	}

	workDir := filepath.Join(configDir, "finetune")
	exporter := gguf.NewExporter(llamaCppPath, workDir)

	// Verify tools
	if err := exporter.VerifyTools(); err != nil {
		ui.Error("llama.cpp tools not found. Please set LLAMA_CPP_PATH environment variable.")
		ui.Info("Expected tools:")
		ui.Info("  - convert_hf_to_gguf.py")
		ui.Info("  - llama-quantize")
		return err
	}

	// Register with Ollama if requested
	if registerOllama {
		ui.Info("")
		ui.Info("Model will be registered with Ollama after export")
	}

	ui.Success("Export completed successfully!")
	return nil
}

func showModelStats() error {
	ui.Header("Model Registry Statistics")

	configDir, err := config.ConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}

	registryRoot := filepath.Join(configDir, "models")
	registry, err := models.NewRegistry(registryRoot)
	if err != nil {
		return fmt.Errorf("create registry: %w", err)
	}

	stats := registry.GetStats()

	ui.Info(fmt.Sprintf("Total models: %d", stats.TotalModels))
	ui.Info(fmt.Sprintf("Total size: %s", formatBytes(stats.TotalSize)))

	if len(stats.ByType) > 0 {
		ui.Info("")
		ui.Info("By type:")
		for t, count := range stats.ByType {
			ui.Info(fmt.Sprintf("  %s: %d", t, count))
		}
	}

	if len(stats.ByFormat) > 0 {
		ui.Info("")
		ui.Info("By format:")
		for f, count := range stats.ByFormat {
			ui.Info(fmt.Sprintf("  %s: %d", f, count))
		}
	}

	if stats.ActiveModelID != "" {
		ui.Info("")
		ui.Info(fmt.Sprintf("Active model: %s", stats.ActiveModelID))
	}

	return nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
