#!/bin/bash
# C4 Installation Script
# Usage: ./install.sh

set -e

C4_DIR="$(cd "$(dirname "$0")" && pwd)"
CLAUDE_COMMANDS="$HOME/.claude/commands"

echo "🚀 Installing C4..."

# 1. Install dependencies
echo "📦 Installing dependencies..."
if command -v uv &> /dev/null; then
    uv sync
else
    echo "❌ Error: uv not found. Please install uv first:"
    echo "   curl -LsSf https://astral.sh/uv/install.sh | sh"
    exit 1
fi

# 2. Copy slash commands
echo "📋 Copying slash commands to ~/.claude/commands/..."
mkdir -p "$CLAUDE_COMMANDS"
cp "$C4_DIR/.claude/commands/c4-"*.md "$CLAUDE_COMMANDS/"
echo "   ✅ Copied $(ls "$C4_DIR/.claude/commands/c4-"*.md | wc -l | tr -d ' ') commands"

# 3. Store install path for /c4-init
echo "📝 Saving install path..."
echo "$C4_DIR" > "$HOME/.c4-install-path"
echo "   ✅ Saved to ~/.c4-install-path"

echo ""
echo "✅ C4 installed successfully!"
echo ""
echo "📌 Next steps:"
echo "   1. Restart Claude Code (close and reopen)"
echo "   2. Go to your project: cd /path/to/your/project"
echo "   3. Run: /c4-init"
echo ""
echo "💡 Note: /c4-init will automatically configure the MCP server for each project."
echo ""
echo "📚 Available commands:"
echo "   /c4-init      - Initialize C4 in project"
echo "   /c4-plan      - Plan tasks from docs"
echo "   /c4-run       - Start execution"
echo "   /c4-status    - Check status"
echo "   /c4-clear     - Reset C4 state (dev)"
echo ""
