# Chat Model Fine-Tuning Guide

This guide explains how to fine-tune language models for the MozartPay CLI chat interface using LoRA/QLoRA and GGUF quantization.

## Overview

The fine-tuning pipeline enables you to create domain-specific models for:
- **Intent Classification**: Identify user intents (payment, swap, balance check, etc.)
- **Parameter Extraction**: Extract values like amounts, addresses, asset codes
- **Response Generation**: Create natural, helpful responses

## Architecture

```
Training Data → LoRA/QLoRA Training → GGUF Export → Ollama Integration
      ↓                ↓                    ↓              ↓
  Synthetic      PEFT + Transformers    llama.cpp     MozartPay CLI
  Generation     4-bit Quantization     Quantization  Inference
```

## Prerequisites

### System Requirements
- **CPU Training**: Any modern CPU (slower)
- **GPU Training**: NVIDIA GPU with 8GB+ VRAM (recommended)
- **Disk Space**: 10GB+ for models and training data
- **RAM**: 16GB+ recommended

### Software Dependencies

1. **Python 3.9+** with required packages:
```bash
pip install torch transformers peft bitsandbytes datasets accelerate
```

2. **llama.cpp** for GGUF conversion:
```bash
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp
make  # or cmake build for your platform
```

3. **Ollama** (optional, for serving models):
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

## Quick Start

### 1. Generate Training Data

Create synthetic training examples from existing prompts:

```bash
# Generate 100 intent examples and 50 parameter examples per type
mozartpay chat train-data --generate --intents 100 --params 50

# Generate with ChatML format (better for modern models)
mozartpay chat train-data --generate --format chatml --output ./training.jsonl
```

### 2. Validate Training Data

```bash
mozartpay chat train-data --validate ./training.jsonl
```

### 3. Fine-Tune with LoRA

```bash
# LoRA training (faster, requires more VRAM)
mozartpay chat finetune --method lora \
  --base-model llama3.2:3b \
  --data ./training.jsonl \
  --name mozartpay-intent-v1 \
  --rank 32 \
  --alpha 64 \
  --epochs 5
```

### 4. Fine-Tune with QLoRA (Recommended)

```bash
# QLoRA training (4-bit, works with less VRAM)
mozartpay chat finetune --method qlora \
  --base-model llama3.2:3b \
  --data ./training.jsonl \
  --name mozartpay-qlora-v1 \
  --bits 4 \
  --rank 32 \
  --epochs 3
```

### 5. Check Training Status

```bash
mozartpay chat finetune --status <job-id>
```

### 6. Export to GGUF

```bash
# Export with multiple quantization levels
mozartpay chat model --export <job-id> \
  --quantizations Q4_K_M,Q5_K_M,Q8_0 \
  --register-ollama
```

### 7. Use the Fine-Tuned Model

```bash
# List available models
mozartpay chat model --list

# Set as active model
mozartpay chat model --use mozartpay-intent-v1-q4_k_m

# Start chat with fine-tuned model
mozartpay chat
```

## Training Data Format

### Alpaca Format (Default)
```json
{
  "instruction": "Classify the user intent for this MozartPay CLI command",
  "input": "Send 100 USDC to GABC...",
  "output": "{\"intent\": \"pay_send\", \"confidence\": 0.95}",
  "system": "You are an intent classifier for MozartPay..."
}
```

### ChatML Format (Recommended for modern models)
```json
{
  "messages": [
    {"role": "system", "content": "You are MozartPay, a crypto payment assistant."},
    {"role": "user", "content": "Classify: Send 100 USDC to GABC..."},
    {"role": "assistant", "content": "{\"intent\": \"pay_send\", \"confidence\": 0.95}"}
  ]
}
```

## LoRA/QLoRA Configuration

### LoRA Hyperparameters

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `rank` | 32 | 8-128 | LoRA rank (r). Higher = more capacity but slower |
| `alpha` | 64 | 16-256 | LoRA alpha. Typically 2x rank |
| `dropout` | 0.05 | 0-0.1 | Dropout for regularization |
| `target_modules` | q,v,k,o_proj | - | Which layers to adapt |
| `learning_rate` | 2e-4 | 1e-4 - 5e-4 | Initial learning rate |
| `batch_size` | 2 | 1-8 | Per-device batch size |
| `epochs` | 3 | 1-10 | Training epochs |

### QLoRA Quantization

| Bits | VRAM Required | Speed | Quality |
|------|--------------|-------|---------|
| 4-bit | 6-8GB | Fast | Good |
| 8-bit | 10-12GB | Medium | Better |

Recommended: **4-bit with NF4 and double quantization** for most use cases.

## GGUF Quantization Levels

