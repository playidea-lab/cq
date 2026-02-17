---
description: |
  Iterative review-fix loop that runs after checkpoint and before finish.
  Reviews code changes, identifies issues by severity, fixes them, then
  re-reviews with fresh context until quality threshold is met.
  Hook-based loop: review → fix → context clear → re-review → repeat.
  Triggers: "리파인", "정제", "반복 리뷰", "refine", "/c4-refine",
  "review and fix loop", "quality loop", "iterative review".
---

# C4 Refine — Iterative Review-Fix Loop

리뷰 → 수정 → 컨텍스트 초기화 → 재리뷰 루프를 반복하여
코드 품질을 일정 수준 이하로 수렴시킵니다.

## 위치: Checkpoint → **Refine** → Finish

```
/c4-plan → /c4-run → /c4-checkpoint → /c4-refine → /c4-finish
```

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

### Phase 1. Review (Read-Only)

변경된 파일을 읽고 이슈를 분류합니다.

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
```

**카운트 요약**:
```
CRITICAL: N / HIGH: N / MEDIUM: N / LOW: N
```

### Phase 2. Fix

Phase 1에서 발견된 이슈를 severity 순서로 수정합니다.

**수정 순서**: CRITICAL → HIGH → MEDIUM (threshold 이하)

**규칙**:
- 한 이슈당 **최소 범위**로 수정 (surgical change)
- 수정 후 빌드 확인 (`go build ./...`, `cargo check`)
- 테스트가 있는 파일은 관련 테스트 실행
- 수정한 파일 목록을 `FIXED`로 저장

### Phase 3. Context Clear

**핵심**: 이전 리뷰의 편향(confirmation bias)을 제거합니다.

```
# 선행 수정 커밋
git add -A && git commit -m "refine(round-N): [요약]"

# 컨텍스트 초기화:
# - 이전 리뷰 결과를 잊고, 코드만 다시 읽음
# - 새 subagent를 스폰하여 fresh eyes로 리뷰
# - 또는 파일을 처음부터 다시 읽기
```

Cursor/Claude Code에서:
- **subagent 스폰**: Task tool로 read-only 리뷰 에이전트 생성
- **셀프 리뷰**: SCOPE 파일을 처음부터 다시 읽고 Phase 1 반복

### Phase 4. Loop Control

```python
round = 1
max_rounds = 5  # 기본값, --max-rounds로 변경 가능
threshold = "high"  # 기본값

while round <= max_rounds:
    issues = review(SCOPE)  # Phase 1
    
    gate_issues = count_above_threshold(issues, threshold)
    if gate_issues == 0:
        print(f"Quality gate passed at round {round}")
        break
    
    fix(issues, threshold)   # Phase 2
    commit_fixes(round)      # Phase 3 - commit
    clear_context()          # Phase 3 - clear
    round += 1

if round > max_rounds:
    print(f"WARNING: Max rounds ({max_rounds}) reached. Remaining issues:")
    print(remaining_issues)
```

### Phase 5. Report

최종 리포트를 생성합니다.

```
## Refine Summary

- Rounds: N/M (converged / max)
- Total issues found: X
- Fixed: Y (CRITICAL: a, HIGH: b, MEDIUM: c)
- Remaining: Z (all below threshold)

### Round History
| Round | CRIT | HIGH | MED | LOW | Action |
|-------|------|------|-----|-----|--------|
| 1     | 2    | 2    | 4   | 1   | Fixed 4 |
| 2     | 0    | 1    | 3   | 1   | Fixed 1 |
| 3     | 0    | 0    | 2   | 1   | PASS   |
```

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

## Cursor에서의 실행 방식

Cursor에서는 C4 MCP 도구가 없으므로 직접 실행합니다:

1. `git diff --name-only` 로 SCOPE 결정
2. 각 파일 Read → 6-axis 리뷰
3. 이슈 수정 → build/test 확인
4. 커밋 후 Task(subagent_type="code-reviewer") 스폰하여 재리뷰
5. 반환된 이슈 수가 threshold 이하이면 종료
