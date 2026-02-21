---
description: |
  Iterative review-fix loop that runs after checkpoint and before finish.
  Spawns C4 Workers with domain="refine" for true context isolation.
  Each round: Worker reviews → Orchestrator fixes → new Worker re-reviews.
  Converges until quality gate passes (CRITICAL+HIGH = 0).
  Triggers: "리파인", "정제", "반복 리뷰", "refine", "/c4-refine",
  "review and fix loop", "quality loop", "iterative review".
---

# C4 Refine — Iterative Review-Fix Loop

반복적 품질 수렴 프로세스. Worker 기반 컨텍스트 격리로
confirmation bias 없는 재리뷰를 보장합니다.

## 위치: Checkpoint → **Refine** → Finish

```
/c4-plan → /c4-run → /c4-checkpoint → /c4-refine → /c4-finish
```

## Review vs Refine 구분

| | Review (R-xxx-0) | Refine (domain=refine) |
|---|---|---|
| **목적** | 다른 에이전트의 코드를 1회 검토 | 반복 루프로 품질 수렴 |
| **횟수** | 1회 | N회 (quality gate 통과까지) |
| **수정** | 안 함 (리뷰만) | 리뷰 + 수정 + 재리뷰 |
| **태스크** | `R-001-0` | `T-RF-{round}-0` |
| **domain** | `review` | `refine` |
| **컨텍스트** | 구현 맥락 포함 가능 | 매 라운드 완전 초기화 |

## 사용법

```
/c4-refine                    # 현재 변경 기준 refine
/c4-refine --max-rounds 5     # 최대 5회 반복
/c4-refine --threshold medium  # MEDIUM 이하까지 수정
```

## Quality Gate (종료 조건)

| 등급 | 정의 | 기본 게이트 |
|------|------|------------|
| CRITICAL | 데이터 손실, 보안 취약, 무한 루프 | 반드시 0 |
| HIGH | 논리 오류, race condition, 계약 불일치 | 반드시 0 |
| MEDIUM | 미사용 코드, 스타일, 마이너 불일치 | 허용 (2개 이하) |
| LOW | 주석 누락, 이름 개선 가능 | 무시 |

**기본 threshold**: `high` — CRITICAL과 HIGH가 모두 0이면 종료.
`--threshold medium`: MEDIUM 이하까지 수정하여 종료.

## Instructions

### Phase 0. Scope Determination

변경 범위를 확정합니다.

```python
# 방법 1: git diff 기반 (기본)
changed_files = shell("git diff --name-only HEAD~N")  # N = refine 대상 커밋 수

# 방법 2: C4 태스크 기반
status = mcp__c4__c4_status()
# 완료된 태스크의 files_changed 수집
```

대상 파일 목록을 `SCOPE`로 저장합니다.

### Phase 0.5. Knowledge Lookup (선택적)

과거 refine에서 발견된 반복 패턴을 조회합니다:

```python
# 과거 이슈 패턴 조회
patterns = c4_pattern_suggest(context="refine " + SCOPE_summary)
# 또는 Cursor: c4_knowledge_search(query="refine pattern")

# 결과가 있으면 Worker 프롬프트에 "주의 패턴"으로 전달
# 예: "과거 resilience axis에서 retry 누락이 3회 반복 발견됨"
```

결과가 없으면 건너뛰고 Phase 1로 진행합니다.

### Phase 1. Spawn Refine Worker (리뷰)

**핵심**: domain="refine" Worker를 스폰하여 컨텍스트 격리된 리뷰를 수행합니다.

#### Claude Code (C4 MCP 사용)

```python
# Refine 리뷰 태스크 생성
c4_add_todo(
    title=f"Refine Round {round} - 6-axis 코드 리뷰",
    scope=SCOPE,                    # 변경된 파일 목록
    dod="6-axis 리뷰 수행, 이슈 테이블 반환 (severity/axis/file/line/description)",
    mode="worker",                  # Worker 모드 (프로세스 격리)
    domain="refine",                # ← review가 아닌 refine
    review_required=False,
    priority=10,                    # 즉시 실행
)

# Worker 스폰 — 태스크를 할당받아 리뷰 수행
# Worker는 이전 라운드의 리뷰 결과를 전혀 모름 (fresh eyes)
```

