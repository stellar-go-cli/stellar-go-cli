// Package finetune provides LoRA/QLoRA fine-tuning capabilities for LLMs
package finetune

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LoRAConfig holds configuration for LoRA training
type LoRAConfig struct {
	// LoRA hyperparameters
	Rank          int      `json:"rank"`          // LoRA rank (r) - typically 8-128
	Alpha         int      `json:"alpha"`         // LoRA alpha - typically 2x rank
	Dropout       float64  `json:"dropout"`       // Dropout probability
	TargetModules []string `json:"targetModules"` // Modules to apply LoRA

	// Training hyperparameters
	BatchSize            int     `json:"batchSize"`            // Per-device batch size
	GradientAccumulation int     `json:"gradientAccumulation"` // Steps to accumulate
	LearningRate         float64 `json:"learningRate"`         // Initial learning rate
	NumEpochs            int     `json:"numEpochs"`            // Number of training epochs
	MaxSeqLength         int     `json:"maxSeqLength"`         // Maximum sequence length
	WarmupSteps          int     `json:"warmupSteps"`          // Learning rate warmup steps
	WeightDecay          float64 `json:"weightDecay"`          // Weight decay for regularization

	// Optimization
	Optimizer   string `json:"optimizer"`   // adamw_torch, adamw_bnb_8bit, etc.
	LRScheduler string `json:"lrScheduler"` // linear, cosine, constant

	// Logging and checkpointing
	LoggingSteps int    `json:"loggingSteps"`
	SaveSteps    int    `json:"saveSteps"`
	EvalSteps    int    `json:"evalSteps"`
	OutputDir    string `json:"outputDir"`
	LoggingDir   string `json:"loggingDir"`
}

// QLoRAConfig extends LoRAConfig with quantization settings
type QLoRAConfig struct {
	LoRAConfig `json:",inline"`

	// Quantization settings
	Bits         int    `json:"bits"`         // 4 or 8
	DoubleQuant  bool   `json:"doubleQuant"`  // Nested quantization
	QuantType    string `json:"quantType"`    // nf4, fp4
	ComputeDtype string `json:"computeDtype"` // bfloat16, float16
	GroupSize    int    `json:"groupSize"`    // Quantization group size

	// Memory optimization
	MaxMemory map[string]string `json:"maxMemory,omitempty"` // Per-GPU memory limits
	DeviceMap string            `json:"deviceMap"`           // auto, balanced, etc.
}

// TrainingJob represents an active fine-tuning job
type TrainingJob struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	BaseModel    string      `json:"baseModel"`
	DataPath     string      `json:"dataPath"`
	Config       interface{} `json:"config"` // LoRAConfig or QLoRAConfig
	Status       JobStatus   `json:"status"`
	Progress     float64     `json:"progress"` // 0.0-1.0
	CurrentStep  int         `json:"currentStep"`
	TotalSteps   int         `json:"totalSteps"`
	Loss         float64     `json:"loss"`
	LearningRate float64     `json:"learningRate"`
	OutputPath   string      `json:"outputPath"`
	CreatedAt    string      `json:"createdAt"`
	UpdatedAt    string      `json:"updatedAt"`
	Error        string      `json:"error,omitempty"`
}

// JobStatus represents the status of a training job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// DefaultLoRAConfig returns sensible defaults for LoRA training
func DefaultLoRAConfig() *LoRAConfig {
	return &LoRAConfig{
		Rank:                 32,
		Alpha:                64,
		Dropout:              0.05,
		TargetModules:        []string{"q_proj", "v_proj", "k_proj", "o_proj"},
		BatchSize:            2,
		GradientAccumulation: 4,
		LearningRate:         2e-4,
		NumEpochs:            3,
		MaxSeqLength:         1024,
		WarmupSteps:          10,
		WeightDecay:          0.01,
		Optimizer:            "adamw_torch",
		LRScheduler:          "cosine",
		LoggingSteps:         10,
		SaveSteps:            100,
		EvalSteps:            50,
	}
}

