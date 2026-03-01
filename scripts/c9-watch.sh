#!/bin/bash
# c9-watch.sh: C5 Hub job 완료를 폴링하고 결과를 cq mail로 전송
#
# 로그 파일 감시 방식 대신 C5 Hub job status API 직접 폴링.
# [C9-DONE] 마커나 "Error" 문자열에 의존하지 않음.
#
# Usage:
#   ./scripts/c9-watch.sh <job_id> <round> <exp_name> [eval_job_id]
#
# Examples:
#   ./scripts/c9-watch.sh j-123 1 exp_simvq
#   ./scripts/c9-watch.sh j-123 1 exp_simvq j-456   # eval job도 함께 감시

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
STATE_FILE="$PROJECT_DIR/.c9/state.yaml"

# ── HUB_URL 로드 (C9_HUB_URL env → state.yaml hub.url) ────────
if [[ -n "${C9_HUB_URL:-}" ]]; then
    HUB_URL="$C9_HUB_URL"
else
    HUB_URL=$(STATE_FILE="$STATE_FILE" python3 -c "
import yaml, sys, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
hub = s.get('hub', {})
url = hub.get('url', '') if isinstance(hub, dict) else ''
if not url:
    sys.exit(1)
print(url)
" 2>/dev/null) || {
        echo "[c9-watch] Error: HUB_URL 미설정. C9_HUB_URL 환경변수 또는 state.yaml hub.url을 설정하세요." >&2
        exit 1
    }
fi

# ── API Key 로드 (cq secret → C9_API_KEY env → 경고 후 진행) ──
API_KEY=""
if command -v cq &>/dev/null; then
    API_KEY=$(cq secret get c9.hub.api_key 2>/dev/null | tr -d '\n\r')
fi
if [[ -z "$API_KEY" && -n "${C9_API_KEY:-}" ]]; then
    API_KEY="$C9_API_KEY"
fi
if [[ -z "$API_KEY" ]]; then
    echo "[c9-watch] Warning: API key 미설정 — 인증 없이 진행합니다. (.env 파일 커밋 금지)" >&2
fi

POLL_INTERVAL=30
MAX_WAIT_MIN=180

TRAIN_JOB_ID="${1:?Usage: $0 <job_id> <round> <exp_name>}"
ROUND="${2:?Usage: $0 <job_id> <round> <exp_name>}"
EXP_NAME="${3:?Usage: $0 <job_id> <round> <exp_name>}"
EVAL_JOB_ID="${4:-}"

