---
name: c4-finish
description: |
  Post-implementation completion workflow: Polish Loop (quality convergence) →
  build verification → testing → binary installation → documentation → knowledge
  recording → commit. Polish loop is built-in — no separate /c4-polish needed.
  Triggers: "마무리", "완료 루틴", "구현 마무리", "finish", "c4-finish", "wrap up",
  "finalize", "complete implementation", "post-implementation".
---

# C4 Finish Routine

구현 완료 후 품질 수렴 + 마무리 워크플로우. **순서대로 모든 단계를 실행한다.**

```
/c4-plan (지식 출구) → /c4-run → /c4-finish (지식 입구)
```

## Steps

### 0. Polish Loop (Build-Test-Review-Fix, 내장)

**수정사항이 0이 될 때까지 반복.** 별도 `/c4-polish` 실행 불필요.

#### 0.1 Phase Lock + Scope

```python
result = c4_phase_lock_acquire(phase="polish")
# acquired: false → 사용자에게 override 여부 확인
changed_files = shell("git diff --name-only origin/main..HEAD")
```

#### 0.2 Knowledge Lookup

```python
patterns = c4_pattern_suggest(context="polish refine " + SCOPE_summary)
# 결과 있으면 리뷰어 프롬프트에 전달. 없으면 건너뜀.
```

#### 0.3 Loop (최대 8라운드)

```python
round = 1
while round <= 8:
    shell("cd c4-core && go build ./... && go vet ./...")
    shell("cd c4-core && go test -count=1 -p 1 ./...")

    review = Task(subagent_type="code-reviewer", prompt=f"""
    Review: {SCOPE}
    6-axis: correctness, security, resilience, consistency, contract, integration
    수정 없으면: MODIFICATIONS: 0
    """)

    if review.modifications == 0:
        break
    fix_issues(review)
    shell(f'git add -A && git commit -m "polish(round-{round}): {summary}"')
    round += 1
```

#### 0.4 Gate Recording

```bash
sqlite3 .c4/c4.db "INSERT INTO c4_gates (gate, status, reason) VALUES ('polish', 'done', '...');"
c4_phase_lock_release(phase="polish")
```

### 1. Phase Lock Acquire

```python
result = c4_phase_lock_acquire(phase="finish")
# acquired: false → override 여부 확인
```

### 2. Verify Build
```bash
cd c4-core && go build ./... && go vet ./...
```

### 3. Run Tests
```bash
cd c4-core && go test -count=1 -p 1 ./...
```
Python 변경 시: `uv run pytest tests/ -x`. C5 변경 시: `cd c5 && go test ./...`

### 4. Verify Worker Output (C4 workflow 사용 시)
`c4_status`로 태스크 상태 + `commit_sha` 존재 여부 확인.

### 5. Install Binary
```bash
cd c4-core && go build -o ~/.local/bin/cq ./cmd/c4/
```

### 6. Update Documentation
변경된 기능 문서 업데이트 (AGENTS.md 등). MEMORY.md에 주요 변경 기록.

### 7. Learn & Record
`c4_knowledge_record`로 인사이트 기록. 반복 실수 → MEMORY.md.

### 7.5. Auto-Distill (knowledge 5건+ 시)
```python
if c4_knowledge_stats().total_docs >= 5:
    c4_knowledge_distill(dry_run=False)
```

### 7.6. Persona Evolution
```bash
./scripts/soul-check.sh
```

### 7.7. POP Extract
세션 요약을 `c4_pop_extract(content=session_summary)`로 주입. 실패 시 non-fatal.

### 8. Git Commit
Conventional commit. 완료 후 `c4_phase_lock_release(phase="finish")`.
알림: `c4_notify(message='구현 완료', event='finish.complete')`

### 9. Release Notes
```
/c4-release
```
CHANGELOG.md + 태그. push는 사용자가 직접.

## Rules
- 단계를 건너뛰지 않는다
- Build/Test 실패 시 다음 단계 진행 금지
- Binary 설치 후 "세션 재시작 필요" 안내