// DefaultQLoRAConfig returns sensible defaults for QLoRA training
func DefaultQLoRAConfig() *QLoRAConfig {
	return &QLoRAConfig{
		LoRAConfig:   *DefaultLoRAConfig(),
		Bits:         4,
		DoubleQuant:  true,
		QuantType:    "nf4",
		ComputeDtype: "bfloat16",
		GroupSize:    64,
		DeviceMap:    "auto",
	}
}

// Trainer orchestrates the fine-tuning process
type Trainer struct {
	workDir    string
	pythonPath string
	scriptsDir string
	activeJobs map[string]*TrainingJob
}

// NewTrainer creates a new fine-tuning trainer
func NewTrainer(workDir string) *Trainer {
	return &Trainer{
		workDir:    workDir,
		pythonPath: "python3", // Default, can be overridden
		scriptsDir: filepath.Join(workDir, "scripts"),
		activeJobs: make(map[string]*TrainingJob),
	}
}

// SetPythonPath sets a custom Python executable path
func (t *Trainer) SetPythonPath(path string) {
	t.pythonPath = path
}

// StartLoRATraining starts a LoRA training job
func (t *Trainer) StartLoRATraining(name, baseModel, dataPath string, config *LoRAConfig) (*TrainingJob, error) {
	jobID := generateJobID()

	// Resolve data path to absolute path
	absDataPath, err := filepath.Abs(dataPath)
	if err != nil {
		return nil, fmt.Errorf("resolve data path: %w", err)
	}

	job := &TrainingJob{
		ID:         jobID,
		Name:       name,
		BaseModel:  baseModel,
		DataPath:   absDataPath,
		Config:     config,
		Status:     JobStatusPending,
		OutputPath: filepath.Join(t.workDir, "outputs", jobID),
		CreatedAt:  getCurrentTimestamp(),
		UpdatedAt:  getCurrentTimestamp(),
	}

	t.activeJobs[jobID] = job

	// Ensure output directory exists
	if err := os.MkdirAll(job.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	// Start training synchronously
	t.runTraining(job)

	return job, nil
}

// StartQLoRATraining starts a QLoRA training job
func (t *Trainer) StartQLoRATraining(name, baseModel, dataPath string, config *QLoRAConfig) (*TrainingJob, error) {
	jobID := generateJobID()

	// Resolve data path to absolute path
	absDataPath, err := filepath.Abs(dataPath)
	if err != nil {
		return nil, fmt.Errorf("resolve data path: %w", err)
	}

	job := &TrainingJob{
		ID:         jobID,
		Name:       name,
		BaseModel:  baseModel,
		DataPath:   absDataPath,
		Config:     config,
		Status:     JobStatusPending,
		OutputPath: filepath.Join(t.workDir, "outputs", jobID),
		CreatedAt:  getCurrentTimestamp(),
		UpdatedAt:  getCurrentTimestamp(),
	}

	t.activeJobs[jobID] = job

	if err := os.MkdirAll(job.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	if err := t.saveJobConfig(job); err != nil {
		return nil, fmt.Errorf("save job config: %w", err)
	}

	// Start training synchronously
	t.runTraining(job)

	return job, nil
}

// runTraining executes the training process
func (t *Trainer) runTraining(job *TrainingJob) {
	job.Status = JobStatusRunning
	job.UpdatedAt = getCurrentTimestamp()

	// Generate training script
	script, err := t.GenerateTrainingScript(job)
	if err != nil {
		job.Status = JobStatusFailed
		job.Error = fmt.Sprintf("generate script: %v", err)
		// Write error to file for debugging
		errPath := filepath.Join(job.OutputPath, "error.log")
		os.WriteFile(errPath, []byte(job.Error), 0644)
		return
	}

	// Save script to file
	scriptPath := filepath.Join(job.OutputPath, "train.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		job.Status = JobStatusFailed
		job.Error = fmt.Sprintf("save script: %v", err)
		return
	}

	// Execute training with real-time output streaming
	logPath := filepath.Join(job.OutputPath, "training.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		job.Status = JobStatusFailed
		job.Error = fmt.Sprintf("create log file: %v", err)
		return
	}
	defer logFile.Close()

	cmd := exec.Command(t.pythonPath, scriptPath)
	cmd.Dir = job.OutputPath

	// Stream to both terminal and log file
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	cmd.Stdout = multiWriter
	cmd.Stderr = multiWriter

	fmt.Println("→ Starting Python training...")
	fmt.Println("")
	if err := cmd.Run(); err != nil {
		job.Status = JobStatusFailed
		job.Error = fmt.Sprintf("training failed: %v", err)
		return
	}

	job.Status = JobStatusCompleted
	job.Progress = 1.0
	job.UpdatedAt = getCurrentTimestamp()
}

// GetJob retrieves a training job by ID
func (t *Trainer) GetJob(jobID string) (*TrainingJob, error) {
	job, ok := t.activeJobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	return job, nil
}

// ListJobs returns all training jobs
func (t *Trainer) ListJobs() []*TrainingJob {
	jobs := make([]*TrainingJob, 0, len(t.activeJobs))
	for _, job := range t.activeJobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// CancelJob cancels a running training job
func (t *Trainer) CancelJob(jobID string) error {
	job, ok := t.activeJobs[jobID]
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}

	if job.Status != JobStatusRunning && job.Status != JobStatusPending {
		return fmt.Errorf("job cannot be cancelled in status: %s", job.Status)
	}

	job.Status = JobStatusCancelled
	job.UpdatedAt = getCurrentTimestamp()
	return nil
}

// saveJobConfig saves the job configuration to disk
func (t *Trainer) saveJobConfig(job *TrainingJob) error {
	configPath := filepath.Join(job.OutputPath, "job_config.json")
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job config: %w", err)
	}
	return os.WriteFile(configPath, data, 0644)
}

// LoadJob loads a job from its output directory
func (t *Trainer) LoadJob(jobID string) (*TrainingJob, error) {
	outputPath := filepath.Join(t.workDir, "outputs", jobID)
	configPath := filepath.Join(outputPath, "job_config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read job config: %w", err)
	}

	var job TrainingJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("unmarshal job config: %w", err)
	}

	t.activeJobs[jobID] = &job
	return &job, nil
}

