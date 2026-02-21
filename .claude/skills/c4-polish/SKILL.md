---
description: |
  Continuous build-test-review-fix loop until reviewer finds zero modifications.
  Each round: Build → Test → Spawn fresh reviewer → If issues found: fix + commit → repeat.
  DoD: Reviewer returns "no changes needed" (0 modifications).
  This is the coding version of c4-refine — wraps the full c4-finish quality loop.
  Triggers: "polish", "c4-polish", "polish loop", "수정 없을 때까지", "계속 돌려",
  "refine loop", "반복 수정", "polish until clean", "빌드 테스트 리뷰 반복".
---

# C4 Polish — Build-Test-Review-Fix Loop

빌드→테스트→리뷰→수정을 **수정사항이 0이 될 때까지** 반복합니다.
매 라운드마다 새 리뷰어를 스폰하여 confirmation bias를 제거합니다.

## 위치: 구현 완료 → **Polish** → Finish

```
/c4-run (구현) → /c4-polish (정제 루프) → /c4-finish (마무리)
```

## c4-refine과의 차이

| | c4-refine | c4-polish |
|---|---|---|
| **루프 단위** | 리뷰 → 수정 | 빌드 → 테스트 → 리뷰 → 수정 |
| **종료 조건** | CRITICAL+HIGH = 0 | 수정사항 = 0 |
| **빌드/테스트** | 수동 | 매 라운드 자동 |
| **사용 시점** | checkpoint 이후 | 구현 직후, finish 직전 |
| **강도** | 품질 게이트 기반 | 완전 수렴 기반 |

## DoD

**"리뷰어가 수정할 것이 없다고 판단하는 라운드"** 에서 종료.

- 리뷰어가 이슈 0개 반환 → 즉시 종료
- 이슈가 LOW만 남은 경우 → 기본적으로 종료 (--threshold low 시 계속)
- max-rounds 도달 → 경고 후 종료

## 사용법

```
/c4-polish                        # 기본 (최대 8라운드, threshold=medium)
/c4-polish --max-rounds 5         # 최대 5회
/c4-polish --threshold low        # LOW 이슈도 0이 될 때까지
/c4-polish --scope "hub/ llm/"    # 특정 디렉토리만
/c4-polish --no-test              # 테스트 생략 (빠른 UI 작업 등)
```

## Instructions

### Phase 0. Phase Lock Acquire (Advisory)

동시 실행 방지를 위한 advisory lock을 획득합니다.

```python
result = c4_phase_lock_acquire(phase="polish")
if not result["acquired"]:
    err = result["error"]
    print(f"다른 세션이 Polish 중입니다 ({err['message']})")
    print("Override하시겠습니까? (Y/N)")
    # N이면 종료
    # Y면 강제 override로 진행 (lock 파일 무시)
```

- `acquired: true` → 정상 진행
- `acquired: false, code: LOCK_HELD` → 사용자에게 override 여부 확인
- 종료 시 반드시 `c4_phase_lock_release(phase="polish")` 호출

### Phase 0.5. Scope Determination

리뷰 대상 파일을 확정합니다.

```python
# 기본: 마지막 N개 커밋의 변경 파일
changed_files = shell("git diff --name-only HEAD~3")

# --scope 지정 시: 해당 경로
# scope이 없으면 git diff origin/main..HEAD로 전체 변경 파일
```

언어별 빌드/테스트 명령을 자동 감지합니다:
- Go (`*.go`) → `go build ./... && go vet ./...` / `go test -count=1 -p 1 ./...`
- Python (`*.py`) → `uv run python -m py_compile` / `uv run pytest tests/ -x`
- Rust (`*.rs`) → `cargo check` / `cargo test`
- TypeScript (`*.ts`) → `tsc --noEmit` / `pnpm test`

### Phase 1. Build + Test

```bash
# Go
cd c4-core && go build ./... && go vet ./...
cd c4-core && go test -count=1 -p 1 ./...

# Python (변경 있을 때만)
uv run pytest tests/ -x

# C5 (변경 있을 때만)
cd c5 && go test ./...
```

**빌드/테스트 실패 시**:
- 즉시 수정 → 수정 후 이 Phase 재실행
- 라운드 카운트를 소비하지 않음 (빌드 오류는 리뷰 전 처리)
- 수정한 내용을 커밋하고 Phase 1부터 재시작

### Phase 2. Spawn Fresh Reviewer

**핵심**: 이전 라운드와 완전히 격리된 새 리뷰어를 스폰합니다.

#### Claude Code (C4 Worker)

```python
c4_add_todo(
    title=f"Polish Round {round} — 코드 리뷰 (수정사항 탐지)",
    scope=SCOPE,
    dod="""
    6-axis 코드 리뷰 수행. 반환 형식:
    ## Round N Review
    | # | File | Line | Severity | Axis | Description | Fix |
    수정 필요 없으면: '## Round N Review\\n수정사항 없음 (PASS)'
    Summary: MODIFICATIONS: N (CRITICAL: a, HIGH: b, MEDIUM: c, LOW: d)
    """,
    mode="worker",
    domain="refine",
    review_required=False,
    priority=10,
)
# Worker 스폰 → 리뷰 결과 대기
```

#### Cursor (subagent)

```python
Task(
    subagent_type="code-reviewer",
    prompt=f"""
    Review these files. Focus on: correctness, security, resilience, consistency, contract, integration.

    Files: {SCOPE}

    Return a markdown table:
    | # | File | Line | Severity | Axis | Description | Fix |

    If NOTHING needs to be changed, return exactly:
    ## Round {round} Review
    수정사항 없음 (PASS)
    MODIFICATIONS: 0

    Otherwise:
    MODIFICATIONS: N (CRITICAL: a, HIGH: b, MEDIUM: c, LOW: d)
    """
)
```

