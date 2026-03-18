#!/usr/bin/env bash
set -euo pipefail

# CQ Installer — One-line setup for CQ AI Orchestration System
#
# Local:   ./install.sh [options]
# Remote:  curl -sSL https://git.pilab.co.kr/pi/cq/raw/main/install.sh | bash
#
# Options:
#   --global-mcp      Register cq in ~/.mcp.json (all-project access)
#   --global-skills   Symlink skills to ~/.claude/commands/ (global slash commands)
#   --with-hub        Also build the C5 Hub binary
#   --dry-run         Skip actual builds/installs
#
# When piped via curl, the script auto-clones the repo first.

C4_REPO="https://git.pilab.co.kr/pi/cq.git"
C4_DEFAULT_DIR="$HOME/cq"

# ─── Flag Parsing ─────────────────────────────────────────────

WITH_HUB=false
DRY_RUN=false
GLOBAL_MCP=false
GLOBAL_SKILLS=false
for arg in "$@"; do
    case "$arg" in
        --with-hub)      WITH_HUB=true ;;
        --dry-run)       DRY_RUN=true ;;
        --global-mcp)    GLOBAL_MCP=true ;;
        --global-skills) GLOBAL_SKILLS=true ;;
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

# ─── Windows Detection ────────────────────────────────────

_os="$(uname -s 2>/dev/null || true)"
case "$_os" in
    MINGW*|MSYS*|CYGWIN*)
        printf "\n  ${YELLOW}[!]${NC} Windows detected (${_os}).\n"
        printf "\n  CQ requires a POSIX environment. Two options:\n\n"
        printf "  ${BOLD}Option 1 — WSL2 (recommended)${NC}\n"
        printf "    1. Install WSL2:  ${CYAN}wsl --install${NC}  (PowerShell as Admin)\n"
        printf "    2. Open Ubuntu terminal and re-run this installer.\n\n"
        printf "  ${BOLD}Option 2 — Pre-built binary from GitHub Releases${NC}\n"
        printf "    Download the latest release for your platform:\n"
        printf "    ${CYAN}https://github.com/pilab-dev/cq/releases/latest${NC}\n\n"
        exit 0
        ;;
esac

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
    printf "\n  ${RED}[✗]${NC} Go not found. Install Go 1.22+:\n"
    printf "       macOS:  ${CYAN}brew install go${NC}\n"
    printf "       Linux:  ${CYAN}curl -sSL https://go.dev/dl/go1.22.0.linux-amd64.tar.gz | sudo tar -C /usr/local -xz${NC}\n"
    printf "       All:    ${CYAN}https://go.dev/dl/${NC}\n\n"
    exit 1
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

# Helper: write/update a .mcp.json file with the cq entry.
# Usage: write_mcp_json <path> <binary_path> <dir_path>
write_mcp_json() {
    local mcp_file="$1"
    local bin_path="$2"
    local dir_path="$3"

    local entry
    entry=$(cat <<EOF
{
  "type": "stdio",
  "command": "$bin_path",
  "args": ["mcp", "--dir", "$dir_path"],
  "env": {
    "C4_PROJECT_ROOT": "$dir_path"
  }
}
EOF
)

    if [ -f "$mcp_file" ]; then
        if command -v jq &>/dev/null; then
            jq --argjson cq "$entry" '.mcpServers.cq = $cq' "$mcp_file" > "${mcp_file}.tmp"
            mv "${mcp_file}.tmp" "$mcp_file"
        else
            python3 -c "
import json
with open('$mcp_file') as f:
    data = json.load(f)
data.setdefault('mcpServers', {})['cq'] = json.loads('''$entry''')
with open('$mcp_file', 'w') as f:
    json.dump(data, f, indent=2)
    f.write('\n')
"
        fi
        ok "$(basename "$mcp_file") updated at $mcp_file"
    else
        mkdir -p "$(dirname "$mcp_file")"
        cat > "$mcp_file" <<MCPEOF
{
  "mcpServers": {
    "cq": $entry
  }
}
MCPEOF
        ok "$(basename "$mcp_file") created at $mcp_file"
    fi
}

# Project-local .mcp.json (always)
write_mcp_json "$C4_ROOT/.mcp.json" "$C4_ROOT/c4-core/bin/cq" "$C4_ROOT"

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
GLOBAL_INSTALLED=false
if [[ "$(echo "$INSTALL_GLOBAL" | tr '[:upper:]' '[:lower:]')" =~ ^y ]]; then
    GLOBAL_INSTALLED=true
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

