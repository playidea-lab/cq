# 워크플로우 개요

CQ는 connected 상태에서 **클라우드 우선 모드**로 동작합니다: 태스크, 지식, LLM 호출이 CQ 클라우드(Supabase SSOT)를 통해 처리됩니다. 실행은 엣지(로컬)에서, 두뇌는 클라우드에 있습니다. `solo` 티어는 모든 것이 로컬에서 실행됩니다.

CQ는 **모든 전환에 게이트가 있는** 구조화된 루프를 따릅니다:

```
/pi → PLAN → EXECUTE → COMPLETE
       ↑        ↑         ↑
    refine    polish    review
     gate      gate      gate
```

## 명령어

| 명령 | 단계 | 동작 |
|------|------|------|
| [`/pi`](/ko/workflow/plan#pi) | Ideation | 토론, 조사, 논쟁. `idea.md` 생성 후 `/c4-plan`으로 자동 전환. |
| [`/c4-plan`](/ko/workflow/plan) | Plan | Discovery → Design → 태스크 분해. **Refine 게이트**: batch 4개+ 시 critique 루프 필수. |
| [`/c4-run`](/ko/workflow/run) | Execute | 병렬 워커 스폰. **Polish 게이트**: 각 워커가 자체 리뷰 후 제출. |
| [`/c4-finish`](/ko/workflow/finish) | Complete | 빌드 → 테스트 → 설치 → 커밋. Polish 루프 내장. |
| `/c4-status` | 언제든지 | 진행 상황, 의존성 그래프, 워커 상태 확인. |
| `/c4-quick` | — | 소규모 단일 태스크, 계획 생략. |

## 태스크 라이프사이클

```
pending → in_progress → polish → submit → review → done
                                            ↘ request_changes → T-XXX-1 (수정)
```

- **T-001-0** — 구현 (버전 0)
- **R-001-0** — 자동 생성 6축 리뷰
- **T-001-1** — 리뷰에서 변경 요청 시 수정 태스크 (최대 3회)
- **CP-001** — 단계 경계 체크포인트

## 품질 게이트

| 게이트 | 트리거 | 강제 방식 |
|--------|--------|----------|
| **Refine** | `c4_add_todo`에서 10분 내 4개+ 태스크 | `c4_record_gate("refine", "done")` 없으면 Go가 거부 |
| **Polish** | `c4_submit`에서 diff ≥ 5줄 | `c4_record_gate("polish", "done")` 없으면 Go가 거부 |
| **Review** | 모든 T- 태스크 완료 시 | R- 태스크 자동 생성, 6축 평가 |
| **Max revision** | 3회 거절 사이클 | Go가 추가 수정 차단 |

이것은 **권장이 아닙니다** — 바이너리에 컴파일된 Go 레벨 체크입니다.

## 워커 격리

각 워커는 자체 git 워크트리(`c4/w-T-XXX-N`)에서 실행됩니다. 병렬 워커가 서로 충돌하지 않습니다. 워크트리는 submit 시 자동 merge됩니다.

## 지식 & 페르소나 루프

```
태스크 완료 → 발견 사항 기록 → 지식 베이스 업데이트
                                    ↓
다음 태스크 ← 지식 자동 주입 ← 페르소나가 스타일 학습
```

**3계층 온톨로지**가 시간이 지남에 따라 축적됩니다:
- **L1**: 로컬 패턴 (네이밍, 리뷰 선호도)
- **L2**: 프로젝트 레벨 교차 포지션 패턴
- **L3**: Hub를 통해 공유되는 집단 패턴

100개 태스크 후, CQ가 당신의 스타일에 적응합니다. 500개 후, 피드백을 예측합니다.
