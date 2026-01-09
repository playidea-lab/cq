#!/bin/bash
# C4 Global Command Installer
# Creates a 'c4' command that works from anywhere

set -e

C4_DIR=$(cat ~/.c4-install-path 2>/dev/null || echo "$HOME/git/c4")
BIN_DIR="$HOME/.local/bin"

# Create bin directory if needed
mkdir -p "$BIN_DIR"

# Create c4 wrapper script
cat > "$BIN_DIR/c4" << EOF
#!/bin/bash
# C4 CLI Wrapper - runs c4 from anywhere
exec uv run --directory "$C4_DIR" c4 "\$@"
EOF

chmod +x "$BIN_DIR/c4"

echo "✅ 'c4' command installed to $BIN_DIR/c4"
echo ""

# Check if bin dir is in PATH
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    echo "⚠️  $BIN_DIR is not in your PATH"
    echo ""
    echo "Add this line to your ~/.zshrc or ~/.bashrc:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "Then run: source ~/.zshrc"
else
    echo "You can now use 'c4' from anywhere:"
    echo "  c4 init --path /path/to/project"
    echo "  c4 status"
    echo "  c4 run"
fi