if [ "$WITH_HUB" = true ] && [ "$GLOBAL_INSTALLED" = true ]; then
    info "Building C5 Hub global binary..."
    cd "$C4_ROOT/c5"
    go build -ldflags "$C5_LDFLAGS" -o "$HOME/.local/bin/c5" ./cmd/c5/
    ok "C5 Hub global binary installed (~/.local/bin/c5)"
    cd "$C4_ROOT"
fi

# ─── Global MCP Registration (--global-mcp) ───────────────
#
# Writes/updates ~/.mcp.json so cq tools are available in ALL projects.
# The global server uses ~/.local/bin/cq (must be globally installed).
# Each per-project .mcp.json overrides the global entry for that project.

if [ "$GLOBAL_MCP" = true ]; then
    GLOBAL_BIN="$HOME/.local/bin/cq"
    if [ ! -f "$GLOBAL_BIN" ]; then
        warn "--global-mcp requires global binary. Re-run with 'y' at the global install prompt."
        warn "Skipping ~/.mcp.json update."
    else
        # Global server uses C4_ROOT as its --dir so it has access to its own .c4/ DB.
        # Per-project servers (via cq init) override this for project-specific state.
        write_mcp_json "$HOME/.mcp.json" "$GLOBAL_BIN" "$C4_ROOT"
        printf "\n"
        info "Global MCP server registered. To activate in Claude Code:"
        info "  Settings → MCP → enable 'cq' server, then restart Claude Code."
        info "  Or add to ~/.claude/settings.json:"
        info '    { "enabledMcpjsonServers": ["cq"] }'
    fi
fi

# ─── Global Skills Symlinks (--global-skills) ─────────────
#
# Symlinks .claude/skills/*.md to ~/.claude/commands/ so /c4-plan, /c4-run,
# etc. are available as slash commands in ALL projects.

if [ "$GLOBAL_SKILLS" = true ]; then
    SKILLS_SRC="$C4_ROOT/.claude/skills"
    COMMANDS_DST="$HOME/.claude/commands"

    if [ ! -d "$SKILLS_SRC" ]; then
        warn "Skills directory not found: $SKILLS_SRC"
    else
        mkdir -p "$COMMANDS_DST"
        linked=0
        skipped=0
        for skill_file in "$SKILLS_SRC"/*.md; do
            [ -f "$skill_file" ] || continue
            skill_name="$(basename "$skill_file")"
            target="$COMMANDS_DST/$skill_name"
            if [ -L "$target" ] && [ "$(readlink "$target")" = "$skill_file" ]; then
                skipped=$((skipped + 1))
            else
                ln -sf "$skill_file" "$target"
                linked=$((linked + 1))
            fi
        done
        ok "Skills symlinked to ~/.claude/commands/ (new=$linked, up-to-date=$skipped)"
        info "Restart Claude Code to pick up new slash commands (/c4-plan, /c4-run, etc.)"
    fi
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
if [ "$GLOBAL_MCP" = true ] && [ "$GLOBAL_INSTALLED" = true ]; then
    printf "  1. Enable MCP server in Claude Code:\n"
    printf "       Settings → MCP → enable ${CYAN}cq${NC}  (or add to ~/.claude/settings.json)\n"
    printf "       ${CYAN}{ \"enabledMcpjsonServers\": [\"cq\"] }${NC}\n"
    printf "  2. Restart Claude Code to activate MCP tools\n"
    printf "  3. Run ${CYAN}cq auth login${NC} to sign in (required for cloud features)\n\n"
    printf "  Per-project isolation: run ${CYAN}cq init${NC} in any project directory.\n"
    printf "  The project .mcp.json overrides the global entry for that project.\n\n"
else
    printf "  1. Restart Claude Code to activate MCP tools\n"
    printf "  2. Run ${CYAN}cq auth login${NC} to sign in (required for cloud features)\n"
    printf "  3. (Optional) For global access across all projects:\n"
    printf "       ${CYAN}./install.sh --global-mcp --global-skills${NC}\n\n"
fi

if [ "$WITH_HUB" = true ]; then
    printf "${BOLD}C5 Hub:${NC}\n"
    printf "  Binary: ${BOLD}$C4_ROOT/c5/bin/c5${NC}\n"
    printf "  Start:  ${CYAN}c5 serve --port 8585 --db ./c5.db${NC}\n"
    printf "  Docker: ${CYAN}cd c5 && docker compose up -d${NC}\n\n"
fi
