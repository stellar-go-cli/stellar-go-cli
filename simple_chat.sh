#!/bin/bash

# Simple MozartPay Chat Launcher
# Usage: ./simple_chat.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check if mozartpay binary exists
if [ ! -f "$SCRIPT_DIR/mozartpay" ]; then
    echo "🔨 Building mozartpay..."
    cd "$SCRIPT_DIR"
    go build ./cmd/stellar-go-cli
    if [ $? -ne 0 ]; then
        echo "❌ Failed to build mozartpay"
        exit 1
    fi
    echo "✅ Build complete"
fi

# Check if Python 3 is available
if ! command -v python3 &> /dev/null; then
    echo "❌ Python 3 is required but not installed"
    exit 1
fi

# Run the simple chat interface
cd "$SCRIPT_DIR"
python3 simple_chat.py
