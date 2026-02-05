#!/bin/bash
# =============================================================================
# C4 Installer
# =============================================================================
#
# Usage:
#   git clone https://github.com/USER/c4.git && cd c4 && ./install.sh
#
# What it does:
#   1. Install dependencies (uv sync)
#   2. Save C4 install path (~/.c4-install-path)
#   3. Create global 'c4' command (~/.local/bin/c4)
#   4. Install Claude Code slash commands (~/.claude/commands/)
#   5. Install Cursor commands & MCP (~/.cursor/)
#   6. Install security & stop hooks (~/.claude/hooks/)
#   7. Register hooks in Claude settings
#
# =============================================================================

set -e

echo "C4 Installer"
echo "============"
echo ""

# =============================================================================
# Step 0: Check & Install Git (required)
# =============================================================================
echo "[0/8] Checking Git..."

if command -v git &> /dev/null; then
    GIT_VERSION=$(git --version | cut -d' ' -f3)
    echo "   OK: Git $GIT_VERSION found"
else
    echo "   Git not found. Attempting to install..."
    echo ""
    
    # Detect OS and install Git
    install_git() {
        if [[ "$OSTYPE" == "darwin"* ]]; then
            # macOS
            if command -v brew &> /dev/null; then
                echo "   Installing Git via Homebrew..."
                brew install git
            else
                echo "   ERROR: Git is required but Homebrew is not installed."
                echo ""
                echo "   Install options:"
                echo "     1. Install Xcode Command Line Tools:"
                echo "        xcode-select --install"
                echo "     2. Or install Homebrew first:"
                echo "        /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
                echo "        brew install git"
                return 1
            fi
        elif [[ -f /etc/debian_version ]]; then
            # Debian/Ubuntu
            echo "   Installing Git via apt..."
            sudo apt-get update && sudo apt-get install -y git
        elif [[ -f /etc/redhat-release ]]; then
            # RHEL/CentOS/Fedora
            echo "   Installing Git via dnf/yum..."
            if command -v dnf &> /dev/null; then
                sudo dnf install -y git
            else
                sudo yum install -y git
            fi
        elif [[ -f /etc/arch-release ]]; then
            # Arch Linux
            echo "   Installing Git via pacman..."
            sudo pacman -S --noconfirm git
        elif [[ -f /etc/alpine-release ]]; then
            # Alpine
            echo "   Installing Git via apk..."
            sudo apk add git
        else
            echo "   ERROR: Unknown OS. Please install Git manually:"
            echo ""
            echo "   macOS:   brew install git"
            echo "   Ubuntu:  sudo apt install git"
            echo "   Fedora:  sudo dnf install git"
            echo "   Arch:    sudo pacman -S git"
            return 1
        fi
    }
    
    if install_git; then
        if command -v git &> /dev/null; then
            GIT_VERSION=$(git --version | cut -d' ' -f3)
            echo "   OK: Git $GIT_VERSION installed successfully"
        else
            echo "   ERROR: Git installation failed."
            exit 1
        fi
    else
        echo ""
        echo "   Git is required for C4 to manage code versions."
        exit 1
    fi
fi

echo ""

# Determine C4 directory
C4_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Validate
if [[ ! -f "$C4_DIR/c4/cli.py" ]]; then
    echo "❌ Error: Not a valid C4 installation"
    exit 1
fi

echo "📁 C4 directory: $C4_DIR"
echo ""

# =============================================================================
# Step 1: Install dependencies
# =============================================================================
echo "[1/8] Installing dependencies..."
if command -v uv &> /dev/null; then
    (cd "$C4_DIR" && uv sync --quiet)
    echo "   ✅ Dependencies installed"
else
    echo "❌ Error: uv not found. Install it first:"
    echo "   curl -LsSf https://astral.sh/uv/install.sh | sh"
    exit 1
fi

# =============================================================================
# Step 2: Save install path & detect platforms
# =============================================================================
echo "[2/8] Saving install path & detecting platforms..."
echo "$C4_DIR" > ~/.c4-install-path
echo "   → ~/.c4-install-path"

# Detect installed platforms
DETECTED_PLATFORMS=""
DEFAULT_PLATFORM=""

# Check Claude Code
if command -v claude &> /dev/null || [[ -d "$HOME/.claude" ]]; then
    DETECTED_PLATFORMS="claude"
    DEFAULT_PLATFORM="claude"
    echo "   ✓ Claude Code detected"
fi

