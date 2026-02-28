#!/bin/bash
# c9-survey.sh: Gemini Google Search grounding으로 arXiv/SOTA 배경조사 수행
#
# Usage:
#   ./scripts/c9-survey.sh "VQ-VAE codebook collapse"
#   ./scripts/c9-survey.sh "Human Mesh Recovery MPJPE benchmark"
#
# Output: 구조화된 survey 결과 (논문 테이블 + SOTA + C9 Conference 입력용 컨텍스트)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PERSONA_FILE="$SCRIPT_DIR/c9-survey-persona.md"

if [ ! -f "$PERSONA_FILE" ]; then
    echo "Error: $PERSONA_FILE not found" >&2
    exit 1
fi

TOPIC="${1:-}"
if [ -z "$TOPIC" ]; then
    echo "Usage: $0 \"research topic\"" >&2
    exit 1
fi

TODAY=$(date +%Y-%m-%d)

PROMPT="$(cat "$PERSONA_FILE")

---
## Survey Request
Topic: ${TOPIC}
Date: ${TODAY}

---
Use Google Search to find the most relevant recent papers (2023-2025 preferred) on this topic.
Follow the Output Format exactly. Return real papers with real arXiv links."

echo "$PROMPT" | gemini \
    -p "Research Librarian로서 Google Search를 사용해 실제 논문을 찾아 구조화된 survey를 작성하라." \
    -o text \
    --approval-mode yolo \
    2>/dev/null | grep -v "^YOLO\|^Loaded\|^Session\|^Error during\|^MCP\|^Loaded cached"
