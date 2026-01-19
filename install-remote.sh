#!/bin/bash
# =============================================================================
# C4 Remote Installer (curl | bash style)
# =============================================================================
#
# Usage:
#   curl -fsSL https://git.pilab.co.kr/pi/c4/raw/main/install-remote.sh | bash
#
# What it does:
#   1. Check/Install uv (if missing)
#   2. Clone C4 to ~/.c4
#   3. Run install.sh
#
# =============================================================================

set -e

C4_REPO="https://git.pilab.co.kr/pi/c4.git"
C4_DIR="$HOME/.c4"

echo ""
echo "  ██████╗██╗  ██╗"
echo " ██╔════╝██║  ██║"
echo " ██║     ███████║"
echo " ██║     ╚════██║"
echo " ╚██████╗     ██║"
echo "  ╚═════╝     ╚═╝"
echo ""
echo " AI Project Orchestration"
echo ""

# =============================================================================
# Step 1: Check/Install uv
# =============================================================================
echo "[1/3] Checking uv..."

if command -v uv &> /dev/null; then
    UV_VERSION=$(uv --version 2>/dev/null | head -1)
    echo "   OK: $UV_VERSION"
else
    echo "   Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh

    # Add to current session
    export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"

    if command -v uv &> /dev/null; then
        UV_VERSION=$(uv --version 2>/dev/null | head -1)
        echo "   OK: $UV_VERSION installed"
    else
        echo "   ERROR: uv installation failed"
        echo "   Try manually: curl -LsSf https://astral.sh/uv/install.sh | sh"
        exit 1
    fi
fi

# =============================================================================
# Step 2: Clone or update C4
# =============================================================================
echo "[2/3] Installing C4..."

if [[ -d "$C4_DIR/.git" ]]; then
    echo "   Updating existing installation..."
    (cd "$C4_DIR" && git pull --quiet)
else
    if [[ -d "$C4_DIR" ]]; then
        echo "   Removing old installation..."
        rm -rf "$C4_DIR"
    fi
    echo "   Cloning C4..."
    git clone --quiet "$C4_REPO" "$C4_DIR"
fi

echo "   OK: $C4_DIR"

# =============================================================================
# Step 3: Run installer
# =============================================================================
echo "[3/3] Running installer..."
echo ""

cd "$C4_DIR"
exec ./install.sh
