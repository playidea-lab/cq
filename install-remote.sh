#!/bin/bash
# C4 Remote Installation Script
# Usage: curl -LsSf https://git.pilab.co.kr/pi/c4/-/raw/main/install-remote.sh | sh
#    or: curl -LsSf https://git.pilab.co.kr/pi/c4/-/raw/main/install-remote.sh | sh -s -- --dir ~/tools/c4

set -e

# Default installation directory
C4_INSTALL_DIR="${C4_DIR:-$HOME/.c4}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dir)
            C4_INSTALL_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

CLAUDE_CONFIG="$HOME/.claude.json"
CLAUDE_COMMANDS="$HOME/.claude/commands"

echo "🚀 Installing C4..."
echo "   Install directory: $C4_INSTALL_DIR"

# Check for uv
if ! command -v uv &> /dev/null; then
    echo "📦 Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.local/bin:$PATH"
fi

# Clone or update repository
if [ -d "$C4_INSTALL_DIR" ]; then
    echo "📥 Updating existing installation..."
    cd "$C4_INSTALL_DIR"
    git pull
else
    echo "📥 Cloning C4..."
    git clone https://git.pilab.co.kr/pi/c4.git "$C4_INSTALL_DIR"
    cd "$C4_INSTALL_DIR"
fi

# Install dependencies
echo "📦 Installing dependencies..."
uv sync

# Copy slash commands
echo "📋 Copying slash commands..."
mkdir -p "$CLAUDE_COMMANDS"
cp "$C4_INSTALL_DIR/.claude/commands/c4-"*.md "$CLAUDE_COMMANDS/"

# Configure MCP server
echo "🔧 Configuring MCP server..."

if [ ! -f "$CLAUDE_CONFIG" ]; then
    echo '{}' > "$CLAUDE_CONFIG"
fi

# Use Python to safely edit JSON
python3 << EOF
import json
from pathlib import Path

config_path = Path("$CLAUDE_CONFIG")
try:
    config = json.loads(config_path.read_text())
except:
    config = {}

if "mcpServers" not in config:
    config["mcpServers"] = {}

config["mcpServers"]["c4"] = {
    "command": "uv",
    "args": ["--directory", "$C4_INSTALL_DIR", "run", "python", "-m", "c4.mcp_server"]
}

config_path.write_text(json.dumps(config, indent=2))
EOF

echo ""
echo "✅ C4 installed successfully!"
echo ""
echo "📌 Next steps:"
echo "   1. Restart Claude Code"
echo "   2. cd /path/to/your/project"
echo "   3. /c4-init"
echo ""