Worker 내부 동작:
```
1. c4_get_task(worker_id) → refine 태스크 할당
2. SCOPE 파일 읽기 (c4_read_file)
3. 6-axis 리뷰 수행 (read-only)
4. 이슈 테이블을 handoff에 기록
5. c4_submit(task_id, ...) → 리뷰 결과 전달
```

#### Cursor (subagent 사용)

```python
# Task tool로 code-reviewer subagent 스폰
Task(
    subagent_type="code-reviewer",
    readonly=True,
    prompt=f"""
    Review these files for issues using 6-axis analysis:
    {SCOPE}
    Return markdown table: # | File | Line | Severity | Axis | Description
    Summary: CRITICAL: N / HIGH: N / MEDIUM: N / LOW: N
    """
)
```

**리뷰 관점 (6-axis)**:
1. **Correctness** — 로직 오류, edge case 누락, off-by-one
2. **Security** — injection, 권한 우회, 비밀 노출
3. **Resilience** — 재시도/timeout 누락, 무한 루프, 리소스 누수
4. **Consistency** — 인터페이스 불일치, 중복 로직, naming
5. **Contract** — 반환값 무시, 에러 전파 누락, 타입 불일치
6. **Integration** — 컴포넌트 간 연결, config 미반영, 포트/경로 불일치

**출력 형식**:
```
## Refine Round N — Review

| # | File | Line | Severity | Axis | Description |
|---|------|------|----------|------|-------------|
| 1 | hub/client.go | 95 | CRITICAL | Resilience | ... |
| 2 | eventbus/embedded.go | 170 | HIGH | Correctness | ... |

CRITICAL: N / HIGH: N / MEDIUM: N / LOW: N
```

### Phase 2. Fix (Orchestrator)

Phase 1의 Worker/subagent가 반환한 이슈를 **Orchestrator(메인 세션)**가 수정합니다.

**수정 순서**: CRITICAL → HIGH → MEDIUM (threshold 이하)

**규칙**:
- 한 이슈당 **최소 범위**로 수정 (surgical change)
- 수정 후 빌드 확인 (`go build ./...`, `cargo check`)
- 테스트가 있는 파일은 관련 테스트 실행
- 수정한 파일 목록을 `FIXED`로 저장

### Phase 3. Context Clear + Commit

**핵심**: 수정분 커밋 → 다음 라운드에서 새 Worker가 처음부터 리뷰.

```bash
git add -A && git commit -m "refine(round-N): [요약]"
```

**컨텍스트 클리어 메커니즘**:

| 환경 | 방법 | 격리 수준 |
|------|------|----------|
| Claude Code | 새 C4 Worker 프로세스 스폰 | **완전** (별도 프로세스, 별도 컨텍스트) |
| Cursor | 새 subagent(Task) 스폰 | **높음** (별도 컨텍스트, 동일 프로세스) |
| 수동 | 세션 종료 → 새 세션에서 재리뷰 | **완전** |

Worker/subagent는 **이전 라운드의 리뷰 결과, 수정 의도, 대화 이력**을
전혀 알지 못합니다. 코드만 보고 독립적으로 판단합니다.

### Phase 4. Loop Control

```python
round = 1
max_rounds = 5  # 기본값, --max-rounds로 변경 가능
threshold = "high"  # 기본값

while round <= max_rounds:
    # Phase 1: 새 Worker/subagent로 리뷰 (context isolated)
    issues = spawn_refine_worker(SCOPE, round)
    
    gate_issues = count_above_threshold(issues, threshold)
    if gate_issues == 0:
        print(f"Quality gate PASSED at round {round}")
        break
    
    # Phase 2: Orchestrator가 수정
    fix(issues, threshold)
    
    # Phase 3: 커밋 (다음 Worker가 이 커밋 기준으로 리뷰)
    commit_fixes(round)
    
    round += 1

if round > max_rounds:
    print(f"WARNING: Max rounds ({max_rounds}) reached.")
    print(f"Remaining: {remaining_issues}")
```

### Phase 5. Report + Gate Recording

