#!/usr/bin/env bash
set -euo pipefail

# CQ Installer — One-line setup for CQ AI Orchestration System
#
# Local:   ./install.sh
# Remote:  curl -sSL https://git.pilab.co.kr/pi/cq/raw/main/install.sh | bash
#
# When piped via curl, the script auto-clones the repo first.

C4_REPO="https://git.pilab.co.kr/pi/cq.git"
C4_DEFAULT_DIR="$HOME/cq"

# ─── Flag Parsing ─────────────────────────────────────────────

WITH_HUB=false
DRY_RUN=false
for arg in "$@"; do
    case "$arg" in
        --with-hub) WITH_HUB=true ;;
        --dry-run) DRY_RUN=true ;;
    esac
done

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

# ─── Project Root (auto-clone if needed) ──────────────────

detect_or_clone() {
    # Case 1: Running locally inside the repo (./install.sh)
    if [ -n "${BASH_SOURCE[0]:-}" ] && [ "${BASH_SOURCE[0]}" != "" ] && [ -f "${BASH_SOURCE[0]}" ]; then
        local script_dir
        script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if [ -d "$script_dir/c4-core" ]; then
            C4_ROOT="$script_dir"
            return
        fi
    fi

    # Case 2: Piped via curl — need to clone
    if ! command -v git &>/dev/null; then
        fail "git not found. Install git first."
    fi

    local install_dir="${C4_INSTALL_DIR:-$C4_DEFAULT_DIR}"

    if [ -d "$install_dir/c4-core" ]; then
        info "Existing C4 found at $install_dir, updating..."
        cd "$install_dir" && git pull --ff-only
        C4_ROOT="$install_dir"
    else
        info "Cloning C4 to $install_dir..."
        git clone "$C4_REPO" "$install_dir"
        C4_ROOT="$install_dir"
    fi
}

printf "\n${BOLD}CQ Installer${NC}\n"
printf "─────────────────\n"

detect_or_clone
ok "Project root: $C4_ROOT"

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

# ─── Cloud Defaults (built into binary) ────────────────────
# These are PUBLIC values (anon key + RLS = safe to embed).
# Override with C4_SUPABASE_URL / C4_SUPABASE_KEY env vars at build time.
C4_SB_URL="${C4_SUPABASE_URL:-}"
C4_SB_KEY="${C4_SUPABASE_KEY:-}"

info "Building Go binary..."
cd "$C4_ROOT/c4-core"
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
LDFLAGS="-X main.version=$VERSION -X main.builtinC4Root=$C4_ROOT"
if [ -n "$C4_SB_URL" ]; then
    LDFLAGS="$LDFLAGS -X main.builtinSupabaseURL=$C4_SB_URL"
fi
if [ -n "$C4_SB_KEY" ]; then
    LDFLAGS="$LDFLAGS -X main.builtinSupabaseKey=$C4_SB_KEY"
fi
mkdir -p bin
go build -ldflags "$LDFLAGS" -o bin/cq ./cmd/c4/
ok "Go binary built (c4-core/bin/cq)"

# ─── C5 Hub Binary Build (optional) ─────────────────────────
if [ "$WITH_HUB" = true ]; then
    info "Building C5 Hub binary..."
    cd "$C4_ROOT/c5"
    C5_LDFLAGS="-X main.version=$VERSION"
    if [ "$DRY_RUN" = true ]; then
        ok "C5 Hub binary (dry-run, skipped)"
    else
        mkdir -p bin
        go build -ldflags "$C5_LDFLAGS" -o bin/c5 ./cmd/c5/
        ok "C5 Hub binary built (c5/bin/c5)"
    fi
    cd "$C4_ROOT"
fi

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
  "command": "$C4_ROOT/c4-core/bin/cq",
  "args": ["mcp", "--dir", "$C4_ROOT"],
  "env": {
    "C4_PROJECT_ROOT": "$C4_ROOT"
  }
}
EOF
)

