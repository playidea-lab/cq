#!/usr/bin/env bash
set -euo pipefail

# C4 Installer — One-line setup for C4 AI Orchestration System
# Usage: ./install.sh  or  curl -sSL https://... | sh

# ─── Colors & Helpers ───────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { printf "  ${GREEN}[✓]${NC} %s\n" "$1"; }
warn() { printf "  ${YELLOW}[!]${NC} %s\n" "$1"; }
fail() { printf "  ${RED}[✗]${NC} %s\n" "$1"; exit 1; }
info() { printf "  ${CYAN}[·]${NC} %s\n" "$1"; }

# ─── Project Root ───────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
C4_ROOT="$SCRIPT_DIR"

printf "\n${BOLD}C4 Installer${NC}\n"
printf "─────────────────\n"

# ─── Dependency Checks ─────────────────────────────────────

# version_ge: returns 0 if $1 >= $2 (semver-ish comparison)
version_ge() {
    printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1 | grep -qx "$2"
}

# Check Go (1.22+)
if ! command -v go &>/dev/null; then
    fail "Go not found. Install Go 1.22+ from https://go.dev/dl/"
fi
GO_VER="$(go version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
if ! version_ge "$GO_VER" "1.22"; then
    fail "Go $GO_VER detected, but 1.22+ required"
fi
ok "Go $GO_VER detected"

# Check Python (3.11+)
if ! command -v python3 &>/dev/null; then
    fail "Python3 not found. Install Python 3.11+ from https://python.org"
fi
PY_VER="$(python3 --version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
if ! version_ge "$PY_VER" "3.11"; then
    fail "Python $PY_VER detected, but 3.11+ required"
fi
ok "Python $PY_VER detected"

# Check uv
if ! command -v uv &>/dev/null; then
    fail "uv not found. Install: curl -LsSf https://astral.sh/uv/install.sh | sh"
fi
UV_VER="$(uv --version | grep -oE '[0-9]+\.[0-9]+(\.[0-9]+)?' | head -1)"
ok "uv $UV_VER detected"

# ─── Go Binary Build ───────────────────────────────────────

info "Building Go binary..."
cd "$C4_ROOT/c4-core"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
mkdir -p bin
go build -ldflags "-X main.version=$VERSION" -o bin/c4 ./cmd/c4/
ok "Go binary built (c4-core/bin/c4)"

# ─── Python Dependencies ───────────────────────────────────

info "Installing Python dependencies..."
cd "$C4_ROOT"
uv sync --quiet
ok "Python dependencies installed"

# ─── .mcp.json Configuration ───────────────────────────────

MCP_JSON="$C4_ROOT/.mcp.json"
C4_ENTRY=$(cat <<EOF
{
  "type": "stdio",
  "command": "$C4_ROOT/c4-core/bin/c4",
  "args": ["mcp", "--dir", "$C4_ROOT"],
  "env": {
    "C4_PROJECT_ROOT": "$C4_ROOT"
  }
}
EOF
)

if [ -f "$MCP_JSON" ]; then
    # Existing .mcp.json — merge c4 entry, preserve other servers
    if command -v jq &>/dev/null; then
        # jq available: surgical update
        jq --argjson c4 "$C4_ENTRY" '.mcpServers.c4 = $c4' "$MCP_JSON" > "${MCP_JSON}.tmp"
        mv "${MCP_JSON}.tmp" "$MCP_JSON"
    else
        # Fallback: Python JSON merge
        python3 -c "
import json, sys
with open('$MCP_JSON') as f:
    data = json.load(f)
data.setdefault('mcpServers', {})['c4'] = json.loads('''$C4_ENTRY''')
with open('$MCP_JSON', 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
"
    fi
    ok ".mcp.json updated (existing servers preserved)"
else
    # Create new .mcp.json
    cat > "$MCP_JSON" <<MCPEOF
{
  "mcpServers": {
    "c4": $C4_ENTRY
  }
}
MCPEOF
    ok ".mcp.json created"
fi

# ─── Global Install (optional) ────────────────────────────

printf "\n"
read -rp "  Install c4 globally to ~/.local/bin/c4? [y/N] " INSTALL_GLOBAL
if [[ "${INSTALL_GLOBAL,,}" =~ ^y ]]; then
    mkdir -p "$HOME/.local/bin"
    # CRITICAL: Use go build -o, NOT cp (macOS ARM64 code signing)
    info "Building global binary..."
    cd "$C4_ROOT/c4-core"
    go build -ldflags "-X main.version=$VERSION" -o "$HOME/.local/bin/c4" ./cmd/c4/
    ok "Global binary installed (~/.local/bin/c4)"

    # Check if ~/.local/bin is in PATH
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$HOME/.local/bin"; then
        warn "~/.local/bin is not in PATH. Add to your shell profile:"
        info '  export PATH="$HOME/.local/bin:$PATH"'
    fi
fi

# ─── .c4/ Directory Init ───────────────────────────────────

mkdir -p "$C4_ROOT/.c4/knowledge/docs"
ok ".c4/ directory initialized"

# ─── Verification ───────────────────────────────────────────

INSTALLED_VER="$("$C4_ROOT/c4-core/bin/c4" --version 2>&1 | head -1 || true)"
if [ -z "$INSTALLED_VER" ]; then
    warn "Binary built but version check failed"
else
    ok "Verified: $INSTALLED_VER"
fi

# ─── Done ───────────────────────────────────────────────────

printf "\n${GREEN}${BOLD}C4 $VERSION installed successfully!${NC}\n"
printf "Restart Claude Code to activate.\n\n"