**리뷰 관점 (6-axis)**:
1. **Correctness** — 로직 오류, edge case, off-by-one, nil/null 참조
2. **Security** — injection, 권한 우회, 비밀 노출, SSRF
3. **Resilience** — 재시도 누락, timeout 없음, 리소스 누수, goroutine leak
4. **Consistency** — 인터페이스 불일치, 중복 로직, naming convention
5. **Contract** — 에러 전파 누락, 반환값 무시, 타입 불일치
6. **Integration** — 컴포넌트 연결, config 미반영, 포트/경로 불일치

### Phase 3. Convergence Check

```python
if review.modifications == 0:
    print(f"✅ CONVERGED at round {round}")
    break  # 루프 종료

# threshold별 종료 조건
if threshold == "medium" and review.critical == 0 and review.high == 0:
    print(f"✅ Quality gate PASSED at round {round}")
    break
```

**수정사항이 있으면 Phase 4로**:

### Phase 4. Fix + Commit

```python
# 수정 순서: CRITICAL → HIGH → MEDIUM → LOW (threshold 이하)
for issue in sorted_issues:
    fix_issue(issue)  # surgical change only

# 빌드 확인
shell("cd c4-core && go build ./...")

# 커밋
shell(f'git add -A && git commit -m "polish(round-{round}): {summary}"')

round += 1
# → Phase 1로 돌아감
```

**수정 규칙**:
- 이슈와 **직접 관련된 줄만** 수정 (surgical)
- 수정 후 해당 패키지 빌드 확인
- 수정 파일 목록을 `FIXED_THIS_ROUND`로 기록

### Phase 5. Loop Control

```python
round = 1
max_rounds = 8  # 기본값
threshold = "medium"  # 기본값: MEDIUM 이하 이슈만 허용

while round <= max_rounds:
    # Phase 1: Build + Test
    if not build_and_test():
        fix_build_errors()  # 라운드 소비 없이 수정
        continue

    # Phase 2: 새 리뷰어 스폰
    review = spawn_fresh_reviewer(SCOPE, round)

    # Phase 3: 수렴 체크
    if is_converged(review, threshold):
        print(f"✅ Converged at round {round}/{max_rounds}")
        break

    # Phase 4: 수정 + 커밋
    fix_and_commit(review, round)
    round += 1

else:
    print(f"⚠️ Max rounds ({max_rounds}) reached")
    print_remaining_issues(review)
```

### Phase 6. Final Report

```
## Polish Summary

- Rounds: N/M (converged / max reached)
- Total modifications found: X → Fixed: Y
- Remaining: Z (all below threshold)
- Files touched: [list]

### Round History
| Round | Reviewer | CRIT | HIGH | MED | LOW | Mods | Action |
|-------|----------|------|------|-----|-----|------|--------|
| 1     | worker-a1 | 1  | 2   | 4   | 2   | 9    | Fixed  |
| 2     | worker-b2 | 0  | 1   | 2   | 1   | 4    | Fixed  |
| 3     | worker-c3 | 0  | 0   | 0   | 1   | 0    | PASS ✅|
```

### Phase 6.5. Knowledge Recording

반복 발견 패턴을 기록합니다:

```python
# 2회 이상 같은 axis에서 이슈 발견 시
if recurring_issues:
    c4_knowledge_record(
        doc_type="pattern",
        title=f"Polish pattern: {axis} ({count}x rounds)",
        content=issue_summary,
        tags=["polish", "auto-pattern", axis],
    )
```

## Exit Conditions

| 조건 | 동작 |
|------|------|
| 수정사항 0 | ✅ 정상 종료 → `/c4-finish`로 진행 |
| Quality gate 통과 (threshold=medium) | ✅ 정상 종료 |
| Max rounds 도달 | ⚠️ 경고 + 잔여 이슈 리포트 → 사용자 판단 |
| 빌드 실패 | 즉시 수정, 라운드 카운트 불소모 |
| 새 이슈 > 이전 이슈 (regression) | ⚠️ 경고, 사용자 확인 요청 |

## Integration

### c4-finish와 연동

`/c4-finish` 전에 `/c4-polish`를 먼저 실행하면 가장 깨끗한 상태로 마무리됩니다:

```
/c4-polish        # 수정사항 0까지 정제
/c4-finish        # 빌드 검증 + 바이너리 설치 + 커밋
```

### 자동 연동 (config.yaml)

```yaml
# .c4/config.yaml
polish:
  enabled: true
  max_rounds: 8
  threshold: "medium"   # medium | high | low
  auto_before_finish: false  # true로 설정 시 /c4-finish 전 자동 실행
```

## Task ID Convention

```
T-PL-001-0  : Polish Round 1 리뷰 태스크
T-PL-002-0  : Polish Round 2 리뷰 태스크
```

`T-PL-` prefix로 일반 구현(T-), 리뷰(R-), 리파인(T-RF-)과 구분.

## Related Skills

| 스킬 | 연결 |
|------|------|
| `/c4-refine` | 품질 게이트 기반 정제 (checkpoint 이후) |
| `/c4-finish` | Polish 완료 후 최종 마무리 |
| `/c4-validate` | 단발성 검증 (루프 없음) |
| `/c4-run` | 구현 태스크 실행 (polish 전 단계) |
