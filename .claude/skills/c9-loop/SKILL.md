# C9 Loop

> **Read-Only Notice**: `LoopOrchestrator` (serve component) is the single writer for
> `.c9/state.yaml`. This skill reads state for status display and phase routing.
> Direct writes to state.yaml from this skill are deprecated — all state transitions
> (gate_wait → running → stopped) happen inside LoopOrchestrator automatically.
> Use `c4_research_loop_start` to start a loop and `c4_research_intervene` to steer.

C4 워크플로우를 ML 연구에 적용한 완전 자율 루프 드라이버.
state.yaml의 현재 phase를 읽어 다음 단계를 자동 실행.

**트리거**: "c9-loop", "루프 시작", "다음 단계", "연구 루프"

## Phase 상태 머신

```
CONFERENCE → IMPLEMENT → RUN → CHECK → REFINE → CONFERENCE (반복)
                                    └→ FINISH (수렴)
※ SURVEY는 수동 실행만 (`/c9-survey`). 루프 자동 실행 대상 아님.
```

IMPLEMENT는 CONFERENCE에서 결정된 실험을 실제로 실행 가능하게 만드는 단계.
코드 작성, 설정 파일 생성, 의존성 설치 등. 결과물이 확인된 후 RUN으로 전환.

## 실행 순서

### Step 0: state.yaml 존재 확인
```bash
ls .c9/state.yaml 2>/dev/null || echo "NOT FOUND"
```
- **없으면**: "/c9-init을 먼저 실행해 C9 프로젝트를 초기화하세요." 안내 후 종료.
- **있으면**: Step 1로 진행.

### Step 1: state.yaml 읽기
```bash
cat .c9/state.yaml
```
현재 phase, round, metric_history, active_jobs, notify 확인.
- `metric.name`: 측정 지표명 (예: MPJPE, accuracy, F1)
- `metric.baseline`: 기준 수치
- `metric.unit`: 단위 (예: mm, %, score)
- `metric.lower_is_better`: true/false (낮을수록 좋은지 여부)

### Step 2: Phase별 행동

**phase=CONFERENCE** (실험 설계):
→ `/c9-conference` 스킬 실행
- **[Knowledge] 과거 실험 검색 먼저** (설계 컨텍스트 보강, non-blocking):
  ```
  c4_knowledge_search("{metric.name} experiment round N", doc_type="experiment")
  → 결과 있으면 컨텍스트로 주입 후 토론 시작
  # 실패 시(도구 미존재/네트워크 오류) → 무시하고 진행
  ```
- metric_history와 이전 round conference.txt를 컨텍스트로 제공
- 합의된 다음 실험들을 `.c9/experiments/rN_*.yaml`로 저장
- 각 실험의 `blocked_by` 확인 → 구현 필요 시 state.yaml phase → IMPLEMENT
- 구현 불필요 시 바로 phase → RUN
- Dooray 알림: `./scripts/c9-notify.sh CONFERENCE "Round N 실험 계획 수립" N`

**phase=IMPLEMENT** (코드 구현):
→ C5 Hub Job으로 원격 서버에 코드 작성/배포
- `.c9/experiments/rN_*.yaml`의 `implement` 섹션에 명시된 작업 실행:
  - 코드 파일 생성/수정 (모델, 학습 스크립트, 설정 등)
  - 의존성 설치 (`uv add`, `apt install` 등)
  - 데이터 전처리 스크립트 실행
- 구현 완료 검증 (`[C9-IMPL-DONE]` 마커 확인)
- state.yaml phase → RUN
- Dooray 알림: `./scripts/c9-notify.sh IMPLEMENT "구현 완료 → RUN 전환" N`

**phase=RUN** (실험 실행):
1. `./scripts/c9-run.sh [round]` 실행
   - `.c9/experiments/rN_*.yaml` 파일들을 C5 Hub에 제출
   - state.yaml active_jobs에 job_id 기록
2. Dooray 알림 발송: `./scripts/c9-notify.sh RUN "Round N 실험 제출 — 훈련 시작" N`
3. **자동 standby 진입** → `/c9-standby` 스킬 실행
   - cq mail `[C9-CHECK]` 수신 대기
   - 수신 시 자동 CHECK 실행

**phase=CHECK** (결과 판정):
→ `./scripts/c9-check.sh [round]` 실행
- `.c9/rounds/rN/results.txt`에서 `{metric.name}` 파싱
- 수렴 여부 판단:
  - 수렴 → phase=FINISH
  - 미수렴 → phase=REFINE, round++
  - BLOCKED → phase=CONFERENCE
