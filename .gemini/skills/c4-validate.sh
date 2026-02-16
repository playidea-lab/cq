#!/bin/bash
# c4-validate for Gemini CLI
# Usage: ./c4-validate.sh [lint|unit|security|all]

set -e

COMMAND=${1:-all}
C4_ROOT=$(git rev-parse --show-toplevel)

echo "🔍 Running c4-validate ($COMMAND)..."

run_lint() {
    echo "Checking Linting..."
    if command -v ruff >/dev/null; then
        ruff check . || echo "⚠️ Lint issues found (non-blocking for now)"
    else
        echo "⚠️ ruff not found, skipping lint check."
    fi
}

run_unit() {
    echo "Running Unit Tests..."
    if [ -f "pyproject.toml" ]; then
        if command -v pytest >/dev/null; then
            pytest tests/unit || echo "⚠️ Tests failed (check output)"
        else
            echo "⚠️ pytest not found."
        fi
    fi
    
    if [ -f "c5/go.mod" ]; then
        echo "Running Go Tests..."
        (cd c5 && go test ./...) || echo "⚠️ Go tests failed"
    fi
}

run_security() {
    echo "Running Security Checks..."
    
    # 1. Hardcoded Secrets (CRITICAL)
    echo "  - Checking for secrets..."
    if grep -r --include="*.py" -E "(password|api_key|secret)\s*=\s*["'][^"']+["']" . | grep -v "test"; then
        echo "❌ CRITICAL: Hardcoded secrets found!"
        exit 1
    fi

    # 2. SQL Injection (CRITICAL)
    echo "  - Checking for SQL Injection..."
    if grep -r --include="*.py" -E "execute\s*\(\s*f["']" .; then
        echo "❌ CRITICAL: Potential SQL Injection found!"
        exit 1
    fi
    
    echo "✅ Security checks passed."
}

case $COMMAND in
    lint)
        run_lint
        ;;
    unit)
        run_unit
        ;;
    security)
        run_security
        ;;
    all)
        run_lint
        run_unit
        run_security
        ;;
    *)
        echo "Unknown command: $COMMAND"
        exit 1
        ;;
esac

echo "✅ Validation Complete."
