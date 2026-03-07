#!/usr/bin/env bash
# test-gpu-e2e.sh
# Hub + GPU worker E2E smoke test
#
# Usage:
#   bash scripts/test-gpu-e2e.sh --dry-run                        # env var check only (CI-safe)
#   C5_HUB_URL=https://... C5_HUB_API_KEY=... bash scripts/test-gpu-e2e.sh
#
# Required env vars:
#   C5_HUB_URL     — Hub base URL (e.g. https://hub.example.com)
#   C5_HUB_API_KEY — API key for Hub HTTP MCP endpoint (X-API-Key header)
#                    Note: distinct from C5_API_KEY used by c5 worker → Hub auth
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }
info() { echo -e "${CYAN}[INFO]${NC} $*"; }

PASS_COUNT=0
FAIL_COUNT=0

record() {
    local status="$1"; shift
    if [ "$status" = "pass" ]; then
        pass "$*"; PASS_COUNT=$((PASS_COUNT + 1))
    else
        fail "$*"; FAIL_COUNT=$((FAIL_COUNT + 1))
    fi
}

# ── --dry-run: env var validation only ────────────────────────────────
DRY_RUN=false
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
        *) echo "Unknown argument: $arg" >&2; echo "Usage: $0 [--dry-run]" >&2; exit 1 ;;
    esac
done

if [ "$DRY_RUN" = "true" ]; then
    info "Dry-run mode: checking env vars only (no Hub connection)"
    MISSING=false
    if [ -z "${C5_HUB_URL:-}" ]; then
        fail "C5_HUB_URL is not set"
        MISSING=true
    else
        pass "C5_HUB_URL is set (${C5_HUB_URL})"
    fi
    if [ -z "${C5_HUB_API_KEY:-}" ]; then
        fail "C5_HUB_API_KEY is not set"
        MISSING=true
    else
        pass "C5_HUB_API_KEY is set"
    fi
    if [ "$MISSING" = "true" ]; then
        echo ""
        fail "Missing required env vars. Export C5_HUB_URL and C5_HUB_API_KEY before running."
        exit 1
    fi
    echo ""
    pass "All env vars present. Remove --dry-run to run actual tests."
    exit 0
fi

# ── Default mode: real Hub connection tests ────────────────────────────
if [ -z "${C5_HUB_URL:-}" ] || [ -z "${C5_HUB_API_KEY:-}" ]; then
    fail "Missing required env vars"
    echo "  C5_HUB_URL      — ${C5_HUB_URL:-(not set)}"
    echo "  C5_HUB_API_KEY  — ${C5_HUB_API_KEY:+(set)}${C5_HUB_API_KEY:-(not set)}"
    exit 1
fi

HUB="${C5_HUB_URL%/}"
AUTH_HEADER="X-API-Key: ${C5_HUB_API_KEY}"

info "Hub: ${HUB}"
echo ""

# ── Step 1: Hub health check ───────────────────────────────────────────
info "Step 1: Hub health check (GET /v1/health)"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 \
    -H "${AUTH_HEADER}" \
    "${HUB}/v1/health" 2>/dev/null || echo "000")

if [ "$HTTP_CODE" = "200" ]; then
    record pass "Hub health check → HTTP ${HTTP_CODE}"
else
    record fail "Hub health check → HTTP ${HTTP_CODE} (expected 200)"
fi

# ── Step 2: MCP tools/list — gpu_status tool exists ───────────────────
info "Step 2: MCP tools/list (POST /v1/mcp)"
TOOLS_RESPONSE=$(curl -s --max-time 15 \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
    "${HUB}/v1/mcp" 2>/dev/null || echo "")

if echo "${TOOLS_RESPONSE}" | grep -q '"gpu_status"'; then
    record pass "MCP tools/list → gpu_status tool found"
else
    record fail "MCP tools/list → gpu_status tool NOT found"
    if [ -n "${TOOLS_RESPONSE}" ]; then
        info "Response snippet: $(echo "${TOOLS_RESPONSE}" | head -c 200)"
    fi
fi

# ── Step 3 + 4: MCP tools/call gpu_status — gpu_count key + latency ───
info "Step 3: MCP tools/call gpu_status (latency < 30s)"
T_START=$(date +%s)
GPU_RESPONSE=$(curl -s --max-time 30 \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gpu_status","arguments":{}}}' \
    "${HUB}/v1/mcp" 2>/dev/null || echo "")
T_END=$(date +%s)
LATENCY=$((T_END - T_START))

if [ "${LATENCY}" -lt 30 ]; then
    record pass "gpu_status call latency: ${LATENCY}s (< 30s)"
else
    record fail "gpu_status call latency: ${LATENCY}s (>= 30s)"
fi

info "Step 4: gpu_status response contains gpu_count key"
if echo "${GPU_RESPONSE}" | grep -q '"gpu_count"'; then
    record pass "gpu_status response → gpu_count key found"
else
    record fail "gpu_status response → gpu_count key NOT found"
    if [ -n "${GPU_RESPONSE}" ]; then
        info "Response snippet: $(echo "${GPU_RESPONSE}" | head -c 300)"
    fi
fi

# ── Summary ────────────────────────────────────────────────────────────
echo ""
echo "─────────────────────────────────────"
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo "Results: ${PASS_COUNT}/${TOTAL} passed"
if [ "${FAIL_COUNT}" -eq 0 ]; then
    pass "All steps passed."
    exit 0
else
    fail "${FAIL_COUNT} step(s) failed."
    exit 1
fi
