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
    HUB_URL=$(python3 -c "
import yaml, sys
s = yaml.safe_load(open('$STATE_FILE'))
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

SESSION_NAME=$(python3 -c "
import yaml
s = yaml.safe_load(open('$STATE_FILE'))
print(s.get('notify', {}).get('session', ''))
" 2>/dev/null)

echo "[c9-watch] 감시 시작: job=$TRAIN_JOB_ID round=$ROUND exp=$EXP_NAME"
echo "[c9-watch] session=$SESSION_NAME interval=${POLL_INTERVAL}s max=${MAX_WAIT_MIN}min"

poll_job() {
    local job_id="$1"
    curl -s "$HUB_URL/v1/jobs/$job_id" \
        ${API_KEY:+-H "X-API-Key: $API_KEY"} \
        -o /tmp/c9_watch_poll_${job_id}.json 2>/dev/null
    python3 -c "
import json
d = json.load(open('/tmp/c9_watch_poll_$job_id.json'))
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
        SUCCEEDED)
            echo "[c9-watch] 훈련 완료 (SUCCEEDED)"

            # eval job이 있으면 완료 대기
            if [ -n "$EVAL_JOB_ID" ]; then
                echo "[c9-watch] eval job 완료 대기: $EVAL_JOB_ID"
                for i in $(seq 1 60); do
                    EVAL_STATUS=$(poll_job "$EVAL_JOB_ID")
                    echo "[c9-watch] eval status=$EVAL_STATUS"
                    [ "$EVAL_STATUS" = "SUCCEEDED" ] && break
                    [ "$EVAL_STATUS" = "FAILED" ] && break
                    sleep "$POLL_INTERVAL"
                done
            fi

            # metrics.json에서 결과 파싱
            METRICS_JOB=$(curl -s -X POST "$HUB_URL/v1/jobs/submit" \
                ${API_KEY:+-H "X-API-Key: $API_KEY"} \
                -H "Content-Type: application/json" \
                -d "{\"name\":\"c9-read-metrics-r${ROUND}\",\"command\":\"python3 -c \\\"import json; m=json.load(open('/home/pi/git/hmr_unified/experiments/paper1/${EXP_NAME}/metrics.json')); e=json.load(open('/home/pi/git/hmr_unified/experiments/paper1/${EXP_NAME}/eval_results.json')) if __import__('os').path.exists('/home/pi/git/hmr_unified/experiments/paper1/${EXP_NAME}/eval_results.json') else {}; print('MPJPE=' + str(e.get('mpjpe', '?')) + ' PA=' + str(e.get('pa_mpjpe', '?')) + ' loss=' + str(m.get('best_val_loss','?')))\\\"\"}" \
                -o /tmp/c9_metrics_job.json 2>/dev/null)
            METRICS_JID=$(python3 -c "import json; print(json.load(open('/tmp/c9_metrics_job.json')).get('job_id',''))" 2>/dev/null)

            sleep 20
            MPJPE="?"
            PA="?"
            if [ -n "$METRICS_JID" ]; then
                curl -s "$HUB_URL/v1/jobs/$METRICS_JID/logs" \
                    ${API_KEY:+-H "X-API-Key: $API_KEY"} \
                    -o /tmp/c9_metrics_out.json 2>/dev/null
                RESULT=$(python3 -c "
import json, re
d = json.load(open('/tmp/c9_metrics_out.json'))
lines = d.get('lines', [])
for l in lines:
    m = re.search(r'MPJPE=([0-9.?]+) PA=([0-9.?]+)', l)
    if m:
        print(m.group(1), m.group(2))
        break
" 2>/dev/null)
                if [ -n "$RESULT" ]; then
                    MPJPE=$(echo "$RESULT" | awk '{print $1}')
                    PA=$(echo "$RESULT" | awk '{print $2}')
                fi
            fi

            echo "[c9-watch] Round=${ROUND} exp=${EXP_NAME} mpjpe=${MPJPE} pa=${PA}"
            "$SCRIPT_DIR/c9-notify.sh" CHECK "훈련 완료 — MPJPE=${MPJPE}mm PA=${PA}mm" "$ROUND" "mpjpe=${MPJPE},pa=${PA}"
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
