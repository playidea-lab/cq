#!/bin/bash
# gemini-debate.sh: Gemini를 Research Scientist 토론 상대로 헤드리스 호출
#
# Usage:
#   ./scripts/gemini-debate.sh "Claude의 주장 또는 토론 질문"
#   echo "실험 컨텍스트" | ./scripts/gemini-debate.sh "Claude의 주장"
#
# Output: Gemini의 반론 (stderr 노이즈 제거, stdout만 출력)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PERSONA_FILE="$SCRIPT_DIR/debate-scientist-persona.md"

if [ ! -f "$PERSONA_FILE" ]; then
    echo "Error: persona file not found at $PERSONA_FILE" >&2
    exit 1
fi

CLAUDE_POSITION="${1:-}"
if [ -z "$CLAUDE_POSITION" ]; then
    echo "Usage: $0 \"Claude의 주장 또는 질문\"" >&2
    echo "       echo \"컨텍스트\" | $0 \"주장\"" >&2
    exit 1
fi

# stdin에 데이터가 있으면 컨텍스트로, 없으면 빈 문자열
if [ -t 0 ]; then
    CONTEXT=""
else
    CONTEXT=$(cat)
fi

# 프롬프트 조립: 페르소나 + 컨텍스트 + Claude 주장
FULL_PROMPT="$(cat "$PERSONA_FILE")"

if [ -n "$CONTEXT" ]; then
    FULL_PROMPT="${FULL_PROMPT}

---
## 실험 컨텍스트
${CONTEXT}"
fi

FULL_PROMPT="${FULL_PROMPT}

---
## Claude의 주장/질문
${CLAUDE_POSITION}

---
위 debate rules에 따라 응답하라."

# Gemini 헤드리스 호출 (stderr 노이즈 제거)
echo "$FULL_PROMPT" | gemini \
    -p "Research Scientist로서 debate rules에 따라 응답하라. 반드시 'Q:'로 끝내라." \
    -o text \
    --approval-mode yolo \
    2>/dev/null | grep -v "^YOLO\|^Loaded\|^Session\|^Error during\|^MCP\|^Loaded cached"
