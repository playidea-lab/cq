#!/bin/bash
set -e

echo "=== Worker Environment ==="
echo "hostname: $(hostname)"
echo "pwd: $(pwd)"
echo "python: $(which python3 2>/dev/null || echo 'not found')"
echo "pip: $(which pip3 2>/dev/null || which pip 2>/dev/null || echo 'not found')"
echo "uv: $(which uv 2>/dev/null || echo 'not found')"
echo "ls data/mnist/:"
ls -la data/mnist/ 2>/dev/null || echo "data/mnist not found"
echo "=========================="

# Install deps
if command -v uv &>/dev/null; then
    uv pip install --system scikit-learn numpy 2>&1 || pip install scikit-learn numpy 2>&1
elif command -v pip3 &>/dev/null; then
    pip3 install scikit-learn numpy --quiet 2>&1
elif command -v pip &>/dev/null; then
    pip install scikit-learn numpy --quiet 2>&1
else
    echo "ERROR: no pip/uv found"
    exit 1
fi

# Run
python3 train.py
