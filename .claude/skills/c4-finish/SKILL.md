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

### 3.5. Spec Test Mapping (동작정의서 검증)

> spec.md에 시나리오가 있으면, 테스트 결과와 매핑하여 동작정의서를 갱신한다.

```python
# 현재 feature의 spec 파일 탐색
import glob
spec_files = glob.glob("docs/specs/*.md")
specs_with_scenarios = []
for sf in spec_files:
    content = c4_read_file(path=sf)
    if "## 동작 시나리오" in content:
        specs_with_scenarios.append(sf)

if not specs_with_scenarios:
    print("⏭️  동작 시나리오 없음 — spec mapping skip")
    → Step 4
```

**매핑 프로세스** (spec 파일이 있을 때):

1. spec.md에서 시나리오 ID(S1, S2...) + VERIFY 조건 추출
2. 테스트 실행 결과에서 테스트 이름 목록 추출
3. 매핑: 테스트 이름에 시나리오 ID 포함(TestXxx_S1_Yyy) → 자동 매칭. 불일치 → LLM 추론
4. 테스트 매핑 테이블 갱신:

```markdown
## 테스트 매핑
| 시나리오 | 테스트 | 상태 |
|---------|--------|------|
| S1 | TestTransfer_S1_HappyPath | ✅ PASS |
| S2 | TestTransfer_S2_Offline | ✅ PASS |
| S3 | (미구현) | ⚠️ NO TEST |
| — | TestTransfer_LargeFile | 🆕 UNDOCUMENTED |
```

상태 아이콘:
- ✅ PASS — 시나리오 + 테스트 존재 + 통과
- ⚠️ NO TEST — 시나리오 있지만 테스트 없음
- ❌ FAIL — 시나리오 + 테스트 존재 + 실패
- 🆕 UNDOCUMENTED — 테스트 있지만 시나리오 없음

**Soft gate**: 미매핑 경고만 출력. blocking 아님.

```python
unmapped = [s for s in scenarios if s.status == "NO TEST"]
if unmapped:
    print(f"⚠️  시나리오 {len(unmapped)}건 테스트 미구현: {', '.join(s.id for s in unmapped)}")

undocumented = [t for t in tests if t.status == "UNDOCUMENTED"]
if undocumented:
    print(f"🆕  미문서화 테스트 {len(undocumented)}건 발견")
```

**에디터 열기** (갱신 후):

```python
import shutil
if shutil.which("code"):
    Bash(f"code '{spec_path}'")
elif shutil.which("open"):
    Bash(f"open '{spec_path}'")

print(f"""
📋 동작정의서 갱신 완료: {spec_path}
   v0 시나리오 ↔ 실제 테스트 결과를 확인해주세요.
   ⚠️ = 테스트 추가 필요, 🆕 = 시나리오 문서화 필요
""")
```

**갭 발견 시 종료 판단**:

```python
if unmapped or undocumented:
    # 사람이 에디터에서 확인 후 판단
    # (a) spec.md를 수정하지 않고 닫음 → "현 상태로 수용" → Step 4로 진행
    # (b) spec.md에서 시나리오 추가/삭제 후 닫음 → 변경 반영, Step 4로 진행
    #     (추가 구현은 별도 태스크로 — 이번 finish에서는 commit)
    spec_after = c4_read_file(path=spec_path)
    if spec_before != spec_after:
        print("📝 spec 수정 감지 — 갱신 반영 완료")
else:
    print("✅ 전 시나리오 매핑 완료 — 작업 종료 조건 충족")
```

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

### 7.6. Persona Pattern Collection (자동)
커밋 후 git diff에서 코딩 패턴을 자동 추출하여 raw_patterns.json에 축적.
```python
# 마지막 태그 또는 이전 커밋과의 diff에서 패턴 학습
last_tag = Bash("git describe --tags --abbrev=0 2>/dev/null || echo HEAD~5").strip()
c4_persona_learn_from_diff(commit_range=f"{last_tag}..HEAD")
# 실패 시 non-fatal — 패턴 수집은 best-effort
```

### 7.7. POP Extract
세션 요약을 `c4_pop_extract(content=session_summary)`로 주입. 실패 시 non-fatal.

### 8. Git Commit
Conventional commit. 완료 후 `c4_phase_lock_release(phase="finish")`.
알림: `c4_notify(message='구현 완료', event='finish.complete')`

### 9. Release
```
/c4-release
```
CHANGELOG.md → 태그 → push (기본). `--no-push`로 로컬만 가능.

## Rules
- 단계를 건너뛰지 않는다
- Build/Test 실패 시 다음 단계 진행 금지
- Binary 설치 후 "세션 재시작 필요" 안내
