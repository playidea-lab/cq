# C9 Loop

C4 워크플로우를 ML 연구에 적용한 완전 자율 루프 드라이버.
state.yaml의 현재 phase를 읽어 다음 단계를 자동 실행.

**트리거**: "c9-loop", "루프 시작", "다음 단계", "연구 루프"

## Phase 상태 머신

```
CONFERENCE → IMPLEMENT → RUN → CHECK → REFINE → (다시 CONFERENCE)
                                    └→ FINISH (수렴)
```

IMPLEMENT는 CONFERENCE에서 결정된 실험을 실제로 실행 가능하게 만드는 단계.
코드 작성, 설정 파일 생성, 의존성 설치 등. 결과물이 확인된 후 RUN으로 전환.

## 실행 순서

### Step 1: state.yaml 읽기
```bash
cat .c9/state.yaml
```
현재 phase, round, mpjpe_history, active_jobs, notify 확인.

### Step 2: Phase별 행동

**phase=CONFERENCE** (실험 설계):
→ `/c9-conference` 스킬 실행
- mpjpe_history와 이전 round conference.txt를 컨텍스트로 제공
- 합의된 다음 실험들을 `.c9/experiments/rN_*.yaml`로 저장
- 각 실험의 `blocked_by` 확인 → 구현 필요 시 state.yaml phase → IMPLEMENT
- 구현 불필요 시 바로 phase → RUN
- Dooray 알림: `./scripts/c9-notify.sh CONFERENCE "Round N 실험 계획 수립" N`

**phase=IMPLEMENT** (코드 구현):
→ C5 Hub Job으로 pi 서버에 코드 작성/배포
- `.c9/experiments/rN_*.yaml`의 `implement` 섹션 실행
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
- `.c9/rounds/rN/results.txt`에서 MPJPE 파싱
- 수렴 여부 판단:
  - 수렴 → phase=FINISH
  - 미수렴 → phase=REFINE, round++
  - BLOCKED → phase=CONFERENCE
- Dooray 알림: `./scripts/c9-notify.sh CHECK "Round N 결과: MPJPE=Xmm (개선 Y.Ymm)" N "mpjpe=X,pa=Y,util=Z"`
- **즉시 다음 phase 자동 실행** (사용자 승인 없이)

**phase=REFINE** (재설계):
→ `/c9-conference` 스킬 실행 (새 가설로)
- 이전 결과를 컨텍스트로 제공
- 새 실험 `.c9/experiments/r(N+1)_*.yaml` 생성
- state.yaml phase → IMPLEMENT (또는 RUN), round → N+1
- **즉시 IMPLEMENT → RUN 진행**
- Dooray 알림: `./scripts/c9-notify.sh REFINE "Round N+1 새 가설 수립" N+1`

**phase=FINISH** (완료):
→ `/c9-finish` 스킬 실행
- best model 저장 + 결과 문서화
- Dooray 알림: `./scripts/c9-notify.sh FINISH "연구 완료! Best MPJPE=Xmm (개선 Y.Ymm, Z라운드)" N "best_mpjpe=X,baseline=102.6,improvement=Y"`

**phase=DONE**:
→ 루프 종료. `/c9-deploy`는 별도 실행.

### Step 3: 루프 상태 출력 (매 phase 전환 시)

```
## C9 Loop 상태
Round: N / 최대 10
Phase: [현재 phase]
Best MPJPE: Xmm (Round N, exp_name)
Baseline: 102.6mm (개선: X.Xmm)

[히스토리 테이블]
| Round | Best Exp | MPJPE | PA-MPJPE | 개선 |
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
- 결과: MPJPE=Xmm, PA=Ymm, Util=Z%
- 개선: +/-X.Xmm vs baseline
- 핵심 발견: ...
- 다음 방향: ...
```

## 주의

- CONFERENCE phase에서 실험 yaml 생성 시: `rN_expname.yaml` 형식 필수
- RUN 중 blocked 실험 있으면 → 자동으로 CONFERENCE로 전환
- 수동 개입 필요 시: state.yaml phase 직접 수정 가능
- standby 모드에서 컨텍스트 소진 시: `/c9-loop` 재실행으로 재개 (state.yaml이 SSOT)