SESSION_NAME=$(STATE_FILE="$STATE_FILE" python3 -c "
import yaml, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
print(s.get('notify', {}).get('session', ''))
" 2>/dev/null)

echo "[c9-watch] 감시 시작: job=$TRAIN_JOB_ID round=$ROUND exp=$EXP_NAME"
echo "[c9-watch] session=$SESSION_NAME interval=${POLL_INTERVAL}s max=${MAX_WAIT_MIN}min"

poll_job() {
    local job_id="$1"
    # job_id 형식 검증 (C5 Hub 형식: [a-zA-Z0-9_-]+)
    if ! [[ "$job_id" =~ ^[a-zA-Z0-9_-]+$ ]]; then
        echo "ERROR"
        return 1
    fi
    local poll_tmp
    poll_tmp=$(mktemp "/tmp/c9_watch_poll_${job_id}_XXXXXX.json")
    trap 'rm -f "$poll_tmp"' RETURN
    curl_args=(-s --max-time 10)
    [[ -n "$API_KEY" ]] && curl_args+=(-H "X-API-Key: $API_KEY")
    curl "${curl_args[@]}" "$HUB_URL/v1/jobs/$job_id" \
        -o "$poll_tmp" 2>/dev/null
    C9_POLL_TMP="$poll_tmp" python3 -c "
import json, os
d = json.load(open(os.environ['C9_POLL_TMP']))
print(d.get('status', 'UNKNOWN'))
" 2>/dev/null || echo "ERROR"
}

# 최대 대기 횟수 계산
MAX_POLLS=$(( MAX_WAIT_MIN * 60 / POLL_INTERVAL ))
count=0

while [ $count -lt $MAX_POLLS ]; do
    count=$(( count + 1 ))
    elapsed=$(( count * POLL_INTERVAL / 60 ))

    STATUS=$(poll_job "$TRAIN_JOB_ID")
    echo "[c9-watch] min=${elapsed} job=${TRAIN_JOB_ID} status=${STATUS}"

    case "$STATUS" in
        SUCCEEDED|DONE)
            echo "[c9-watch] 훈련 완료 (${STATUS})"

            # eval job이 있으면 완료 대기
            if [ -n "$EVAL_JOB_ID" ]; then
                echo "[c9-watch] eval job 완료 대기: $EVAL_JOB_ID"
                EVAL_STATUS="UNKNOWN"
                MAX_EVAL_POLLS=$(( MAX_WAIT_MIN * 60 / POLL_INTERVAL / 2 ))
                for i in $(seq 1 "$MAX_EVAL_POLLS"); do
                    EVAL_STATUS=$(poll_job "$EVAL_JOB_ID")
                    echo "[c9-watch] eval status=$EVAL_STATUS"
                    [ "$EVAL_STATUS" = "SUCCEEDED" ] && break
                    [ "$EVAL_STATUS" = "DONE" ] && break
                    [ "$EVAL_STATUS" = "FAILED" ] && break
                    [ "$EVAL_STATUS" = "CANCELLED" ] && break
                    sleep "$POLL_INTERVAL"
                done
                if [ "$EVAL_STATUS" != "SUCCEEDED" ] && [ "$EVAL_STATUS" != "DONE" ]; then
                    echo "[c9-watch] eval job 미완료/실패: $EVAL_JOB_ID ($EVAL_STATUS)"
                    "$SCRIPT_DIR/c9-notify.sh" BLOCKED "eval 미완료/실패 — job $EVAL_JOB_ID ($EVAL_STATUS)" "$ROUND"
                    exit 1
                fi
            fi

            # Job 로그에서 [C9-DONE] 마커 파싱 (metric.name 기반 범용화)
            METRIC_NAME=$(STATE_FILE="$STATE_FILE" python3 -c "
import yaml, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
m = s.get('metric', {})
print(m.get('name', 'value') if isinstance(m, dict) else 'value')
" 2>/dev/null || echo "value")
            METRIC_UNIT=$(STATE_FILE="$STATE_FILE" python3 -c "
import yaml, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
m = s.get('metric', {})
print(m.get('unit', '') if isinstance(m, dict) else '')
" 2>/dev/null || echo "")

            TARGET_JOB_ID="${EVAL_JOB_ID:-$TRAIN_JOB_ID}"
            curl_log_args=(-s)
            [[ -n "$API_KEY" ]] && curl_log_args+=(-H "X-API-Key: $API_KEY")
            RESULT=$(curl "${curl_log_args[@]}" "$HUB_URL/v1/jobs/$TARGET_JOB_ID/logs" | python3 -c "
import json, re, sys
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(1)
lines = d.get('lines', [])
for l in lines:
    m = re.search(r'\[C9-DONE\]\s+\S+\s+(\S+)=([\d.]+)', l)
    if m:
        print(m.group(1), m.group(2))
        break
" 2>/dev/null)

            METRIC_KEY="?"
            METRIC_VAL="?"
            if [ -n "$RESULT" ]; then
                METRIC_KEY=$(echo "$RESULT" | awk '{print $1}')
                METRIC_VAL=$(echo "$RESULT" | awk '{print $2}')
            fi

            echo "[c9-watch] Round=${ROUND} exp=${EXP_NAME} ${METRIC_NAME}=${METRIC_VAL}${METRIC_UNIT}"
            "$SCRIPT_DIR/c9-notify.sh" CHECK "훈련 완료 — ${METRIC_NAME}=${METRIC_VAL}${METRIC_UNIT}" "$ROUND" "${METRIC_NAME}=${METRIC_VAL}"
            exit 0
            ;;

        FAILED|CANCELLED)
            echo "[c9-watch] 훈련 실패: $STATUS"
            "$SCRIPT_DIR/c9-notify.sh" BLOCKED "훈련 실패 — job $TRAIN_JOB_ID ($STATUS)" "$ROUND"
            exit 1
            ;;

        QUEUED|RUNNING)
            sleep "$POLL_INTERVAL"
            ;;

        *)
            echo "[c9-watch] 알 수 없는 상태: $STATUS"
            sleep "$POLL_INTERVAL"
            ;;
    esac
done

echo "[c9-watch] 타임아웃 (${MAX_WAIT_MIN}분)"
"$SCRIPT_DIR/c9-notify.sh" BLOCKED "타임아웃 ${MAX_WAIT_MIN}min — ${EXP_NAME}" "$ROUND"
exit 1