최종 리포트를 생성합니다.

```
## Refine Summary

- Rounds: N/M (converged / max)
- Total issues found: X
- Fixed: Y (CRITICAL: a, HIGH: b, MEDIUM: c)
- Remaining: Z (all below threshold)

### Round History
| Round | Worker | CRIT | HIGH | MED | LOW | Action |
|-------|--------|------|------|-----|-----|--------|
| 1     | worker-a1b2 | 2 | 2 | 4 | 1 | Fixed 4 |
| 2     | worker-c3d4 | 0 | 1 | 3 | 1 | Fixed 1 |
| 3     | worker-e5f6 | 0 | 0 | 2 | 1 | PASS   |
```

**Refine 완료 후 반드시 게이트를 DB에 기록한다 (c4-finish가 이 레코드를 확인):**

```bash
sqlite3 .c4/c4.db "
CREATE TABLE IF NOT EXISTS c4_gates (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id     TEXT,
  gate         TEXT    NOT NULL,
  status       TEXT    NOT NULL CHECK(status IN ('done','skipped','override')),
  reason       TEXT,
  completed_at TEXT    DEFAULT (datetime('now'))
);
INSERT INTO c4_gates (gate, status, reason)
  VALUES ('refine', 'done', 'quality gate passed at round ${round}/${max_rounds}');"

echo "✅ refine gate recorded → c4-finish 진행 가능"
```

### Phase 5.5. Knowledge Recording (자동)

반복 발견된 이슈 패턴을 knowledge에 기록하여 다음 프로젝트에서 재활용합니다.

**규칙**: 같은 axis에서 2회 이상 발견된 이슈 유형 → pattern으로 자동 기록

```python
# Claude Code:
c4_knowledge_record(
    doc_type="pattern",
    title=f"Refine pattern: {axis} issues ({count}x)",
    content="## 반복 이슈 패턴\n\n" + issue_descriptions,
    tags=["refine", "auto-pattern", axis],
)

# Cursor:
# 이슈 요약을 knowledge_record로 기록하거나
# 별도 insight 문서로 남김
```

이를 통해 다음 `/c4-plan`의 Phase 0.1에서 `c4_pattern_suggest`가
"과거 refine에서 resilience axis 이슈가 빈발" 등의 패턴을 자동 반환합니다.

## Hook Integration

### C4 Workflow Hook

`/c4-finish` 스킬에서 refine을 자동 호출:

```yaml
# config.yaml
refine:
  enabled: true
  auto_run: true         # /c4-finish 전 자동 실행
  max_rounds: 5
  threshold: "high"      # high | medium | critical
  scope: "git"           # git | task
```

### 수동 호출

```
/c4-refine                          # 기본 설정으로 실행
/c4-refine --threshold medium       # MEDIUM까지 수정
/c4-refine --max-rounds 3           # 최대 3회
/c4-refine --scope "hub/ eventbus/" # 특정 디렉토리만
```

## Exit Conditions

| 조건 | 동작 |
|------|------|
| Quality gate 통과 | 정상 종료, `/c4-finish`로 진행 |
| Max rounds 도달 | 경고 + 잔여 이슈 리포트, 사용자에게 판단 위임 |
| 새 이슈가 이전보다 증가 | 경고 (regression 의심), 사용자 확인 |
| 빌드/테스트 실패 | 수정 필수, 라운드 카운트 소비하지 않음 |

## Task ID Convention

```
T-RF-001-0  : Refine Round 1 리뷰 태스크 (domain=refine)
T-RF-002-0  : Refine Round 2 리뷰 태스크
...
```

`T-RF-` prefix로 일반 구현(T-), 리뷰(R-), 체크포인트(CP-)와 구분.

## Related Skills

| 스킬 | 연결 |
|------|------|
| `/c4-plan` | Refine 대상 태스크의 원본 계획 |
| `/c4-submit` | Refine 결과 제출 (handoff JSON 포함) |
| `/c4-checkpoint` | Refine 완료 후 체크포인트 리뷰 |
| `/c4-finish` | 전체 Refine 루프 완료 → auto-distill |
| `/c4-run` | Refine fix 태스크의 Worker 스폰 |
