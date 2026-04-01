#!/usr/bin/env bash
set -euo pipefail

# CQ Installer — One-line setup for CQ AI Orchestration System
#
# Remote:  curl -sSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
# Local:   ./install.sh [options]
#
# Options:
#   --version <tag>   Install a specific version (default: latest)
#   --dry-run         Show what would be done without doing it
#
# For developers who need source builds: cd c4-core && make install

GITHUB_REPO="PlayIdea-Lab/cq"
INSTALL_DIR="$HOME/.local/bin"

# ─── Flag Parsing ─────────────────────────────────────────────

VERSION=""
DRY_RUN=false
for arg in "$@"; do
    case "$arg" in
        --version=*) VERSION="${arg#--version=}" ;;
        --dry-run)   DRY_RUN=true ;;
    esac
done

# ─── Colors & Helpers ───────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { printf "  ${GREEN}[ok]${NC} %s\n" "$1"; }
warn() { printf "  ${YELLOW}[!]${NC} %s\n" "$1"; }
fail() { printf "  ${RED}[x]${NC} %s\n" "$1"; exit 1; }
info() { printf "  ${CYAN}[-]${NC} %s\n" "$1"; }

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
        printf "    ${CYAN}https://github.com/${GITHUB_REPO}/releases/latest${NC}\n\n"
        exit 0
        ;;
esac

# ─── OS/Arch Detection ───────────────────────────────────

detect_platform() {
    local os arch
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$os" in
        linux)  os="linux" ;;
        darwin) os="darwin" ;;
        *)      fail "Unsupported OS: $os" ;;
    esac

    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             fail "Unsupported architecture: $arch" ;;
    esac

    PLATFORM_OS="$os"
    PLATFORM_ARCH="$arch"
}

# ─── Version Resolution ──────────────────────────────────

resolve_version() {
    if [ -n "$VERSION" ]; then
        return
    fi

    info "Fetching latest version..."
    VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

    if [ -z "$VERSION" ]; then
        fail "Could not determine latest version. Try: --version=v1.50.0"
    fi
}

# ─── Download & Install ──────────────────────────────────

install_binary() {
    local binary_name="cq-${PLATFORM_OS}-${PLATFORM_ARCH}"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${binary_name}"

    info "Downloading cq ${VERSION} (${PLATFORM_OS}/${PLATFORM_ARCH})..."

    if [ "$DRY_RUN" = true ]; then
        ok "Would download: $download_url"
        ok "Would install to: ${INSTALL_DIR}/cq"
        return
    fi

    mkdir -p "$INSTALL_DIR"

    local tmp_file
    tmp_file="$(mktemp)"
    trap "rm -f '$tmp_file'" EXIT

    if ! curl -fSL "$download_url" -o "$tmp_file"; then
        rm -f "$tmp_file"
        fail "Download failed (${VERSION}, ${PLATFORM_OS}/${PLATFORM_ARCH}). Manual: https://github.com/${GITHUB_REPO}/releases/latest"
    fi

    chmod +x "$tmp_file"
    mv "$tmp_file" "${INSTALL_DIR}/cq"
    trap - EXIT

    ok "Binary installed: ${INSTALL_DIR}/cq"
}

# ─── Global Config ────────────────────────────────────────

setup_global_config() {
    local config_dir="$HOME/.c4"
    local config_file="$config_dir/config.yaml"

    mkdir -p "$config_dir"

    if [ "$DRY_RUN" = true ]; then
        ok "Would create: $config_file (cloud: mode: cloud-primary)"
        return
    fi

    if [ -f "$config_file" ]; then
        # Check if cloud section already exists
        if grep -q "^cloud:" "$config_file" 2>/dev/null; then
            ok "Global config exists (cloud section present)"
            return
        fi
        # Append cloud section
        printf "\ncloud:\n  mode: cloud-primary\n" >> "$config_file"
        ok "Global config updated (cloud: mode: cloud-primary)"
    else
        cat > "$config_file" <<'EOF'
# CQ Global Config — applies to all projects
# Override per-project in .c4/config.yaml

cloud:
  mode: cloud-primary
EOF
        ok "Global config created: $config_file"
    fi
}

# ─── PATH Check ───────────────────────────────────────────

check_path() {
    if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        warn "${INSTALL_DIR} is not in PATH. Add to your shell profile:"
        info '  export PATH="$HOME/.local/bin:$PATH"'
    fi
}

# ─── Main ─────────────────────────────────────────────────

printf "\n${BOLD}CQ Installer${NC}\n"
printf "────────────────\n"

detect_platform
ok "Platform: ${PLATFORM_OS}/${PLATFORM_ARCH}"

resolve_version
ok "Version: ${VERSION}"

install_binary
setup_global_config
check_path

# ─── Verification ─────────────────────────────────────────

if [ "$DRY_RUN" = false ]; then
    INSTALLED_VER="$("${INSTALL_DIR}/cq" --version 2>&1 | head -1 || true)"
    if [ -z "$INSTALLED_VER" ]; then
        warn "Binary installed but version check failed"
    else
        ok "Verified: $INSTALLED_VER"
    fi
fi

# ─── Done ─────────────────────────────────────────────────

printf "\n${GREEN}${BOLD}CQ ${VERSION} installed successfully!${NC}\n"
printf "  Binary: ${BOLD}${INSTALL_DIR}/cq${NC}\n"
printf "\n${BOLD}Next steps:${NC}\n"
printf "  1. ${CYAN}cq auth login${NC}    — authenticate with GitHub OAuth\n"
printf "  2. ${CYAN}cq init${NC}          — initialize CQ in your project\n"
printf "  3. Start Claude Code — CQ MCP tools are auto-available\n\n"
printf "${BOLD}For developers:${NC}\n"
printf "  Source build: ${CYAN}cd c4-core && make install${NC}\n\n"