- Dooray 알림: `./scripts/c9-notify.sh CHECK "Round N 결과: {metric.name}=X{metric.unit} (개선 Y.Y{metric.unit})" N "{metric.name}=X,util=Z"`
- **[Knowledge] 실험 결과 기록** (non-blocking):
  ```
  c4_experiment_record(
    title="C9 Round N exp_name",
    content="{metric.name}=X{metric.unit}, util=Z",
    tags=["c9", "round-N"]
  )
  # 실패 시(도구 미존재/네트워크 오류) → 무시하고 진행
  # 파라미터 오류 시 → 경고 후 진행
  ```
- **즉시 다음 phase 자동 실행** (사용자 승인 없이)

**phase=REFINE** (재설계):
→ `/c9-conference` 스킬 실행 (새 가설로)
- **[Knowledge] 과거 실험 검색 먼저** (토론 컨텍스트 보강, non-blocking):
  ```
  c4_knowledge_search("{metric.name} experiment round N", doc_type="experiment")
  → 결과 있으면 컨텍스트로 주입 후 토론 시작
  # 실패 시(도구 미존재/네트워크 오류) → 무시하고 진행
  ```
- 이전 결과를 컨텍스트로 제공
- 새 실험 `.c9/experiments/r(N+1)_*.yaml` 생성
- state.yaml round → N+1
- 새 실험 yaml에 `blocked_by:` 구현 필요 항목이 있으면 phase → IMPLEMENT, 없으면 phase → RUN
- **즉시 IMPLEMENT → RUN 진행**
- Dooray 알림: `./scripts/c9-notify.sh REFINE "Round N+1 새 가설 수립" N+1`

**phase=FINISH** (완료):
→ `/c9-finish` 스킬 실행
- best model 저장 + 결과 문서화
- Dooray 알림: `./scripts/c9-notify.sh FINISH "연구 완료! Best {metric.name}=X{metric.unit} (개선 Y.Y{metric.unit}, Z라운드)" N "best_{metric.name}=X,baseline={metric.baseline},improvement=Y"`
- **[Knowledge] 연구 인사이트 기록** (non-blocking):
  ```
  c4_knowledge_record(
    doc_type="insight",
    title="C9 Research Completed: Best {metric.name}=X{metric.unit}",
    content="Best round: N, exp: exp_name. {metric.name}=X{metric.unit}. 개선: Z{metric.unit} vs baseline {metric.baseline}{metric.unit}. 핵심 발견: ...",
    tags=["c9", "completed"]
  )
  # 실패 시(도구 미존재/네트워크 오류) → 무시하고 진행
  # 파라미터 오류 시 → 경고 후 진행
  ```

**phase=DONE**:
→ 루프 종료. `/c9-deploy`는 별도 실행.

### Step 3: 루프 상태 출력 (매 phase 전환 시)

```
## C9 Loop 상태
Round: N / 최대 10
Phase: [현재 phase]
Best {metric.name}: X{metric.unit} (Round N, exp_name)
Baseline: {metric.baseline}{metric.unit} (개선: X.X{metric.unit})

[히스토리 테이블]
| Round | Best Exp | {metric.name} | 개선 |
```

## 자율 실행 규칙

1. **사용자 승인 없이 진행**: CHECK → REFINE → CONFERENCE → IMPLEMENT → RUN 전부 자동
2. **중단 조건**:
   - `[C9-BLOCKED]`이 2회 연속 → 두레이로 알림 후 사용자 입력 대기
   - max_rounds 도달 → 자동 FINISH
   - 예상치 못한 에러 → Dooray 알림 + state.yaml에 error 기록
3. **FINISH는 자동이지만 DEPLOY는 수동**: 배포는 항상 사용자가 `/c9-deploy`로 직접 실행
4. **각 phase 전환마다 두레이 알림**: 사용자가 언제든 진행 상황 확인 가능

## 진행 보고서 자동 작성

매 라운드 완료 시 `.c9/RESEARCH_LOG.md`에 자동 추가:
```markdown
## Round N — [날짜]
- 가설: ...
- 실험: exp_name
- 결과: {metric.name}=X{metric.unit}, Util=Z%
- 개선: +/-X.X{metric.unit} vs baseline
- 핵심 발견: ...
- 다음 방향: ...
```

## 주의

- CONFERENCE phase에서 실험 yaml 생성 시: `rN_expname.yaml` 형식 필수
- RUN 중 blocked 실험 있으면 → 자동으로 CONFERENCE로 전환
- 수동 개입 필요 시: state.yaml phase 직접 수정 가능
- standby 모드에서 컨텍스트 소진 시: `/c9-loop` 재실행으로 재개 (state.yaml이 SSOT)
