#!/bin/bash
# c9-check.sh: 실험 결과 파싱 + 수렴 판정
#
# Usage:
#   ./scripts/c9-check.sh [round]
#
# - rounds/rN/results.txt에서 MPJPE 파싱
# - state.yaml mpjpe_history 업데이트
# - 수렴 기준 충족 → phase=FINISH
# - 미충족 → phase=REFINE (새 conference 필요)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
C9_DIR="$PROJECT_DIR/.c9"
STATE_FILE="$C9_DIR/state.yaml"

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

# MPJPE 파싱 (C9-DONE 마커에서)
python3 << PYEOF
import re, yaml, sys, os

results = open('$RESULTS_FILE').read()
state_file = '$STATE_FILE'
round_num = $ROUND

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
state = yaml.safe_load(open(state_file))
history = state.get('mpjpe_history', [])
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
    state['mpjpe_history'] = history

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