// GenerateTrainingScript creates a Python training script for the job
func (t *Trainer) GenerateTrainingScript(job *TrainingJob) (string, error) {
	switch cfg := job.Config.(type) {
	case *LoRAConfig:
		return t.generateLoRAScript(job, cfg)
	case *QLoRAConfig:
		return t.generateQLoRAScript(job, cfg)
	default:
		return "", fmt.Errorf("unknown config type")
	}
}

// generateLoRAScript generates Python code for LoRA training
func (t *Trainer) generateLoRAScript(job *TrainingJob, config *LoRAConfig) (string, error) {
	// This would generate actual Python training code
	script := fmt.Sprintf(`#!/usr/bin/env python3
# Auto-generated LoRA training script for job %s

import torch
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    TrainingArguments,
    Trainer,
    DataCollatorForLanguageModeling
)
from peft import LoraConfig, get_peft_model, TaskType
from datasets import load_dataset

# Configuration
MODEL_NAME = "%s"
DATA_PATH = "%s"
OUTPUT_DIR = "%s"

LORA_R = %d
LORA_ALPHA = %d
LORA_DROPOUT = %f
TARGET_MODULES = %s

# Load model and tokenizer
model = AutoModelForCausalLM.from_pretrained(
    MODEL_NAME,
    torch_dtype=torch.float16,
    device_map="auto"
)
tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
tokenizer.pad_token = tokenizer.eos_token

# Configure LoRA
lora_config = LoraConfig(
    r=LORA_R,
    lora_alpha=LORA_ALPHA,
    target_modules=TARGET_MODULES,
    lora_dropout=LORA_DROPOUT,
    bias="none",
    task_type=TaskType.CAUSAL_LM
)

model = get_peft_model(model, lora_config)
model.print_trainable_parameters()

# Load and preprocess dataset
dataset = load_dataset("json", data_files=DATA_PATH, split="train")

def format_example(example):
    """Format chat example into training text"""
    system = example.get("system", "")
    instruction = example.get("instruction", "")
    input_text = example.get("input", "")
    output = example.get("output", "")
    
    if system:
        text = f"<|system|>\n{system}\n<|user|>\n{instruction}"
    else:
        text = f"<|user|>\n{instruction}"
    
    if input_text:
        text += f"\n{input_text}"
    
    text += f"\n<|assistant|>\n{output}"
    return text

def preprocess_function(examples):
    """Tokenize the formatted examples - handles batched data"""
    # examples is a dict of lists when batched=True
    texts = []
    for i in range(len(examples['instruction'])):
        system = examples.get('system', [''] * len(examples['instruction']))[i]
        instruction = examples['instruction'][i]
        input_text = examples.get('input', [''] * len(examples['instruction']))[i]
        output = examples['output'][i]
        
        if system:
            text = f"<|system|>\n{system}\n<|user|>\n{instruction}"
        else:
            text = f"<|user|>\n{instruction}"
        
        if input_text:
            text += f"\n{input_text}"
        
        text += f"\n<|assistant|>\n{output}"
        texts.append(text)
    
    model_inputs = tokenizer(
        texts,
        truncation=True,
        max_length=1024,
        padding="max_length",
        return_tensors=None
    )
    # Labels are same as input_ids for causal LM
    model_inputs["labels"] = model_inputs["input_ids"].copy()
    return model_inputs

# Tokenize dataset
tokenized_dataset = dataset.map(
    preprocess_function,
    batched=True,
    remove_columns=dataset.column_names,
    desc="Tokenizing dataset"
)

# Training arguments
training_args = TrainingArguments(
    output_dir=OUTPUT_DIR,
    per_device_train_batch_size=%d,
    gradient_accumulation_steps=%d,
    num_train_epochs=%d,
    learning_rate=%f,
    warmup_steps=%d,
    weight_decay=%f,
    logging_steps=%d,
    save_steps=%d,
    eval_steps=%d,
    save_total_limit=3,
    load_best_model_at_end=False,
    optim="%s",
    lr_scheduler_type="%s"
)

# Initialize trainer
trainer = Trainer(
    model=model,
    args=training_args,
    train_dataset=tokenized_dataset,
    data_collator=DataCollatorForLanguageModeling(tokenizer, mlm=False)
)

# Train
trainer.train()

# Save adapter
model.save_pretrained(OUTPUT_DIR + "/adapter")
tokenizer.save_pretrained(OUTPUT_DIR + "/adapter")
`, job.ID, job.BaseModel, job.DataPath, job.OutputPath,
		config.Rank, config.Alpha, config.Dropout, formatStringSlice(config.TargetModules),
		config.BatchSize, config.GradientAccumulation, config.NumEpochs,
		config.LearningRate, config.WarmupSteps, config.WeightDecay,
		config.LoggingSteps, config.SaveSteps, config.EvalSteps,
		config.Optimizer, config.LRScheduler)

	return script, nil
}

