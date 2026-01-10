#!/bin/bash
# =============================================================================
# C4 Remote Installation Script
# =============================================================================
#
# Usage:
#   curl -LsSf https://git.pilab.co.kr/pi/c4/-/raw/main/install-remote.sh | sh
#
# Options:
#   --dir <path>   설치 경로 지정 (기본: ~/.c4)
#   --update       기존 설치 업데이트만
#
# Examples:
#   # 기본 설치 (~/.c4)
#   curl -LsSf https://git.pilab.co.kr/pi/c4/-/raw/main/install-remote.sh | sh
#
#   # 경로 지정 설치
#   curl -LsSf ... | sh -s -- --dir ~/tools/c4
#
#   # 업데이트만
#   curl -LsSf ... | sh -s -- --update
#
# =============================================================================

set -e

# Default installation directory
C4_INSTALL_DIR="${C4_DIR:-$HOME/.c4}"
UPDATE_ONLY=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dir)
            C4_INSTALL_DIR="$2"
            shift 2
            ;;
        --update)
            UPDATE_ONLY=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: curl ... | sh -s -- [--dir <path>] [--update]"
            exit 1
            ;;
    esac
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 C4 Remote Installer"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📁 Install directory: $C4_INSTALL_DIR"
echo ""

# =============================================================================
# Step 1: Check/Install uv
# =============================================================================
if ! command -v uv &> /dev/null; then
    echo "📦 uv not found. Installing..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.local/bin:$PATH"
    echo "   ✅ uv installed"
else
    echo "✅ uv found"
fi

# =============================================================================
# Step 2: Clone or Update repository
# =============================================================================
if [ -d "$C4_INSTALL_DIR" ]; then
    if [ "$UPDATE_ONLY" = true ]; then
        echo "📥 Updating existing installation..."
        cd "$C4_INSTALL_DIR"
        git pull
        echo "   ✅ Updated"
    else
        echo "📥 Reinstalling (removing old installation)..."
        rm -rf "$C4_INSTALL_DIR"
        git clone https://git.pilab.co.kr/pi/c4.git "$C4_INSTALL_DIR"
        echo "   ✅ Cloned"
    fi
else
    echo "📥 Cloning C4..."
    git clone https://git.pilab.co.kr/pi/c4.git "$C4_INSTALL_DIR"
    echo "   ✅ Cloned"
fi

# =============================================================================
# Step 3: Run full install.sh
# =============================================================================
echo ""
echo "🔧 Running install.sh..."
echo ""

cd "$C4_INSTALL_DIR"
bash ./install.sh

# install.sh already prints completion message
