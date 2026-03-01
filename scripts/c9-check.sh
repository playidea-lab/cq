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

# [C9-DONE] 마커 파싱
done_pattern = re.compile(r'\[C9-DONE\]\s+(\S+)\s+mpjpe=([\d.]+)\s+pa=([\d.]+)(?:\s+util=([\d.]+))?')
blocked_pattern = re.compile(r'\[C9-BLOCKED\]\s+(.*)')

findings = []
blocked = []

for m in done_pattern.finditer(results):
    findings.append({
        'exp': m.group(1),
        'mpjpe': float(m.group(2)),
        'pa_mpjpe': float(m.group(3)),
        'codebook_util': float(m.group(4)) if m.group(4) else None
    })

for m in blocked_pattern.finditer(results):
    blocked.append(m.group(1).strip())

# 결과 출력
if findings:
    print('=== 실험 결과 ===')
    for f in findings:
        util_str = f' util={f["codebook_util"]:.2f}' if f['codebook_util'] else ''
        print(f'  {f["exp"]}: MPJPE={f["mpjpe"]}mm PA={f["pa_mpjpe"]}mm{util_str}')
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

baseline = history[0]['best_mpjpe'] if history else 999.0
prev_best = history[-1]['best_mpjpe'] if history else baseline

if findings:
    best = min(findings, key=lambda x: x['mpjpe'])
    improvement = prev_best - best['mpjpe']

    history.append({
        'round': round_num,
        'best_mpjpe': best['mpjpe'],
        'pa_mpjpe': best['pa_mpjpe'],
        'best_exp': best['exp'],
        'improvement': round(improvement, 3)
    })

    if use_legacy_key:
        state['mpjpe_history'] = history
    else:
        state['metric_history'] = history

    print(f'\n=== 수렴 판정 ===')
    print(f'  이전 best: {prev_best}mm')
    print(f'  현재 best: {best["mpjpe"]}mm ({best["exp"]})')
    print(f'  개선량: {improvement:.3f}mm (threshold: {threshold}mm)')

    # 2라운드 연속 threshold 미달 체크
    recent = [h for h in history if h.get('improvement') is not None][-2:]
    consecutive_small = len(recent) >= 2 and all(abs(h['improvement']) < threshold for h in recent)

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

yaml.dump(state, open(state_file, 'w'), default_flow_style=False, allow_unicode=True)
print(f'\n[c9-check] state.yaml 업데이트 완료: phase={state["phase"]}')
PYEOF