if [ -f "$MCP_JSON" ]; then
    # Existing .mcp.json — merge cq entry, preserve other servers
    if command -v jq &>/dev/null; then
        # jq available: surgical update
        jq --argjson cq "$C4_ENTRY" '.mcpServers.cq = $cq' "$MCP_JSON" > "${MCP_JSON}.tmp"
        mv "${MCP_JSON}.tmp" "$MCP_JSON"
    else
        # Fallback: Python JSON merge
        python3 -c "
import json, sys
with open('$MCP_JSON') as f:
    data = json.load(f)
data.setdefault('mcpServers', {})['cq'] = json.loads('''$C4_ENTRY''')
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
    "cq": $C4_ENTRY
  }
}
MCPEOF
    ok ".mcp.json created"
fi

# ─── Global Install (optional) ────────────────────────────

INSTALL_GLOBAL="${C4_GLOBAL_INSTALL:-}"
if [ -z "$INSTALL_GLOBAL" ] && [ -t 0 ]; then
    # Interactive terminal — ask user
    printf "\n"
    read -rp "  Install cq globally to ~/.local/bin/cq? [y/N] " INSTALL_GLOBAL
elif [ -z "$INSTALL_GLOBAL" ]; then
    # Piped (curl | bash) — skip by default
    INSTALL_GLOBAL="n"
fi
if [[ "$(echo "$INSTALL_GLOBAL" | tr '[:upper:]' '[:lower:]')" =~ ^y ]]; then
    mkdir -p "$HOME/.local/bin"
    # CRITICAL: Use go build -o, NOT cp (macOS ARM64 code signing)
    info "Building global binary..."
    cd "$C4_ROOT/c4-core"
    go build -ldflags "$LDFLAGS" -o "$HOME/.local/bin/cq" ./cmd/c4/
    ok "Global binary installed (~/.local/bin/cq)"

    # Check if ~/.local/bin is in PATH
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$HOME/.local/bin"; then
        warn "~/.local/bin is not in PATH. Add to your shell profile:"
        info '  export PATH="$HOME/.local/bin:$PATH"'
    fi
fi

if [ "$WITH_HUB" = true ] && [[ "$(echo "$INSTALL_GLOBAL" | tr '[:upper:]' '[:lower:]')" =~ ^y ]]; then
    info "Building C5 Hub global binary..."
    cd "$C4_ROOT/c5"
    go build -ldflags "$C5_LDFLAGS" -o "$HOME/.local/bin/c5" ./cmd/c5/
    ok "C5 Hub global binary installed (~/.local/bin/c5)"
    cd "$C4_ROOT"
fi

# ─── .c4/ Directory Init ───────────────────────────────────

mkdir -p "$C4_ROOT/.c4/knowledge/docs"
ok ".c4/ directory initialized"

# ─── Record C4 Install Path ──────────────────────────────────

echo "$C4_ROOT" > "$HOME/.c4-install-path"
ok "Install path recorded (~/.c4-install-path)"

# ─── Verification ───────────────────────────────────────────

INSTALLED_VER="$("$C4_ROOT/c4-core/bin/cq" --version 2>&1 | head -1 || true)"
if [ -z "$INSTALLED_VER" ]; then
    warn "Binary built but version check failed"
else
    ok "Verified: $INSTALLED_VER"
fi

# ─── Done ───────────────────────────────────────────────────

printf "\n${GREEN}${BOLD}CQ $VERSION installed successfully!${NC}\n"
printf "  Location: ${BOLD}$C4_ROOT${NC}\n"
printf "  Binary:   ${BOLD}$C4_ROOT/c4-core/bin/cq${NC}\n"
printf "\n${BOLD}Next steps:${NC}\n"
printf "  1. Restart Claude Code to activate MCP tools\n"
printf "  2. Run ${CYAN}cq auth login${NC} to sign in (required for cloud features)\n\n"

if [ "$WITH_HUB" = true ]; then
    printf "${BOLD}C5 Hub:${NC}\n"
    printf "  Binary: ${BOLD}$C4_ROOT/c5/bin/c5${NC}\n"
    printf "  Start:  ${CYAN}c5 serve --port 8585 --db ./c5.db${NC}\n"
    printf "  Docker: ${CYAN}cd c5 && docker compose up -d${NC}\n\n"
fi
