#!/usr/bin/env bash
# gpu-infer.sh — GPU 추론 실행
# C5 워커 프로토콜:
#   입력: C5_PARAMS env (JSON) → {script: string, args: string}
#   출력: C5_RESULT_FILE 경로에 {exit_code: int, output: string} JSON 저장

set -uo pipefail

RESULT_FILE="${C5_RESULT_FILE:-}"
PARAMS="${C5_PARAMS:-{}}"

# C5_PARAMS에서 script, args 파싱
SCRIPT=$(python3 -c "import sys,json; d=json.loads(sys.argv[1]); print(d.get('script',''))" "$PARAMS" 2>/dev/null || true)
ARGS=$(python3 -c "import sys,json; d=json.loads(sys.argv[1]); print(d.get('args',''))" "$PARAMS" 2>/dev/null || true)

if [ -z "$SCRIPT" ]; then
    OUTPUT="Error: 'script' parameter is required in C5_PARAMS"
    if [ -n "$RESULT_FILE" ]; then
        python3 -c "import json,sys; print(json.dumps({'exit_code':1,'output':sys.argv[1]}))" "$OUTPUT" > "$RESULT_FILE"
    fi
    echo "$OUTPUT" >&2
    exit 1
fi

# 추론 실행 (stdout+stderr 캡처)
set +e
read -ra ARGS_ARR <<< "$ARGS"
OUTPUT=$(python3 "$SCRIPT" "${ARGS_ARR[@]}" 2>&1)
EXIT_CODE=$?
set -e

# 결과를 C5_RESULT_FILE에 저장
if [ -n "$RESULT_FILE" ]; then
    python3 -c "
import json, sys
exit_code = int(sys.argv[1])
output = sys.argv[2]
print(json.dumps({'exit_code': exit_code, 'output': output}))
" "$EXIT_CODE" "$OUTPUT" > "$RESULT_FILE"
fi

echo "$OUTPUT"
exit "$EXIT_CODE"
