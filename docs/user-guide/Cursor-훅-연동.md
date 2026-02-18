# Cursor에서의 훅 사용 현황

Claude Code에서 쓰는 훅 설정과 Cursor에서 실제로 적용되는 범위를 정리한 문서.

## 1. Git 훅 (공통)

**위치**: `.git/hooks/`  
**적용**: 터미널/Claude Code/Cursor 등 **모든** `git commit` / `git push` 호출에 동일 적용.

| 훅 | 역할 |
|----|------|
| `pre-commit` | C4 프로젝트일 때 `uv run ruff check . --fix` 실행, 실패 시 커밋 거부 |
| `commit-msg` | 커밋 메시지에 Task ID (`[T-XXX-N]` 등) 없으면 경고(strict 모드면 거부) |
| `post-commit` | 커밋 후 `.c4/events/git-<sha>.json` 이벤트 파일 생성 (daemon 연동) |

→ **Cursor에서도 동일하게 동작** (에디터와 무관).

---

## 2. Claude Code 전용 훅

### 2.1 `.claude/hooks.json`

**적용**: Claude Code(Everything Claude Code)에서만 사용. **Cursor는 이 파일을 읽지 않음.**

| 구간 | 훅 | 설명 |
|------|-----|------|
| PreToolUse | security-check-before-commit | `git commit` 전 시크릿 스캔 |
| PreToolUse | prevent-force-push-main | `git push --force` to main 차단 |
| PostToolUse | auto-lint-python | `.py` Edit/Write 후 ruff check + format |
| PostToolUse | auto-lint-typescript | `.ts`/`.tsx` Edit/Write 후 eslint + prettier |
| PostToolUse | type-check-typescript | `.ts`/`.tsx` 후 `tsc --noEmit` |
| PostToolUse | auto-vet-go | `.go` Edit/Write 후 `go vet` |
| PostToolUse | warn-console-log | JS/TS에 `console.log` 있으면 경고 |
| Stop | final-cleanup-check | 세션 종료 시 `TODO: remove` 검사 |

→ Cursor에서는 **적용되지 않음**.

### 2.2 `.claude/settings.json` 안의 `hooks`

**적용**: Cursor는 “Third-party skills” 사용 시 **`.claude/settings.json`** 에서만 훅을 로드함.  
(참고: [Third Party Hooks](https://cursor.com/docs/agent/third-party-hooks))

| Claude Code 훅 | Cursor 매핑 | Cursor 지원 |
|----------------|-------------|-------------|
| PreToolUse | preToolUse | 예 |
| PostToolUse | postToolUse | 예 |
| Stop | stop | 예 |
| PermissionRequest | (없음) | **아니오** |

현재 `.claude/settings.json`에 정의된 훅:

- **PermissionRequest**: `permission-reviewer.py` (Bash/Read/Edit/Write 등)  
  → Cursor에서는 **PermissionRequest 미지원**이라 **실행되지 않음**.
- **PostToolUse** (Edit|Write):
  - `.go` 수정 시 `go build ./...`
  - `.py` 수정 시 `python -m py_compile`  
  → Cursor에서 **postToolUse로 매핑 가능**. Third-party skills 켜져 있으면 **이 부분만** Cursor에서도 동작할 수 있음.

**요약**: Cursor가 쓰는 건 **settings.json** 안의 훅뿐이고, **hooks.json** 전체(시크릿/force-push/ruff/prettier/go vet/console.log/cleanup)는 Cursor에서 **자동으로 쓰이지 않음**.

---

## 3. Cursor 전용 훅 (구현됨)

**`.cursor/hooks.json`** 과 **`.cursor/hooks/`** 스크립트로 `.claude/hooks.json` 동작을 Cursor 포맷으로 이전해 두었음.

| Cursor 훅 | 스크립트 | 역할 |
|-----------|----------|------|
| beforeShellExecution | `security-check-commit.sh` | `git commit` 시 시크릿 스캔, 발견 시 차단(exit 2) |
| beforeShellExecution | `block-force-push-main.sh` | `git push --force` to main/master 차단 |
| afterFileEdit | `after-edit.sh` | 확장자별: .py ruff, .ts/.tsx eslint+prettier+tsc, .go go vet, console.log 경고 |
| stop | `stop-cleanup.sh` | 세션 종료 시 `TODO: remove` 검사 |

Cursor를 재시작하면 프로젝트 훅이 로드됨. 훅 로그는 Output 패널 → Hooks에서 확인 가능.

## 4. (참고) Third-party skills

- **Settings → Features → Third-party skills** 를 켜면 `.claude/settings.json`의 PostToolUse(go build / py_compile)도 Cursor에서 적용될 수 있음.
- Cursor 전용 훅은 위 `.cursor/hooks.json` 만 사용해도 동일한 자동화가 동작함.

---

## 5. 요약 표

| 훅 소스 | Claude Code | Cursor |
|---------|-------------|--------|
| `.git/hooks/*` | 사용 | 사용 (동일) |
| `.claude/settings.json` → hooks | 사용 | Third-party skills 켜면 PostToolUse 등 매핑 가능 |
| `.claude/settings.json` → PermissionRequest | 사용 | 미지원 |
| `.claude/hooks.json` | 사용 | **미로드** (설정 파일 자체를 읽지 않음) |

**정리**:  
- Git 훅은 Cursor에서도 그대로 사용됨.  
- Claude Code용 “풀 세트” 훅은 **.claude/hooks.json** 에만 있고, Cursor는 이 파일을 사용하지 않으므로, Cursor에서 동일한 정책을 쓰려면 **.cursor/hooks.json** 에 맞춰 별도로 정의해야 함.
