# 예시

CQ가 실제로 어떻게 동작하는지 보여주는 실제 시나리오입니다. 각 예시는 사용자와 Claude Code 간의 대화와 CQ가 뒤에서 하는 일을 보여줍니다.

## 예시

| 예시 | 시나리오 | 티어 | 사용된 스킬 |
|------|---------|:----:|-----------|
| [첫 번째 태스크](/ko/examples/first-task) | 처음부터 CSV 요약 스크립트 생성 | `solo` | `/pi` `/c4-run` |
| [빠른 버그 수정](/ko/examples/quick-fix) | 전체 계획 없이 버그 수정 | `solo` | `/c4-quick` |
| [아이디어에서 배포까지](/ko/examples/idea-to-ship) | 막연한 아이디어에서 커밋까지 전자동 | `solo` | `/pi` → 자동 파이프라인 |
| [기능 계획](/ko/examples/feature-planning) | 새 기능을 end-to-end로 빌드 | `solo` | `/c4-plan` `/c4-run` `/c4-finish` |
| [품질 게이트 실전](/ko/examples/team-review) | refine, polish, review 게이트 동작 | `solo` | `/c4-plan` `/c4-run` |
| [분산 실험](/ko/examples/distributed-experiments) | 여러 머신에서 ML 실험 실행 | `full` | `/c4-plan` `/c4-standby` `/c4-status` |

## 패턴

모든 예시는 같은 형태를 따릅니다:

```
자연스럽게 설명합니다
  ↓
CQ 스킬이 활성화됩니다 (트리거 키워드로)
  ↓
워커가 격리된 워크트리에서 작업을 처리합니다
  ↓
DoD가 검증된 결과가 저장소에 남습니다
```

외울 명령이 없습니다. 원하는 것을 설명하기만 하면 됩니다.
