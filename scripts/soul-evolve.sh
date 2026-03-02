#!/bin/bash
# soul-evolve.sh: LLM 기반 사용자 페르소나 진화 (SOUL_LLM=auto|claude|gemini|openai)

USER_NAME="changmin"
SOUL_DIR=".c4/souls/$USER_NAME"
RAW_PATTERNS_FILE="$SOUL_DIR/raw_patterns.json"
SOUL_FILE="$SOUL_DIR/soul-developer.md"

if [ ! -f "$RAW_PATTERNS_FILE" ]; then
    echo "❌ 분석할 패턴 파일이 없습니다."
    exit 0
fi

echo "🧪 LLM 페르소나 분석 엔진을 가동합니다... (provider: ${SOUL_LLM:-auto})"

# 1. 원본 소울 파일 로드 (없으면 기본값)
if [ -f "$SOUL_FILE" ]; then
    CURRENT_SOUL=$(cat "$SOUL_FILE")
else
    CURRENT_SOUL="사용자의 초기 개발 페르소나입니다."
fi

# 2. 누적된 패턴 로드
PATTERNS=$(cat "$RAW_PATTERNS_FILE")

# 3. LLM에게 분석 및 합성 요청
PROMPT="
당신은 'Persona Expert'입니다. 다음 제공된 사용자의 코드 수정 패턴들을 분석하여, 기존 페르소나(Soul)를 더 구체적이고 진화된 형태로 업데이트하세요.

[기존 페르소나]
$CURRENT_SOUL

[최근 12건의 수정 패턴 (JSON)]
$PATTERNS

[요구사항]
1. 위 패턴들에서 사용자의 코딩 스타일, 선호하는 라이브러리, 에러 처리 방식, 네이밍 규칙 등의 일관된 철학을 찾아내세요.
2. 기존 페르소나에 이 새로운 철학을 자연스럽게 융합하여 마크다운 형태로 반환하세요.
3. 지침은 에이전트가 읽었을 때 바로 행동으로 옮길 수 있는 명령형(Imperative)으로 작성하세요.
4. 출력은 오직 업데이트된 전체 마크다운 내용만 출력하세요 (기타 설명 제외).
"

# LLM 호출 (SOUL_LLM 환경변수로 provider 선택, 기본: auto)
./scripts/llm-call.sh "$PROMPT" > /tmp/new_soul.md

# 4. 검증 및 덮어쓰기
if [ -s /tmp/new_soul.md ]; then
    cat /tmp/new_soul.md > "$SOUL_FILE"
    echo "✨ 소울 진화 완료: $SOUL_FILE 이 업데이트되었습니다."
    
    # 5. 원본 패턴 파일 비우기 (학습 완료)
    echo "[]" > "$RAW_PATTERNS_FILE"
else
    echo "❌ 소울 진화 실패: LLM 응답이 비어 있습니다."
fi

rm -f /tmp/new_soul.md
