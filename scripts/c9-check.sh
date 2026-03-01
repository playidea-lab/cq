#!/bin/bash
# c9-check.sh: 실험 결과 파싱 + 수렴 판정
#
# Usage:
#   ./scripts/c9-check.sh [round]
#
# - rounds/rN/results.txt에서 [C9-DONE] 마커 파싱 (metric.name 기반 범용화)
# - state.yaml metric_history 업데이트 (fallback: mpjpe_history 하위호환)
# - 수렴 기준 충족 → phase=FINISH
# - 미충족 → phase=REFINE (새 conference 필요)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
C9_DIR="$PROJECT_DIR/.c9"
STATE_FILE="$C9_DIR/state.yaml"

# ── HUB_URL 로드 (C9_HUB_URL env → state.yaml hub.url) ────────
# (결과 재수집 등 향후 hub 접근 시 사용)
if [[ -n "${C9_HUB_URL:-}" ]]; then
    HUB_URL="$C9_HUB_URL"
else
    HUB_URL=$(STATE_FILE="$STATE_FILE" uv run python -c "
import yaml, sys, os
s = yaml.safe_load(open(os.environ['STATE_FILE']))
hub = s.get('hub', {})
url = hub.get('url', '') if isinstance(hub, dict) else ''
print(url)
" 2>/dev/null)
fi

# ── API Key 로드 (cq secret get c9.hub.api_key → C9_API_KEY env → 경고) ──
API_KEY=""
if command -v cq &>/dev/null; then
    API_KEY=$(cq secret get c9.hub.api_key 2>/dev/null | tr -d '\n\r')
fi
if [[ -z "$API_KEY" && -n "${C9_API_KEY:-}" ]]; then
    API_KEY="$C9_API_KEY"
fi

ROUND=${1:-$(STATE_FILE="$STATE_FILE" uv run python -c "
import yaml, os
print(yaml.safe_load(open(os.environ['STATE_FILE'])).get('round', 1))
" 2>/dev/null)}
if [[ -z "$ROUND" || ! "$ROUND" =~ ^[0-9]+$ ]]; then
    echo "[c9-check] Error: ROUND 읽기 실패 (state.yaml 파싱 오류 또는 round 키 없음)" >&2
    exit 1
fi

RESULTS_FILE="$C9_DIR/rounds/r${ROUND}/results.txt"

if [[ ! -f "$RESULTS_FILE" ]]; then
    echo "[c9-check] Error: $RESULTS_FILE not found. c9-run.sh를 먼저 실행하세요."
    exit 1
fi

echo "[c9-check] Round $ROUND 결과 분석"
echo ""

# [C9-DONE] 파서: state.yaml metric.name 기반 범용화 (MPJPE 고정 제거)
RESULTS_FILE="$RESULTS_FILE" STATE_FILE="$STATE_FILE" ROUND_NUM="$ROUND" \
    C9_API_KEY_ENV="$API_KEY" SCRIPT_DIR_ENV="$SCRIPT_DIR" uv run python << 'PYEOF'
import re, yaml, sys, os

results = open(os.environ['RESULTS_FILE']).read()
state_file = os.environ['STATE_FILE']
round_num = int(os.environ['ROUND_NUM'])

state = yaml.safe_load(open(state_file))

# metric 설정 읽기 (범용화)
metric_cfg = state.get('metric', {})
metric_name = metric_cfg.get('name', 'mpjpe') if isinstance(metric_cfg, dict) else 'mpjpe'
metric_unit = (metric_cfg.get('unit') or '') if isinstance(metric_cfg, dict) else ''
lower_is_better = metric_cfg.get('lower_is_better', True) if isinstance(metric_cfg, dict) else True

# [C9-DONE] 마커 파싱 — metric.name 기반 범용화
# 형식: [C9-DONE] exp_name {metric_name}=X.X [secondary=X.X] [util=X.X]
done_pattern = re.compile(
    rf'\[C9-DONE\]\s+(\S+)\s+{re.escape(metric_name)}=([\d.]+)'
    r'(?:\s+\S+=([\d.]+))?(?:\s+util=([\d.]+))?'
)
blocked_pattern = re.compile(r'\[C9-BLOCKED\]\s+(.*)')

findings = []
blocked = []

for m in done_pattern.finditer(results):
    findings.append({
        'exp': m.group(1),
        'primary_value': float(m.group(2)),        # primary metric value
        'pa_value': float(m.group(3)) if m.group(3) else None,
        'codebook_util': float(m.group(4)) if m.group(4) else None
    })

for m in blocked_pattern.finditer(results):
    blocked.append(m.group(1).strip())

# 결과 출력
if findings:
    print('=== 실험 결과 ===')
    for f in findings:
        util_str = f' util={f["codebook_util"]:.2f}' if f['codebook_util'] else ''
        pa_str = f' PA={f["pa_value"]}{metric_unit}' if f['pa_value'] is not None else ''
        print(f'  {f["exp"]}: {metric_name}={f["primary_value"]}{metric_unit}{pa_str}{util_str}')
else:
    print('=== C9-DONE 마커 없음 (실험 미완료 또는 blocked) ===')

if blocked:
    print('=== Blocked ===')
    for b in blocked:
        print(f'  {b}')

# state.yaml 업데이트
# metric_history 우선, fallback: mpjpe_history (하위호환)
history = state.get('metric_history', None)
if history is None:
    history = state.get('mpjpe_history', [])
    use_legacy_key = True
else:
    use_legacy_key = False

# convergence_threshold: metric.convergence_threshold 우선, fallback: convergence_threshold_mm
threshold = None
if isinstance(metric_cfg, dict):
    threshold = metric_cfg.get('convergence_threshold')
if threshold is None:
    threshold = state.get('convergence_threshold_mm', 0.5)

def _get_val(entry):
    """value 키 우선, best_mpjpe fallback (하위호환)"""
    v = entry.get('value')
    return v if v is not None else entry.get('best_mpjpe', 999.0)

baseline = _get_val(history[0]) if history else 999.0
prev_best = _get_val(history[-1]) if history else baseline

if findings:
    if lower_is_better:
        best = min(findings, key=lambda x: x['primary_value'])
    else:
        best = max(findings, key=lambda x: x['primary_value'])
    # lower_is_better=True: improvement>0이면 개선 / False: best가 크면 개선
    improvement = (prev_best - best['primary_value']) if lower_is_better else (best['primary_value'] - prev_best)

    new_entry = {
        'round': round_num,
        'value': best['primary_value'],  # 신규 schema 키 (metric_history[].value)
        'pa_value': best['pa_value'],    # 선택적 secondary metric
        'best_exp': best['exp'],
        'improvement': round(improvement, 3)
    }
    # 동일 라운드 재실행 시 중복 기록 방지 (수렴 판정 오류 방지)
    existing_idx = next((i for i, h in enumerate(history) if h.get('round') == round_num), None)
    if existing_idx is not None:
        history[existing_idx] = new_entry
    else:
        history.append(new_entry)

    if use_legacy_key:
        state['mpjpe_history'] = history
    else:
        state['metric_history'] = history

    direction = '↓' if lower_is_better else '↑'
    print(f'\n=== 수렴 판정 ===')
    print(f'  이전 best: {prev_best}{metric_unit}')
    print(f'  현재 best: {best["primary_value"]}{metric_unit} ({best["exp"]}) {direction}')
    print(f'  개선량: {improvement:.3f}{metric_unit} (threshold: {threshold}{metric_unit})')

    # 2라운드 연속 threshold 미달 체크
    recent = [h for h in history if h.get('improvement') is not None][-2:]
    consecutive_small = len(recent) >= 2 and all(abs(h.get('improvement', 0.0)) < threshold for h in recent)

    if consecutive_small:
        state['phase'] = 'FINISH'
        print(f'  → 수렴! phase=FINISH (2라운드 연속 개선 < {threshold}{metric_unit})')
    elif abs(improvement) < threshold and round_num >= state.get('max_rounds', 10):
        state['phase'] = 'FINISH'
        print(f'  → 최대 라운드 도달. phase=FINISH')
    elif blocked:
        state['phase'] = 'CONFERENCE'
        state['round'] = round_num + 1
        print(f'  → Blocked 실험 있음. phase=CONFERENCE (새 계획 필요)')
    else:
        state['phase'] = 'REFINE'
        state['round'] = round_num + 1
        print(f'  → 미수렴. phase=REFINE → /c9-conference로 다음 실험 설계')
else:
    # 결과 없으면 (blocked 등) conference로
    state['phase'] = 'CONFERENCE'
    state['round'] = round_num + 1
    print('  → 실험 결과 없음. phase=CONFERENCE')

import tempfile, os as _os
tmp = tempfile.NamedTemporaryFile(
    mode='w', dir=_os.path.dirname(state_file) or '.',
    delete=False, suffix='.tmp'
)
yaml.dump(state, tmp, default_flow_style=False, allow_unicode=True)
tmp.close()
_os.replace(tmp.name, state_file)
print(f'\n[c9-check] state.yaml 업데이트 완료: phase={state["phase"]}')

# G12-B: state.yaml 갱신 후 Research State API에도 동기화 (non-fatal)
hub_url = ''
hub_cfg = state.get('hub', {})
if isinstance(hub_cfg, dict):
    hub_url = hub_cfg.get('url', '')
if hub_url:
    import subprocess, sys as _sys
    api_key = _os.environ.get('C9_API_KEY_ENV', '')
    _round = state.get('round', round_num)
    _version = state.get('version', 0)
    _scripts_dir = _os.environ.get('SCRIPT_DIR_ENV', _os.path.dirname(state_file))
    _script = _os.path.join(_scripts_dir, 'c9-state-api.py')
    try:
        _proc = subprocess.run(
            ['uv', 'run', 'python', _script, 'set',
             hub_url, api_key, str(_round), state['phase'], str(_version)],
            capture_output=True, text=True, timeout=15
        )
        if _proc.returncode == 0:
            print(f'[c9-check] Research State API 동기화 완료: phase={state["phase"]}')
        elif _proc.returncode == 2:
            print(f'[c9-check] Warning: Research State API 409 충돌 (state.yaml은 갱신됨)', file=_sys.stderr)
        else:
            print(f'[c9-check] Warning: Research State API 동기화 실패 (state.yaml은 갱신됨)', file=_sys.stderr)
            if _proc.stderr:
                print(_proc.stderr.strip(), file=_sys.stderr)
    except Exception as _e:
        print(f'[c9-check] Warning: Research State API 동기화 오류: {_e} (state.yaml은 갱신됨)', file=_sys.stderr)
PYEOF
