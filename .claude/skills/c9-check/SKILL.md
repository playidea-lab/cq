# C9 Check

> **Read-Only Notice**: This skill reads `.c9/state.yaml` for display purposes only.
> State writes are managed exclusively by `LoopOrchestrator` (serve component).
> Use `ReadState()` for programmatic access — do not write state.yaml directly.

실험 결과 파싱 + 수렴 판정. C4의 checkpoint에 해당.

**트리거**: "c9-check", "결과 확인", "수렴 확인", "check"

## 실행 순서

### Step 1: 결과 파싱
```bash
./scripts/c9-check.sh [round]
```

`.c9/rounds/rN/results.txt`에서:
- `[C9-DONE]` 마커 → MPJPE, PA-MPJPE, codebook utilization 파싱
- `[C9-BLOCKED]` 마커 → 구현 필요 사항 파싱

### Step 2: 수렴 판정

```
수렴 조건 (AND):
  ① MPJPE 개선 < convergence_threshold_mm (0.5mm) × 2라운드 연속
  ② OR max_rounds 도달

수렴 → phase=FINISH
미수렴 → phase=REFINE, round++
Blocked → phase=CONFERENCE (새 계획 필요)
```

### Step 3: 결과 보고

```
## C9 Check — Round N

실험 결과:
| 실험명 | MPJPE | PA-MPJPE | Util | vs 이전 |
|--------|-------|----------|------|---------|

수렴 판정: [수렴 / 미수렴 / Blocked]
다음 단계: [FINISH / /c9-conference (REFINE)]
```

### Step 4: c9-conference 자동 연계 (미수렴 시)
결과 summary를 컨텍스트로 `/c9-conference` 자동 트리거:
```
"Round N 결과: exp_simvq MPJPE=98.3 개선=4.3mm.
 다음 실험 방향을 토론해줘."
```

## 수렴 기준 변경
```bash
# state.yaml 직접 수정
convergence_threshold_mm: 0.3  # 더 엄격하게
```
