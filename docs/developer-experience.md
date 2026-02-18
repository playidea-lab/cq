# Developer Experience (DX) Guide

> C4의 개발자 경험 향상 기능: IDE 통합, Git Hooks, Health Monitoring

---

## Overview

C4는 개발 워크플로우를 자동화하는 다양한 기능을 제공합니다:

| 기능 | 설명 | 기본값 |
|------|------|--------|
| **Git Hooks** | Pre-commit 검증, Task ID 추적 | ✅ 활성화 |
| **LSP Server** | IDE에서 hover, completion 지원 | ✅ 활성화 |
| **Daemon Mode** | 백그라운드 상태 모니터링 | ✅ 활성화 |

---

## 1. Git Hooks Integration

### 설치된 Hooks

`c4 init`을 실행하면 다음 Git hooks가 설치됩니다:

```
.git/hooks/
├── pre-commit      # Lint 검증, staged 파일 체크
├── commit-msg      # Task ID 형식 검증
└── post-commit     # C4 이벤트 로깅
```

### Pre-commit Hook

커밋 전 자동 검증:

```bash
# 실행되는 검증
uv run ruff check --fix .  # Lint 자동 수정
uv run ruff format .       # 코드 포매팅
```

**검증 실패 시:**
- 커밋이 차단됩니다
- 오류 메시지와 수정 방법이 표시됩니다
- `--no-verify`로 우회 가능 (권장하지 않음)

### Commit Message Hook

Task ID 형식을 검증합니다:

```bash
# 올바른 형식
git commit -m "[T-001-0] 기능 구현"
git commit -m "[R-001-0] 코드 리뷰 완료"

# 잘못된 형식 (경고)
git commit -m "기능 구현"  # Task ID 없음
```

### 비활성화

Git hooks 없이 초기화:

```bash
c4 init --no-git-hooks
```

---

## 2. LSP Server (IDE Integration)

### 기능

| 기능 | 설명 |
|------|------|
| **Hover** | T-XXX 위에 마우스를 올리면 태스크 정보 표시 |
| **Completion** | `T-` 입력 시 사용 가능한 태스크 ID 자동완성 |
| **Diagnostics** | 잘못된 태스크 참조 경고 |

### 사용 예시

```python
# TODO: See T-001-0 for implementation details
#       ↑ 마우스를 올리면:
#       ┌─────────────────────────────────┐
#       │ T-001-0: API 엔드포인트 구현    │
#       │ Status: in_progress             │
#       │ DoD: REST API 구현, 테스트 작성 │
#       └─────────────────────────────────┘
```

### 지원 IDE

- **VS Code**: Claude Code extension과 함께 동작
- **Cursor**: 네이티브 지원
- **기타 LSP 클라이언트**: 표준 LSP 프로토콜 지원

### 비활성화

LSP 없이 초기화:

```bash
c4 init --no-lsp
```

---

## 3. Daemon Mode (Health Monitoring)

### 기능

백그라운드에서 프로젝트 상태를 모니터링합니다:

- **MCP Server Health**: MCP 서버 연결 상태
- **Worker Status**: 활성 Worker 상태
- **Task Queue**: 대기 중인 태스크 수
- **Validation Status**: 마지막 검증 결과

### 로그 위치

```
.c4/logs/
├── daemon.log     # Daemon 로그
├── health.log     # Health check 결과
└── events/        # 이벤트 히스토리
```

### Health Check 간격

- 기본: 30초마다 체크
- 설정: `.c4/config.yaml`에서 변경 가능

```yaml
# .c4/config.yaml
daemon:
  health_check_interval: 30  # 초
  log_level: INFO
```

### 비활성화

Daemon 없이 초기화:

```bash
c4 init --no-daemon
```

---

## 4. 초기화 옵션 요약

```bash
# 전체 기능 (기본)
c4 init

# Git hooks 제외
c4 init --no-git-hooks

# LSP 제외
c4 init --no-lsp

# Daemon 제외
c4 init --no-daemon

# 모든 추가 기능 제외
c4 init --no-git-hooks --no-lsp --no-daemon

# 템플릿과 함께
c4 init --template image-classification
```

---

## 5. 환경 변수

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `C4_PROJECT_ROOT` | 프로젝트 루트 경로 | 현재 디렉토리 |
| `C4_LSP_ENABLED` | LSP 서버 활성화 | `true` |
| `C4_DAEMON_ENABLED` | Daemon 모드 활성화 | `true` |
| `C4_LOG_LEVEL` | 로그 레벨 | `INFO` |

---

## 6. 트러블슈팅

### Git Hooks가 동작하지 않음

```bash
# 권한 확인
ls -la .git/hooks/pre-commit

# 수동 재설치
c4 init --with-git-hooks
```

### LSP가 동작하지 않음

```bash
# MCP 서버 상태 확인
cq status

# .mcp.json 확인
cat .mcp.json

# Claude Code 재시작
```

### Daemon 로그 확인

```bash
# 최근 로그 확인
tail -f .c4/logs/daemon.log

# Health 상태 확인
cat .c4/logs/health.log
```

---

## 7. Worktree Mode (Parallel Isolation)

### 개요

Worktree 모드는 각 Worker가 독립된 Git Worktree에서 작업하도록 하여 파일 충돌 없이 병렬 작업을 가능하게 합니다.

| 기능 | 설명 |
|------|------|
| **독립된 작업 공간** | 각 Worker가 별도의 디렉토리에서 작업 |
| **브랜치 격리** | 각 태스크별 브랜치 자동 생성 |
| **자동 PR 생성** | 모든 태스크 완료 시 PR 자동 생성 |

### 설정

`.c4/config.yaml`에서 worktree 설정을 활성화합니다:

```yaml
# .c4/config.yaml
project_id: my-project
default_branch: main

worktree:
  enabled: true                # Worktree 모드 활성화
  base_branch: work            # 작업 기준 브랜치 (main이 아닌 별도 브랜치)
  work_dir: .c4/worktrees      # Worktree 저장 디렉토리
  auto_cleanup: true           # 태스크 완료 후 자동 정리
  completion_action: pr        # 완료 시 동작: 'pr' 또는 'merge'
```

### 워크플로우

```
main (기본 브랜치)
  │
  └── work (base_branch) ← 모든 태스크의 기준 브랜치
        │
        ├── c4/w-T-001-0 (Worker 1)
        │     └── .c4/worktrees/worker-1/
        │
        ├── c4/w-T-002-0 (Worker 2)
        │     └── .c4/worktrees/worker-2/
        │
        └── c4/w-T-003-0 (Worker 3)
              └── .c4/worktrees/worker-3/
```

### 완료 시 동작

- **`completion_action: pr`**: 모든 태스크 완료 시 `work` → `main` PR 자동 생성
- **`completion_action: merge`**: 자동으로 `work` 브랜치를 `main`에 병합

### Worktree 관리 명령어

```bash
# Worktree 상태 확인
cq status  # MCP tool: c4_worktree_status

# 특정 Worker 상태
# MCP tool: c4_worktree_status(worker_id="worker-1")

# 완료된 Worktree 정리
# MCP tool: c4_worktree_cleanup(keep_active=True)
```

### 주의사항

- Worktree 디렉토리 (`.c4/worktrees/`)는 `.gitignore`에 자동 추가됩니다
- `base_branch`가 `main`과 같으면 PR이 생성되지 않습니다
- PR 생성에는 `gh` CLI가 필요합니다 (https://cli.github.com/)

---

## 관련 문서

- [Getting Started](./getting-started/README.md)
- [User Guide](./user-guide/README.md)
- [API Reference](./api/README.md)
