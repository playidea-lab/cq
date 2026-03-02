#!/bin/bash
# llm-call.sh: 추상화된 LLM 호출 래퍼
#
# 환경변수:
#   SOUL_LLM=auto|claude|gemini|openai  (기본: auto)
#   LLM_MODEL=<model-id>                (기본: 각 provider 기본값)
#
# 사용법: ./scripts/llm-call.sh "prompt text"
#
# 우선순위 (auto): claude → gemini → openai
# 키 조회 순서:   환경변수 → cq secret get

set -euo pipefail

PROVIDER="${SOUL_LLM:-auto}"
PROMPT="${1:-}"

if [ -z "$PROMPT" ]; then
    echo "Error: prompt required" >&2
    exit 1
fi

_get_key() { cq secret get "$1" 2>/dev/null || true; }

# 특수문자 포함 프롬프트를 JSON 문자열로 안전하게 직렬화
_json_str() {
    printf '%s' "$1" | python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))"
}

_call_claude() {
    local key="${ANTHROPIC_API_KEY:-$(_get_key anthropic.api_key)}"
    [ -z "$key" ] && return 1
    local model="${LLM_MODEL:-claude-sonnet-4-6}"
    curl -sf https://api.anthropic.com/v1/messages \
        -H "x-api-key: $key" \
        -H "anthropic-version: 2023-06-01" \
        -H "content-type: application/json" \
        -d "{\"model\":\"$model\",\"max_tokens\":4096,\"messages\":[{\"role\":\"user\",\"content\":$(_json_str "$PROMPT")}]}" \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['content'][0]['text'])"
}

_call_gemini() {
    # gemini CLI가 있으면 우선 사용 (기존 gemini-headless.sh 호환)
    if command -v gemini &>/dev/null; then
        echo "$PROMPT" | gemini
        return
    fi
    local key="${GEMINI_API_KEY:-${GOOGLE_API_KEY:-$(_get_key gemini.api_key)}}"
    [ -z "$key" ] && return 1
    local model="${LLM_MODEL:-gemini-2.0-flash}"
    curl -sf "https://generativelanguage.googleapis.com/v1beta/models/${model}:generateContent?key=$key" \
        -H "content-type: application/json" \
        -d "{\"contents\":[{\"parts\":[{\"text\":$(_json_str "$PROMPT")}]}]}" \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['candidates'][0]['content']['parts'][0]['text'])"
}

_call_openai() {
    local key="${OPENAI_API_KEY:-$(_get_key openai.api_key)}"
    [ -z "$key" ] && return 1
    local model="${LLM_MODEL:-gpt-4o-mini}"
    curl -sf https://api.openai.com/v1/chat/completions \
        -H "Authorization: Bearer $key" \
        -H "content-type: application/json" \
        -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":$(_json_str "$PROMPT")}]}" \
    | python3 -c "import json,sys; print(json.load(sys.stdin)['choices'][0]['message']['content'])"
}

case "$PROVIDER" in
    claude|anthropic)
        _call_claude || { echo "Error: Claude API 호출 실패 (anthropic.api_key 확인)" >&2; exit 1; }
        ;;
    gemini|google)
        _call_gemini || { echo "Error: Gemini 호출 실패 (gemini.api_key 확인)" >&2; exit 1; }
        ;;
    openai)
        _call_openai || { echo "Error: OpenAI API 호출 실패 (openai.api_key 확인)" >&2; exit 1; }
        ;;
    auto)
        _call_claude 2>/dev/null || \
        _call_gemini 2>/dev/null || \
        _call_openai 2>/dev/null || \
        { echo "Error: 사용 가능한 LLM 없음. SOUL_LLM=<provider> 또는 cq secret set <provider>.api_key" >&2; exit 1; }
        ;;
    *)
        echo "Error: SOUL_LLM='$PROVIDER' 미지원. auto|claude|gemini|openai 중 선택." >&2
        exit 1
        ;;
esac
