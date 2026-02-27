---
description: |
  Post-implementation completion workflow with build verification, testing,
  binary installation, documentation updates, and knowledge recording.
  Execute after implementing features or fixes. Triggers: "마무리", "완료 루틴",
  "구현 마무리", "finish", "c4-finish", "wrap up", "finalize", "complete implementation",
  "post-implementation".
---

# C4 Finish Routine

Post-implementation completion workflow. Execute ALL steps in order.

## Steps

### 0. Gate Check (MANDATORY — 항상 첫 번째)

**세션 메모리가 아닌 DB 상태로 판단한다. 컨텍스트 소진 후 재개해도 동일하게 동작.**

```bash
# Step 0.1: c4_gates 테이블 생성 (없으면)
sqlite3 .c4/c4.db "
CREATE TABLE IF NOT EXISTS c4_gates (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id     TEXT,
  gate         TEXT    NOT NULL,
  status       TEXT    NOT NULL CHECK(status IN ('done','skipped','override')),
  reason       TEXT,
  completed_at TEXT    DEFAULT (datetime('now'))
);"

# Step 0.2: polish 게이트 상태 조회
sqlite3 .c4/c4.db \
  "SELECT gate, status, reason, completed_at FROM c4_gates ORDER BY completed_at DESC LIMIT 5;"
```

| 조회 결과 | 동작 |
|---------|------|
| `polish \| done` 레코드 있음 | ✅ Step 1로 진행 |
| 레코드 없음 | ⛔ **중단** → `/c4-polish` 먼저 실행 |
| `polish \| skipped` | ⚠️ 사유 표시 + 사용자 명시 확인 후 진행 |

**`--no-polish` 긴급 예외 시** (사유 없이는 불가):
```bash
sqlite3 .c4/c4.db \
  "INSERT INTO c4_gates (gate, status, reason) VALUES ('polish', 'skipped', '${사유 필수}')"
```

> ⛔ **레코드가 없으면 c4-finish를 진행하지 않는다.**
> "이번 세션에서 polish를 했다"는 기억에 의존하지 않는다.

### 1. Phase Lock Acquire (Advisory)

동시 실행 방지를 위한 advisory lock을 획득합니다.

```python
result = c4_phase_lock_acquire(phase="finish")
if not result["acquired"]:
    err = result["error"]
    print(f"다른 세션이 Finish 중입니다 ({err['message']})")
    print("Override하시겠습니까? (Y/N)")
    # N이면 종료
    # Y면 강제 override로 진행
```

- `acquired: true` → 정상 진행
- `acquired: false, code: LOCK_HELD` → 사용자에게 override 여부 확인
- 완료 시 `c4_phase_lock_release(phase="finish")` 호출

### 2. Verify Build
```bash
cd c4-core && go build ./... && go vet ./...
```
- Build/vet 실패 시 → 수정 후 재시도, 통과할 때까지 다음 단계 진행 금지

### 3. Run Tests
```bash
cd c4-core && go test -count=1 -p 1 ./...
```
- 실패 테스트 있으면 → 원인 분석 + 수정
- Python 변경 있으면: `uv run pytest tests/ -x`
- C5 변경 있으면: `cd c5 && go test ./...`

### 4. Verify Worker Output (C4 workflow 사용 시)
- `c4_status`로 모든 태스크 상태 확인
- 각 완료 태스크의 `commit_sha` 존재 여부 확인
- `git diff` 또는 `git log`로 실제 코드 변경 확인
- commit_sha 없는 완료 태스크 → 경고 보고

### 5. Install Binary
```bash
cd c4-core && go build -o ~/.local/bin/cq ./cmd/c4/
```
- `cp` 복사 금지 (macOS ARM64 코드 서명 무효화)

### 6. Update Documentation
- 변경된 기능에 해당하는 문서 업데이트 (AGENTS.md, README.md 등)
- 테스트 수, LOC, 도구 수 등 수치 변경 시 AGENTS.md 반영
- MEMORY.md에 주요 변경 기록

### 7. Learn & Record
- `c4_knowledge_record`로 이번 세션의 인사이트 기록
- 반복될 수 있는 실수 패턴 → MEMORY.md에 추가

### 7.5. Auto-Distill (조건부)
```python
# 축적된 knowledge가 5건 이상이면 자동 distill 수행
stats = c4_knowledge_stats()
if stats.total_docs >= 5:
    c4_knowledge_distill(dry_run=False)
    # 유사 experiment 클러스터에서 pattern을 자동 추출
    # 추출된 패턴은 다음 /c4-plan에서 pattern_suggest로 반환됨
```
Cursor에서는 수동으로 `c4_knowledge_distill` 호출하거나 건너뜁니다.

### 8. Git Commit
- `git status` → 변경 파일 확인
- `git diff` → 변경 내용 검토
- Conventional commit message 작성 (feat/fix/docs/refactor)
- 커밋 생성 (push는 사용자 요청 시에만)
- 완료 후: `c4_phase_lock_release(phase="finish")` 호출

### 9. Release Notes (c4-release)

커밋 완료 후 자동으로 `/c4-release`를 실행합니다.

```
/c4-release
```

- 마지막 태그 이후 커밋 분석 → CHANGELOG.md 업데이트
- 버전 bump 제안 (Major/Minor/Patch)
- CHANGELOG.md 커밋 + 로컬 태그 생성까지만 수행
- **push는 사용자가 직접 실행** (`git push origin main && git push origin vX.Y.Z`)
- `--no-release` 플래그 명시 시 생략 가능

## Rules
- 단계를 건너뛰지 않는다
- 각 단계 완료 후 상태 보고
- Build/Test 실패 시 다음 단계 진행 금지
- Binary 설치 후 "세션 재시작 필요" 안내
