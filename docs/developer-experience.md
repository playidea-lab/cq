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
c4 status

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

## 관련 문서

- [Getting Started](./getting-started/README.md)
- [User Guide](./user-guide/README.md)
- [API Reference](./api/README.md)
