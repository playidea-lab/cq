#!/usr/bin/env bash
# gpu-train.sh — GPU 학습 실행
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

# 경로 트래버설 방지: .. 포함 또는 절대경로는 거부
if [[ "$SCRIPT" =~ \.\. ]] || [[ "$SCRIPT" = /* ]]; then
    echo "Error: script path traversal or absolute path not allowed: $SCRIPT" >&2
    if [ -n "$RESULT_FILE" ]; then
        python3 -c "import json,sys; print(json.dumps({'exit_code':1,'output':sys.argv[1]}))" "path not allowed: $SCRIPT" > "$RESULT_FILE"
    fi
    exit 1
fi

# 학습 실행 — 대용량 output 대비 temp 파일로 스트리밍 (OOM 방지)
LOGFILE=$(mktemp)
trap 'rm -f "$LOGFILE"' EXIT
set +e
read -ra ARGS_ARR <<< "$ARGS"
python3 "$SCRIPT" "${ARGS_ARR[@]}" 2>&1 | tee "$LOGFILE"
EXIT_CODE=${PIPESTATUS[0]}
set -e

# 결과를 C5_RESULT_FILE에 저장 (마지막 64KB만 — 완전한 로그는 LOGFILE 참조)
if [ -n "$RESULT_FILE" ]; then
    TAIL_OUTPUT=$(tail -c 65536 "$LOGFILE")
    python3 -c "
import json, sys
exit_code = int(sys.argv[1])
output = sys.argv[2]
print(json.dumps({'exit_code': exit_code, 'output': output}))
" "$EXIT_CODE" "$TAIL_OUTPUT" > "$RESULT_FILE"
fi

exit "$EXIT_CODE"