# Check Cursor
if command -v cursor &> /dev/null || [[ -d "$HOME/.cursor" ]]; then
    if [[ -n "$DETECTED_PLATFORMS" ]]; then
        DETECTED_PLATFORMS="$DETECTED_PLATFORMS, cursor"
    else
        DETECTED_PLATFORMS="cursor"
        DEFAULT_PLATFORM="cursor"
    fi
    echo "   ✓ Cursor detected"
fi

# Set default platform in global config
mkdir -p "$HOME/.c4"
if [[ -n "$DEFAULT_PLATFORM" ]]; then
    cat > "$HOME/.c4/config.yaml" << EOF
# C4 Global Configuration
# Auto-generated by install.sh

platform: $DEFAULT_PLATFORM
EOF
    echo "   → Default platform: $DEFAULT_PLATFORM"
else
    echo ""
    echo "   ⚠️  WARNING: No supported IDE detected!"
    echo "   Supported: Claude Code, Cursor"
    echo "   C4 will be installed, but you need one of these IDEs to use slash commands."
    echo ""
fi

# =============================================================================
# Step 3: Create global 'c4' command
# =============================================================================
echo "[3/8] Creating global 'c4' command..."

BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR"

cat > "$BIN_DIR/c4" << EOF
#!/bin/bash
# C4 CLI - Auto-generated by install.sh
# Pass current directory to c4 (uv --directory changes cwd)
exec uv run --directory "$C4_DIR" c4 --path "\$(pwd)" "\$@"
EOF

chmod +x "$BIN_DIR/c4"
echo "   → $BIN_DIR/c4"

# =============================================================================
# Step 4: Install /c4-* slash commands & rules globally (Claude Code)
# =============================================================================
echo "[4/8] Installing Claude Code slash commands & rules..."

CLAUDE_COMMANDS="$HOME/.claude/commands"
CLAUDE_RULES="$HOME/.claude/rules"
mkdir -p "$CLAUDE_COMMANDS"
mkdir -p "$CLAUDE_RULES"

# 4a. Slash commands
count=0
for cmd in "$C4_DIR/.claude/commands"/c4-*.md; do
    if [[ -f "$cmd" ]]; then
        cp "$cmd" "$CLAUDE_COMMANDS/"
        echo "   → $(basename "$cmd")"
        ((count++))
    fi
done
echo "   ✅ $count commands installed"

# 4b. C4 rules for AI agents
if [[ -f "$C4_DIR/CLAUDE.md" ]]; then
    cp "$C4_DIR/CLAUDE.md" "$CLAUDE_RULES/c4.md"
    echo "   ✅ C4 rules installed: ~/.claude/rules/c4.md"
fi

# =============================================================================
# Step 5: Install Cursor commands, rules & MCP
# =============================================================================
echo "[5/8] Installing Cursor commands, rules & MCP..."

# 5a. Cursor slash commands (global)
CURSOR_COMMANDS="$HOME/.cursor/commands"
CURSOR_RULES="$HOME/.cursor/rules"
mkdir -p "$CURSOR_COMMANDS"
mkdir -p "$CURSOR_RULES"

cursor_count=0
for cmd in "$C4_DIR/.cursor/commands"/c4-*.md; do
    if [[ -f "$cmd" ]]; then
        cp "$cmd" "$CURSOR_COMMANDS/"
        ((cursor_count++))
    fi
done
echo "   → $cursor_count Cursor commands installed"

# 5b. C4 rules for AI agents (Cursor)
if [[ -f "$C4_DIR/CLAUDE.md" ]]; then
    cp "$C4_DIR/CLAUDE.md" "$CURSOR_RULES/c4.md"
    echo "   ✅ C4 rules installed: ~/.cursor/rules/c4.md"
fi

# =============================================================================
# Step 6: Install Gemini CLI slash commands & rules
# =============================================================================
echo "[6/8] Installing Gemini CLI slash commands & rules..."

GEMINI_COMMANDS="$HOME/.gemini/commands"
mkdir -p "$GEMINI_COMMANDS"

gemini_count=0
for cmd in "$C4_DIR/.gemini/commands"/c4-*.md; do
    if [[ -f "$cmd" ]]; then
        cp "$cmd" "$GEMINI_COMMANDS/"
        ((gemini_count++))
    fi
done
echo "   → $gemini_count Gemini commands installed"

# =============================================================================
# Step 7: Install Claude Code hooks
# =============================================================================
echo "[7/8] Installing Claude Code hooks..."

HOOKS_DIR="$HOME/.claude/hooks"
mkdir -p "$HOOKS_DIR"

