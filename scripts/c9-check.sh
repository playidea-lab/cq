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
    HUB_URL=$(python3 -c "
import yaml, sys
s = yaml.safe_load(open('$STATE_FILE'))
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

ROUND=${1:-$(python3 -c "
import yaml
print(yaml.safe_load(open('$STATE_FILE')).get('round', 1))
" 2>/dev/null)}

RESULTS_FILE="$C9_DIR/rounds/r${ROUND}/results.txt"

if [[ ! -f "$RESULTS_FILE" ]]; then
    echo "[c9-check] Error: $RESULTS_FILE not found. c9-run.sh를 먼저 실행하세요."
    exit 1
fi

echo "[c9-check] Round $ROUND 결과 분석"
echo ""

# [C9-DONE] 파서: state.yaml metric.name 기반 범용화 (MPJPE 고정 제거)
python3 << PYEOF
import re, yaml, sys, os

results = open('$RESULTS_FILE').read()
state_file = '$STATE_FILE'
round_num = $ROUND

state = yaml.safe_load(open(state_file))

# metric 설정 읽기 (범용화)
metric_cfg = state.get('metric', {})
metric_name = metric_cfg.get('name', 'mpjpe') if isinstance(metric_cfg, dict) else 'mpjpe'
metric_unit = metric_cfg.get('unit', '') if isinstance(metric_cfg, dict) else ''
lower_is_better = metric_cfg.get('lower_is_better', True) if isinstance(metric_cfg, dict) else True

# [C9-DONE] 마커 파싱 — metric.name 기반 범용화
# 형식: [C9-DONE] exp_name {metric_name}=X.X [secondary=X.X] [util=X.X]
import re as _re
done_pattern = _re.compile(
    rf'\[C9-DONE\]\s+(\S+)\s+{_re.escape(metric_name)}=([\d.]+)'
    r'(?:\s+\S+=([\d.]+))?(?:\s+util=([\d.]+))?'
)
blocked_pattern = re.compile(r'\[C9-BLOCKED\]\s+(.*)')

findings = []
blocked = []

for m in done_pattern.finditer(results):
    findings.append({
        'exp': m.group(1),
        'mpjpe': float(m.group(2)),        # primary metric value
        'pa_mpjpe': float(m.group(3)) if m.group(3) else None,
        'codebook_util': float(m.group(4)) if m.group(4) else None
    })

for m in blocked_pattern.finditer(results):
    blocked.append(m.group(1).strip())

# 결과 출력
if findings:
    print('=== 실험 결과 ===')
    for f in findings:
        util_str = f' util={f["codebook_util"]:.2f}' if f['codebook_util'] else ''
        pa_str = f' PA={f["pa_mpjpe"]}{metric_unit}' if f['pa_mpjpe'] is not None else ''
        print(f'  {f["exp"]}: {metric_name}={f["mpjpe"]}{metric_unit}{pa_str}{util_str}')
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
        best = min(findings, key=lambda x: x['mpjpe'])
    else:
        best = max(findings, key=lambda x: x['mpjpe'])
    improvement = prev_best - best['mpjpe']

    history.append({
        'round': round_num,
        'value': best['mpjpe'],          # 신규 schema 키 (metric_history[].value)
        'pa_value': best['pa_mpjpe'],    # 선택적 secondary metric
        'best_exp': best['exp'],
        'improvement': round(improvement, 3)
    })

    if use_legacy_key:
        state['mpjpe_history'] = history
    else:
        state['metric_history'] = history

    direction = '↓' if lower_is_better else '↑'
    print(f'\n=== 수렴 판정 ===')
    print(f'  이전 best: {prev_best}{metric_unit}')
    print(f'  현재 best: {best["mpjpe"]}{metric_unit} ({best["exp"]}) {direction}')
    print(f'  개선량: {improvement:.3f}{metric_unit} (threshold: {threshold}{metric_unit})')

    # 2라운드 연속 threshold 미달 체크
    recent = [h for h in history if h.get('improvement') is not None][-2:]
    consecutive_small = len(recent) >= 2 and all(abs(h.get('improvement', 0.0)) < threshold for h in recent)

    if consecutive_small:
        state['phase'] = 'FINISH'
        print(f'  → 수렴! phase=FINISH (2라운드 연속 개선 < {threshold}mm)')
    elif improvement < threshold and round_num >= state.get('max_rounds', 10):
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
PYEOF
