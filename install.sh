#!/bin/bash
# C4 Installation Script
# Usage: ./install.sh

set -e

C4_DIR="$(cd "$(dirname "$0")" && pwd)"
CLAUDE_CONFIG="$HOME/.claude.json"
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

# 3. Add MCP server to global config
echo "🔧 Configuring MCP server..."

if [ ! -f "$CLAUDE_CONFIG" ]; then
    echo "   Creating $CLAUDE_CONFIG..."
    echo '{"mcpServers":{}}' > "$CLAUDE_CONFIG"
fi

# Check if c4 MCP server already configured
if grep -q '"c4"' "$CLAUDE_CONFIG" 2>/dev/null; then
    echo "   ⚠️  C4 MCP server already configured"
else
    # Use Python to safely edit JSON
    python3 << EOF
import json
from pathlib import Path

config_path = Path("$CLAUDE_CONFIG")
config = json.loads(config_path.read_text())

if "mcpServers" not in config:
    config["mcpServers"] = {}

config["mcpServers"]["c4"] = {
    "command": "uv",
    "args": ["--directory", "$C4_DIR", "run", "python", "-m", "c4.mcp_server"]
}

config_path.write_text(json.dumps(config, indent=2))
print("   ✅ Added C4 MCP server to config")
EOF
fi

echo ""
echo "✅ C4 installed successfully!"
echo ""
echo "📌 Next steps:"
echo "   1. Restart Claude Code (close and reopen)"
echo "   2. Go to your project: cd /path/to/your/project"
echo "   3. Run: /c4-init"
echo ""
echo "📚 Available commands:"
echo "   /c4-init      - Initialize C4 in project"
echo "   /c4-plan      - Plan tasks from docs"
echo "   /c4-run       - Start execution"
echo "   /c4-status    - Check status"
echo ""