# Stop hook (inline)
cat > "$HOOKS_DIR/stop.sh" << 'HOOK'
#!/bin/bash
# Check for C4 project (SQLite or legacy JSON)
if [[ ! -f ".c4/c4.db" ]] && [[ ! -f ".c4/state.json" ]]; then exit 0; fi
result=$(python3 ~/.claude/hooks/c4-stop-hook.py 2>/dev/null)
if [[ $? -eq 2 ]]; then
    echo "{\"decision\":\"block\",\"reason\":\"$result\",\"instructions\":\"Continue with /c4-run\"}"
    exit 2
fi
exit 0
HOOK
chmod +x "$HOOKS_DIR/stop.sh"
echo "   → stop.sh"

# Copy template scripts
for script in c4-stop-hook.py c4-bash-security-hook.sh; do
    if [[ -f "$C4_DIR/templates/$script" ]]; then
        cp "$C4_DIR/templates/$script" "$HOOKS_DIR/"
        chmod +x "$HOOKS_DIR/$script"
        echo "   → $script"
    fi
done

# =============================================================================
# Step 8: Register hooks in settings
# =============================================================================
echo "[8/8] Registering hooks..."

python3 << 'PYTHON'
import json
from pathlib import Path

settings_path = Path.home() / ".claude" / "settings.json"
settings_path.parent.mkdir(parents=True, exist_ok=True)

settings = json.loads(settings_path.read_text()) if settings_path.exists() else {}

if "hooks" not in settings:
    settings["hooks"] = {}
if "PreToolUse" not in settings["hooks"]:
    settings["hooks"]["PreToolUse"] = []

bash_hook = {
    "matcher": "Bash",
    "hooks": [{"type": "command", "command": "~/.claude/hooks/c4-bash-security-hook.sh"}]
}

settings["hooks"]["PreToolUse"] = [
    h for h in settings["hooks"]["PreToolUse"] if h.get("matcher") != "Bash"
]
settings["hooks"]["PreToolUse"].append(bash_hook)

settings_path.write_text(json.dumps(settings, indent=2))
PYTHON
echo "   → ~/.claude/settings.json"

# =============================================================================
# Done!
# =============================================================================
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ C4 installed successfully!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📌 Usage:"
echo ""
echo "   Terminal:"
echo "     c4 init --path /path/to/project"
echo "     c4 status"
echo ""
echo "   Claude Code / Cursor / Gemini CLI:"
echo "     /c4-init      # Initialize project"
echo "     /c4-plan      # Parse docs & create tasks"
echo "     /c4-run       # Start execution"
echo "     /c4-status    # Check status"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  🚀 시작하기                                                 ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║                                                              ║"
echo "║  1️⃣  프로젝트 초기화:                                        ║"
echo "║     cd your-project && c4 init                               ║"
echo "║                                                              ║"
echo "║  2️⃣  워크플로우:                                             ║"
echo "║     /c4-plan   → 계획 수립 (Discovery/Design/Plan)           ║"
echo "║     /c4-run    → 실행 시작 (Worker 스폰)                     ║"
echo "║     /c4-status → 진행 상황 확인                              ║"
echo "║     /c4-stop   → 실행 중지                                   ║"
echo "║                                                              ║"
echo "║  3️⃣  빠른 시작 (추천):                                       ║"
echo "║     /c4-quick \"작업 설명\"                                   ║"
echo "║     → 태스크 생성 + 즉시 할당 + 자동 검증                    ║"
echo "║                                                              ║"
echo "║  4️⃣  Economic Mode (76% 비용 절감):                         ║"
echo "║     .c4/config.yaml에서 설정:                                ║"
echo "║     economic_mode:                                           ║"
echo "║       preset: economic                                       ║"
echo "║                                                              ║"
echo "║  5️⃣  도움말:                                                 ║"
echo "║     c4 --help                                                ║"
echo "║     docs/ROADMAP.md (프로젝트 로드맵)                        ║"
echo "║                                                              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Auto-setup PATH if needed
if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    SHELL_RC=""
    if [[ -n "$ZSH_VERSION" ]] || [[ "$SHELL" == */zsh ]]; then
        SHELL_RC="$HOME/.zshrc"
    elif [[ -n "$BASH_VERSION" ]] || [[ "$SHELL" == */bash ]]; then
        SHELL_RC="$HOME/.bashrc"
    fi

    if [[ -n "$SHELL_RC" ]] && ! grep -q '.local/bin' "$SHELL_RC" 2>/dev/null; then
        echo "" >> "$SHELL_RC"
        echo '# C4 CLI' >> "$SHELL_RC"
        echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$SHELL_RC"
        echo ""
        echo "✅ Added ~/.local/bin to PATH in $SHELL_RC"
        echo "   Run: source $SHELL_RC"
    fi
fi

echo ""
echo "🔄 Restart Claude Code / Cursor / Gemini CLI to activate slash commands"
echo ""
