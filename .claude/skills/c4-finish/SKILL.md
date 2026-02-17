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

### 1. Verify Build
```bash
cd c4-core && go build ./... && go vet ./...
```
- Build/vet 실패 시 → 수정 후 재시도, 통과할 때까지 다음 단계 진행 금지

### 2. Run Tests
```bash
cd c4-core && go test -count=1 -p 1 ./...
```
- 실패 테스트 있으면 → 원인 분석 + 수정
- Python 변경 있으면: `uv run pytest tests/ -x`
- C5 변경 있으면: `cd c5 && go test ./...`

### 3. Verify Worker Output (C4 workflow 사용 시)
- `c4_status`로 모든 태스크 상태 확인
- 각 완료 태스크의 `commit_sha` 존재 여부 확인
- `git diff` 또는 `git log`로 실제 코드 변경 확인
- commit_sha 없는 완료 태스크 → 경고 보고

### 4. Install Binary
```bash
cd c4-core && go build -o ~/.local/bin/c4 ./cmd/c4/
```
- `cp` 복사 금지 (macOS ARM64 코드 서명 무효화)

### 5. Update Documentation
- 변경된 기능에 해당하는 문서 업데이트 (AGENTS.md, README.md 등)
- 테스트 수, LOC, 도구 수 등 수치 변경 시 AGENTS.md 반영
- MEMORY.md에 주요 변경 기록

### 6. Learn & Record
- `c4_knowledge_record`로 이번 세션의 인사이트 기록
- 반복될 수 있는 실수 패턴 → MEMORY.md에 추가

### 7. Git Commit
- `git status` → 변경 파일 확인
- `git diff` → 변경 내용 검토
- Conventional commit message 작성 (feat/fix/docs/refactor)
- 커밋 생성 (push는 사용자 요청 시에만)

## Rules
- 단계를 건너뛰지 않는다
- 각 단계 완료 후 상태 보고
- Build/Test 실패 시 다음 단계 진행 금지
- Binary 설치 후 "세션 재시작 필요" 안내
