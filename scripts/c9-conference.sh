#!/bin/bash
# c9-conference.sh: Claude 포지션을 받아 Gemini 반론을 한 턴 가져오는 헬퍼
#
# Usage:
#   echo "컨텍스트" | ./scripts/c9-conference.sh "Claude의 이번 턴 주장"
#
# Output: Gemini의 conference 포맷 응답 (POSITION/CONCEDE/OPEN/CONSENSUS)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PERSONA_FILE="$SCRIPT_DIR/c9-conference-persona.md"

if [ ! -f "$PERSONA_FILE" ]; then
    echo "Error: $PERSONA_FILE not found" >&2
    exit 1
fi

CLAUDE_TURN="${1:-}"
if [ -z "$CLAUDE_TURN" ]; then
    echo "Usage: $0 \"Claude의 주장\"" >&2
    exit 1
fi

# stdin 컨텍스트
if [ -t 0 ]; then
    CONTEXT=""
else
    CONTEXT=$(cat)
fi

PROMPT="$(cat "$PERSONA_FILE")"

if [ -n "$CONTEXT" ]; then
    PROMPT="${PROMPT}

---
## 실험/연구 컨텍스트
${CONTEXT}"
fi

PROMPT="${PROMPT}

---
## Claude의 이번 턴
${CLAUDE_TURN}

---
conference rules에 따라 응답하라. POSITION/CONCEDE/OPEN/CONSENSUS 형식을 반드시 포함하라."

echo "$PROMPT" | gemini \
    -p "C9 Conference 참가자로서 conference rules에 따라 응답하라." \
    -o text \
    --approval-mode yolo \
    2>/dev/null | grep -v "^YOLO\|^Loaded\|^Session\|^Error during\|^MCP\|^Loaded cached"
