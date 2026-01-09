#!/bin/bash
# =============================================================================
# C4 Quick Installer
# =============================================================================
#
# Usage (public repo / GitHub):
#   curl -fsSL https://raw.githubusercontent.com/ORG/c4/main/get-c4.sh | bash
#
# Usage (private GitLab - use git clone instead):
#   git clone https://git.pilab.co.kr/pi/c4.git ~/.c4 && ~/.c4/install.sh
#
# =============================================================================

set -e

C4_INSTALL_DIR="${C4_INSTALL_DIR:-$HOME/.c4}"
REPO_URL="${C4_REPO_URL:-https://git.pilab.co.kr/pi/c4.git}"

echo "🚀 Installing C4..."
echo ""

# =============================================================================
# Prerequisites
# =============================================================================

# Check uv
if ! command -v uv &> /dev/null; then
    echo "📦 Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.local/bin:$PATH"
fi

# Check git
if ! command -v git &> /dev/null; then
    echo "❌ Error: git is required"
    exit 1
fi

# =============================================================================
# Clone or Update
# =============================================================================

if [[ -d "$C4_INSTALL_DIR/.git" ]]; then
    echo "📥 Updating existing installation..."
    (cd "$C4_INSTALL_DIR" && git pull --quiet)
elif [[ -d "$C4_INSTALL_DIR" ]]; then
    echo "📥 Reinstalling (removing old non-git directory)..."
    rm -rf "$C4_INSTALL_DIR"
    git clone --quiet "$REPO_URL" "$C4_INSTALL_DIR"
else
    echo "📥 Cloning C4..."
    git clone --quiet "$REPO_URL" "$C4_INSTALL_DIR"
fi

# =============================================================================
# Run installer
# =============================================================================

(cd "$C4_INSTALL_DIR" && ./install.sh)

# =============================================================================
# Setup PATH (if needed)
# =============================================================================

BIN_DIR="$HOME/.local/bin"
SHELL_RC=""

# Detect shell config file
if [[ -n "$ZSH_VERSION" ]] || [[ "$SHELL" == */zsh ]]; then
    SHELL_RC="$HOME/.zshrc"
elif [[ -n "$BASH_VERSION" ]] || [[ "$SHELL" == */bash ]]; then
    SHELL_RC="$HOME/.bashrc"
fi

# Add to PATH if not present
if [[ -n "$SHELL_RC" ]] && [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    if ! grep -q '.local/bin' "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo '# C4 CLI' >> "$SHELL_RC"
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$SHELL_RC"
        echo ""
        echo "✅ Added ~/.local/bin to PATH in $SHELL_RC"
        echo ""
        echo "👉 Run this to apply now:"
        echo "   source $SHELL_RC"
    fi
fi

echo ""
echo "🎉 Done! Start using C4:"
echo ""
echo "   c4 init --path /your/project"
echo "   # or in Claude Code: /c4-init"
echo ""
