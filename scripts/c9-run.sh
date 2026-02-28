#!/bin/bash
# c9-run.sh: .c9/experiments/rN_*.yaml 파일들을 C5 Hub에 제출하고 완료를 폴링
#
# Usage:
#   ./scripts/c9-run.sh [round]        # 특정 라운드 실험 제출 (default: state.yaml의 round)
#   ./scripts/c9-run.sh --poll-only    # 제출 없이 기존 job 폴링만
#
# state.yaml의 phase를 RUN으로 변경하고, 완료 시 CHECK로 전환

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
C9_DIR="$PROJECT_DIR/.c9"
STATE_FILE="$C9_DIR/state.yaml"
HUB_URL="https://piqsol-c5.fly.dev"
API_KEY="cq-test-key-2026"
POLL_INTERVAL=15

# 현재 round 읽기
get_state() {
    python3 -c "
import yaml, sys
s = yaml.safe_load(open('$STATE_FILE'))
print(s.get('$1', ''))
" 2>/dev/null || grep "^$1:" "$STATE_FILE" | awk '{print $2}'
}

set_phase() {
    sed -i.bak "s/^phase: .*/phase: $1/" "$STATE_FILE"
    echo "[c9-run] phase → $1"
}

ROUND=${1:-$(get_state round)}
ROUNDS_DIR="$C9_DIR/rounds/r${ROUND}"
mkdir -p "$ROUNDS_DIR"

# ── SUBMIT ─────────────────────────────────────────────────────
if [[ "$1" != "--poll-only" ]]; then
    echo "[c9-run] Round $ROUND: 실험 제출 시작"
    set_phase "RUN"

    JOBS_FILE="$ROUNDS_DIR/jobs.json"
    echo "[]" > "$JOBS_FILE"

    for exp_file in "$C9_DIR/experiments/r${ROUND}_"*.yaml; do
        [[ -f "$exp_file" ]] || continue
        EXP_NAME=$(grep "^name:" "$exp_file" | awk '{print $2}')
        CMD=$(python3 -c "
import yaml
cfg = yaml.safe_load(open('$exp_file'))
cmd = cfg.get('command','').strip()
print(cmd)
")
        echo "[c9-run] 제출: $EXP_NAME"

        RESPONSE=$(curl -s -X POST "$HUB_URL/v1/jobs/submit" \
            -H "X-API-Key: $API_KEY" \
            -H "Content-Type: application/json" \
            --data-binary @- << ENDJSON
{"name": "$EXP_NAME", "command": $(python3 -c "import json; print(json.dumps('$CMD'))"), "tags": ["c9", "r${ROUND}", "$EXP_NAME"]}
ENDJSON
        )

        JOB_ID=$(echo "$RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('job_id','ERROR'))")
        echo "[c9-run] $EXP_NAME → $JOB_ID"

        # jobs.json 업데이트
        python3 -c "
import json
jobs = json.load(open('$JOBS_FILE'))
jobs.append({'name': '$EXP_NAME', 'job_id': '$JOB_ID', 'status': 'QUEUED'})
json.dump(jobs, open('$JOBS_FILE', 'w'), indent=2)
"
    done

    echo "[c9-run] 제출 완료. jobs.json: $JOBS_FILE"
fi

# ── POLL ───────────────────────────────────────────────────────
echo "[c9-run] 완료 폴링 시작 (${POLL_INTERVAL}s 간격)"
JOBS_FILE="$ROUNDS_DIR/jobs.json"

while true; do
    ALL_DONE=true
    RESULTS=""

    python3 -c "
import json, urllib.request, sys

jobs = json.load(open('$JOBS_FILE'))
api_key = '$API_KEY'
hub_url = '$HUB_URL'
all_done = True

for job in jobs:
    jid = job['job_id']
    req = urllib.request.Request(f'{hub_url}/v1/jobs/{jid}',
        headers={'X-API-Key': api_key})
    try:
        resp = json.loads(urllib.request.urlopen(req).read())
        status = resp.get('status', 'UNKNOWN')
        job['status'] = status
        if status not in ('DONE', 'FAILED', 'CANCELLED'):
            all_done = False
        print(f'  {job[\"name\"]}: {status}')
    except Exception as e:
        print(f'  {job[\"name\"]}: ERROR ({e})')
        all_done = False

json.dump(jobs, open('$JOBS_FILE', 'w'), indent=2)
sys.exit(0 if all_done else 1)
"
    POLL_EXIT=$?

    if [[ $POLL_EXIT -eq 0 ]]; then
        echo "[c9-run] 모든 Job 완료"
        break
    fi

    echo "[c9-run] 대기 중... ${POLL_INTERVAL}s"
    sleep $POLL_INTERVAL
done

# 로그 수집
echo "[c9-run] 결과 로그 수집"
python3 -c "
import json, urllib.request

jobs = json.load(open('$JOBS_FILE'))
api_key = '$API_KEY'
hub_url = '$HUB_URL'
results = []

for job in jobs:
    jid = job['job_id']
    req = urllib.request.Request(f'{hub_url}/v1/jobs/{jid}/logs',
        headers={'X-API-Key': api_key})
    try:
        resp = json.loads(urllib.request.urlopen(req).read())
        lines = resp.get('lines', [])
        results.append(f'=== {job[\"name\"]} ({jid}) ===')
        results.extend(lines)
    except Exception as e:
        results.append(f'=== {job[\"name\"]} ERROR: {e} ===')

open('$ROUNDS_DIR/results.txt', 'w').write('\n'.join(results))
print('Results saved to $ROUNDS_DIR/results.txt')
"

set_phase "CHECK"
echo "[c9-run] Done. 다음: ./scripts/c9-check.sh"