// generateQLoRAScript generates Python code for QLoRA training
func (t *Trainer) generateQLoRAScript(job *TrainingJob, config *QLoRAConfig) (string, error) {
	// Similar to LoRA but with BitsAndBytesConfig for quantization
	script := fmt.Sprintf(`#!/usr/bin/env python3
# Auto-generated QLoRA training script for job %s

import torch
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    TrainingArguments,
    Trainer,
    DataCollatorForLanguageModeling,
    BitsAndBytesConfig
)
from peft import LoraConfig, get_peft_model, TaskType, prepare_model_for_kbit_training
from datasets import load_dataset

# Configuration
MODEL_NAME = "%s"
DATA_PATH = "%s"
OUTPUT_DIR = "%s"

# QLoRA config
BNB_CONFIG = BitsAndBytesConfig(
    load_in_4bit=%s,
    bnb_4bit_use_double_quant=%s,
    bnb_4bit_quant_type="%s",
    bnb_4bit_compute_dtype=torch.%s
)

LORA_R = %d
LORA_ALPHA = %d
LORA_DROPOUT = %f
TARGET_MODULES = %s

# Load quantized model
model = AutoModelForCausalLM.from_pretrained(
    MODEL_NAME,
    quantization_config=BNB_CONFIG,
    device_map="%s"
)
tokenizer = AutoTokenizer.from_pretrained(MODEL_NAME)
tokenizer.pad_token = tokenizer.eos_token

# Prepare for training
model = prepare_model_for_kbit_training(model)

# Configure LoRA
lora_config = LoraConfig(
    r=LORA_R,
    lora_alpha=LORA_ALPHA,
    target_modules=TARGET_MODULES,
    lora_dropout=LORA_DROPOUT,
    bias="none",
    task_type=TaskType.CAUSAL_LM
)

model = get_peft_model(model, lora_config)
model.print_trainable_parameters()

# Load and preprocess dataset
dataset = load_dataset("json", data_files=DATA_PATH, split="train")

def format_example(example):
    """Format chat example into training text"""
    system = example.get("system", "")
    instruction = example.get("instruction", "")
    input_text = example.get("input", "")
    output = example.get("output", "")
    
    if system:
        text = f"<|system|>\n{system}\n<|user|>\n{instruction}"
    else:
        text = f"<|user|>\n{instruction}"
    
    if input_text:
        text += f"\n{input_text}"
    
    text += f"\n<|assistant|>\n{output}"
    return text

def preprocess_function(examples):
    """Tokenize the formatted examples - handles batched data"""
    # examples is a dict of lists when batched=True
    texts = []
    for i in range(len(examples['instruction'])):
        system = examples.get('system', [''] * len(examples['instruction']))[i]
        instruction = examples['instruction'][i]
        input_text = examples.get('input', [''] * len(examples['instruction']))[i]
        output = examples['output'][i]
        
        if system:
            text = f"<|system|>\n{system}\n<|user|>\n{instruction}"
        else:
            text = f"<|user|>\n{instruction}"
        
        if input_text:
            text += f"\n{input_text}"
        
        text += f"\n<|assistant|>\n{output}"
        texts.append(text)
    
    model_inputs = tokenizer(
        texts,
        truncation=True,
        max_length=1024,
        padding="max_length",
        return_tensors=None
    )
    # Labels are same as input_ids for causal LM
    model_inputs["labels"] = model_inputs["input_ids"].copy()
    return model_inputs

# Tokenize dataset
tokenized_dataset = dataset.map(
    preprocess_function,
    batched=True,
    remove_columns=dataset.column_names,
    desc="Tokenizing dataset"
)

# Training arguments
training_args = TrainingArguments(
    output_dir=OUTPUT_DIR,
    per_device_train_batch_size=%d,
    gradient_accumulation_steps=%d,
    num_train_epochs=%d,
    learning_rate=%f,
    warmup_steps=%d,
    weight_decay=%f,
    logging_steps=%d,
    save_steps=%d,
    eval_steps=%d,
    save_total_limit=3,
    load_best_model_at_end=False,
    optim="%s",
    lr_scheduler_type="%s",
    fp16=True
)

# Initialize trainer
trainer = Trainer(
    model=model,
    args=training_args,
    train_dataset=tokenized_dataset,
    data_collator=DataCollatorForLanguageModeling(tokenizer, mlm=False)
)

# Train
trainer.train()

# Save adapter
model.save_pretrained(OUTPUT_DIR + "/adapter")
tokenizer.save_pretrained(OUTPUT_DIR + "/adapter")
`, job.ID, job.BaseModel, job.DataPath, job.OutputPath,
		formatBool(config.Bits == 4), formatBool(config.DoubleQuant), config.QuantType, config.ComputeDtype,
		config.Rank, config.Alpha, config.Dropout, formatStringSlice(config.TargetModules), config.DeviceMap,
		config.BatchSize, config.GradientAccumulation, config.NumEpochs,
		config.LearningRate, config.WarmupSteps, config.WeightDecay,
		config.LoggingSteps, config.SaveSteps, config.EvalSteps,
		config.Optimizer, config.LRScheduler)

	return script, nil
}

// Helper functions
func generateJobID() string {
	return fmt.Sprintf("job_%d", os.Getpid())
}

func getCurrentTimestamp() string {
	return "2024-01-01T00:00:00Z" // Placeholder
}

// formatBool converts Go bool to Python boolean string
func formatBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
func formatStringSlice(slice []string) string {
	var quoted []string
	for _, s := range slice {
		quoted = append(quoted, fmt.Sprintf("%q", s))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
