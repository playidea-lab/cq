#!/bin/bash
# gemini-headless.sh: Gemini CLI를 비대화형으로 호출하는 래퍼

# 1. Gemini 명령어 존재 여부 확인
if ! command -v gemini &> /dev/null; then
    echo "Error: Gemini CLI is not installed. Please install it to use 'Ultimate Duo' features."
    exit 1
fi

# 2. API Key 확인
if [ -z "$GOOGLE_API_KEY" ] && [ -z "$GEMINI_API_KEY" ]; then
    # cq secret에서 가져오기 시도 (생략 가능, 여기서는 기본 환경변수 체크)
    if ! cq secret get gemini.api_key &> /dev/null; then
        echo "Error: Gemini API Key not found. Run 'cq secret set gemini.api_key'."
        exit 1
    fi
fi

if [ -z "$1" ]; then
    echo "Usage: $0 \"your prompt\" [--image path] [--search]"
    exit 1
fi

PROMPT="$1"
shift

# Gemini CLI 호출 (실제 환경의 gemini 실행 파일 경로 확인 필요)
# 여기서는 표준 gemini 명령어를 비대화형으로 실행한다고 가정
# --non-interactive 플래그가 있다면 사용, 없다면 파이프로 주입
echo "$PROMPT" | gemini "$@"
