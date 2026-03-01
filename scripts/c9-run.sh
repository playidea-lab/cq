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
POLL_INTERVAL=15

# ── Pre-conditions ─────────────────────────────────────────────
if ! command -v python3 &>/dev/null; then
    echo "[c9-run] Error: python3가 설치되어 있지 않습니다." >&2
    exit 1
fi
if ! python3 -c "import yaml" &>/dev/null; then
    echo "[c9-run] Error: python3 yaml 모듈이 없습니다. uv add pyyaml 또는 pip install pyyaml" >&2
    exit 1
fi

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
        echo "[c9-run] Error: HUB_URL 미설정. C9_HUB_URL 환경변수 또는 state.yaml hub.url을 설정하세요." >&2
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
    echo "[c9-run] Warning: API key 미설정 — 인증 없이 진행합니다. (.env 파일 커밋 금지)" >&2
fi

# 현재 round 읽기
get_state() {
    python3 -c "
import yaml, sys
s = yaml.safe_load(open('$STATE_FILE'))
print(s.get('$1', ''))
" 2>/dev/null || grep "^$1:" "$STATE_FILE" | awk '{print $2}'
}

set_phase() {
    # docs/c9-state-schema.md 권장: NamedTemporaryFile → yaml.dump → os.replace (원자 저장)
    python3 -c "
import yaml, tempfile, os
state_file = '$STATE_FILE'
s = yaml.safe_load(open(state_file))
s['phase'] = '$1'
tmp = tempfile.NamedTemporaryFile(mode='w', dir=os.path.dirname(state_file) or '.', delete=False, suffix='.tmp')
yaml.dump(s, tmp, default_flow_style=False, allow_unicode=True)
tmp.close()
os.replace(tmp.name, state_file)
"
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

        PAYLOAD=$(python3 -c "
import yaml, json
cfg = yaml.safe_load(open('$exp_file'))
cmd = cfg.get('command', '').strip()
exp_name = cfg.get('name', '$EXP_NAME')
print(json.dumps({'name': exp_name, 'command': cmd, 'tags': ['c9', 'r${ROUND}', exp_name]}))
")
        RESPONSE=$(echo "$PAYLOAD" | curl -s -X POST "$HUB_URL/v1/jobs/submit" \
            ${API_KEY:+-H "X-API-Key: $API_KEY"} \
            -H "Content-Type: application/json" \
            -d @-)

        JOB_ID=$(echo "$RESPONSE" | python3 -c "import json,sys; print(json.load(sys.stdin).get('job_id','ERROR'))")
        echo "[c9-run] $EXP_NAME → $JOB_ID"

        # jobs.json 업데이트 (원자 저장 — partial write 방지)
        python3 -c "
import json, tempfile, os
jobs_file = '$JOBS_FILE'
jobs = json.load(open(jobs_file))
jobs.append({'name': '$EXP_NAME', 'job_id': '$JOB_ID', 'status': 'QUEUED'})
tmp = tempfile.NamedTemporaryFile(mode='w', dir=os.path.dirname(jobs_file) or '.', delete=False, suffix='.tmp')
json.dump(jobs, tmp, indent=2)
tmp.close()
os.replace(tmp.name, jobs_file)
"
    done

    echo "[c9-run] 제출 완료. jobs.json: $JOBS_FILE"
fi

# ── POLL ───────────────────────────────────────────────────────
echo "[c9-run] 완료 폴링 시작 (${POLL_INTERVAL}s 간격)"
JOBS_FILE="$ROUNDS_DIR/jobs.json"

while true; do
    python3 -c "
import json, urllib.request, sys

jobs = json.load(open('$JOBS_FILE'))
api_key = '$API_KEY'
hub_url = '$HUB_URL'
all_done = True

for job in jobs:
    jid = job['job_id']
    headers = {'X-API-Key': api_key} if api_key else {}
    req = urllib.request.Request(f'{hub_url}/v1/jobs/{jid}', headers=headers)
    try:
        resp = json.loads(urllib.request.urlopen(req).read())
        status = resp.get('status', 'UNKNOWN')
        job['status'] = status
        # C5 Hub는 SUCCEEDED 또는 DONE 둘 다 완료 상태로 사용 가능
        if status not in ('DONE', 'SUCCEEDED', 'FAILED', 'CANCELLED'):
            all_done = False
        print(f'  {job[\"name\"]}: {status}')
    except Exception as e:
        print(f'  {job[\"name\"]}: ERROR ({e})')
        all_done = False

# 폴링 결과 원자 저장
import tempfile as _tf, os as _os
_tmp = _tf.NamedTemporaryFile(mode='w', dir=_os.path.dirname('$JOBS_FILE') or '.', delete=False, suffix='.tmp')
json.dump(jobs, _tmp, indent=2)
_tmp.close()
_os.replace(_tmp.name, '$JOBS_FILE')
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
    headers = {'X-API-Key': api_key} if api_key else {}
    req = urllib.request.Request(f'{hub_url}/v1/jobs/{jid}/logs', headers=headers)
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
