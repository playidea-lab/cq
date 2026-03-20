---
name: c9-steer
description: |
  C9 연구 루프 조종. c4_research_intervene MCP 도구 래핑.
  트리거: "c9-steer", "/c9-steer", "방향 바꿔", "phase 전환", "스티어",
  "steer loop", "change direction"
allowed-tools: mcp__cq__*
---

# C9 Steer — 연구 루프 조종

> LoopOrchestrator가 state.yaml 단일 writer.
> 이 스킬은 `c4_research_intervene` MCP 도구를 래핑합니다.

## 사용법

| 명령 | MCP 호출 |
|------|---------|
| `/c9-steer "방향"` | `c4_research_intervene(action="steering", guidance="방향")` |
| `/c9-steer --status` | `c4_research_loop_status(hypothesis_id=...)` |
| `/c9-steer --skip` | `c4_research_intervene(action="phase_override", phase="다음")` |
| `/c9-steer --back` | `c4_research_intervene(action="phase_override", phase="이전")` |
| `/c9-steer --continue` | `c4_research_intervene(action="continue")` — gate 즉시 해제 |
| `/c9-steer --reset` | `c4_research_intervene(action="reset")` — FINISH→CONFERENCE |

## Phase 전이 참조

```
CONFERENCE → IMPLEMENT → RUN → CHECK → REFINE → CONFERENCE
                                    └→ FINISH
```

## 실행

1. `c4_research_loop_status`로 현재 상태 확인
2. active_jobs가 있으면 경고 표시
3. 해당 MCP 도구 호출
4. 결과 보고 (이전/현재 phase, steer_reason)
