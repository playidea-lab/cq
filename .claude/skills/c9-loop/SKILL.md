---
name: c9-loop
description: |
  C9 연구 루프 시작 + 상태 조회. LoopOrchestrator(Go)가 자율 실행.
  트리거: "c9-loop", "루프 시작", "다음 단계", "연구 루프", "loop start"
allowed-tools: mcp__cq__*
---

# C9 Loop — 연구 루프 시작 + 모니터링

> LoopOrchestrator (Go serve component)가 자율 실행.
> 이 스킬은 시작 트리거 + 상태 조회 UI.

## 시작

```python
c4_research_loop_start(
    hypothesis="가설 텍스트",     # 또는 hypothesis_id="hyp-xxx"
    max_iterations=10,            # budget gate
    max_patience=3,               # early stopping
    convergence_threshold=0.5     # metric 개선 임계값
)
```

## 상태 조회

```python
c4_research_loop_status(hypothesis_id="hyp-xxx")
# → {hypothesis_id, status, round, phase, convergence: {patience_count, best_metric, converged}}
```

## 중지

```python
c4_research_loop_stop(hypothesis_id="hyp-xxx")
```

## 자율 루프 흐름

```
시작 → RUN(실험) → CHECK(수렴?) → 미수렴 → Debate → 다음 실험 or CONFERENCE
                                 → 수렴 → FINISH
```

CONFERENCE/IMPLEMENT 필요 시 → Hub reasoning 잡 → standby agent가 처리.
수동 개입: `/c9-steer`