| Type | Size | Quality | Use Case |
|------|------|---------|----------|
| Q4_K_M | ~4GB | Medium-High | Balanced (recommended) |
| Q5_K_M | ~5GB | High | Better accuracy |
| Q8_0 | ~7GB | Very High | Maximum quality |

## Model Registry

The model registry stores all your fine-tuned models:

```
~/.mozartpay/
├── models/
│   ├── base/           # Base models (Ollama pulled)
│   ├── adapters/       # LoRA adapters (training output)
│   ├── gguf/          # Quantized GGUF files
│   └── ollama/        # Ollama-compatible models
└── registry.json      # Model metadata
```

### Registry Commands

```bash
# Show all models
mozartpay chat model --list

# Show registry statistics
mozartpay chat model --stats

# Set active model
mozartpay chat model --use <model-id>
```

## Advanced Usage

### Custom Training Data

Create your own training data:

```jsonl
{"instruction": "Classify intent", "input": "Check my balance", "output": "{\"intent\": \"wallet_balance\"}", "system": "You are MozartPay..."}
{"instruction": "Classify intent", "input": "Send 50 XLM to GABC...", "output": "{\"intent\": \"pay_send\"}", "system": "You are MozartPay..."}
```

Save as `custom-training.jsonl` and use:
```bash
mozartpay chat finetune --data ./custom-training.jsonl ...
```

### Multiple Quantization Exports

Export one model in multiple sizes:

```bash
mozartpay chat model --export job-123 \
  --quantizations Q4_K_M,Q5_K_M,Q6_K,Q8_0 \
  --register-ollama
```

### Environment Variables

```bash
# Set llama.cpp path
export LLAMA_CPP_PATH=/usr/local/llama.cpp

# Set Python executable for training
export PYTHON_PATH=/usr/bin/python3.11

# Enable debug logging
export MOZARTPAY_DEBUG=1
```

## Troubleshooting

### Training Fails: Out of Memory
- Use QLoRA instead of LoRA: `--method qlora --bits 4`
- Reduce batch size: `--batch-size 1`
- Reduce sequence length in config
- Use a smaller base model (e.g., 1B instead of 7B)

### llama.cpp Tools Not Found
```bash
# Set the correct path
export LLAMA_CPP_PATH=/path/to/llama.cpp

# Verify tools exist
ls $LLAMA_CPP_PATH/convert_hf_to_gguf.py
ls $LLAMA_CPP_PATH/llama-quantize
```

### Ollama Registration Fails
```bash
# Ensure Ollama is running
ollama serve

# Or use system service
sudo systemctl start ollama
```

### Model Quality Issues
1. **Increase training data**: Generate more synthetic examples
2. **More epochs**: Try `--epochs 5-10`
3. **Higher rank**: Increase `--rank 64-128`
4. **Better quantization**: Use Q5_K_M or Q8_0 instead of Q4_K_M

## Best Practices

1. **Start Small**: Begin with 100-200 examples and 3 epochs
2. **Validate First**: Always validate training data before training
3. **Use QLoRA**: Most efficient for most hardware
4. **Test Multiple Quants**: Compare Q4_K_M vs Q5_K_M for your use case
5. **Version Your Models**: Use descriptive names like `mozartpay-intent-v1`
6. **Monitor Metrics**: Watch training loss for overfitting

## Example: Complete Workflow

```bash
# 1. Setup
export LLAMA_CPP_PATH=~/llama.cpp

# 2. Generate data
mozartpay chat train-data --generate --intents 200 --params 100 --format chatml

# 3. Validate
mozartpay chat train-data --validate ./training-data.jsonl

# 4. Train with QLoRA
mozartpay chat finetune --method qlora \
  --base-model llama3.2:3b \
  --data ./training-data.jsonl \
  --name mozartpay-production-v1 \
  --rank 64 \
  --alpha 128 \
  --epochs 5

# 5. Wait for completion, then export
mozartpay chat model --export <job-id> \
  --quantizations Q4_K_M,Q5_K_M \
  --register-ollama

# 6. Set as active
mozartpay chat model --use mozartpay-production-v1-q5_k_m

# 7. Test
mozartpay chat
> Send 100 USDC to GABC123...
```

## API Reference

See the following source files for implementation details:
- `internal/chat/training/` - Training data pipeline
- `internal/chat/finetune/` - LoRA/QLoRA training
- `internal/chat/gguf/` - GGUF export
- `internal/chat/models/` - Model registry
- `cmd/stellar-go-cli/commands/chat_finetune.go` - CLI commands

## Contributing

To add new intent patterns for synthetic data generation, edit:
`internal/chat/training/synthetic.go` - Add to `DefaultIntentPatterns`

## License

The fine-tuning infrastructure follows the same license as MozartPay CLI.
