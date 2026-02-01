# C4 - AI Project Orchestration System

C4 (Codex-Claude-Completion Control)는 AI 에이전트가 계획부터 완료까지 프로젝트를 자동으로 실행할 수 있게 해주는 오케스트레이션 시스템입니다.

## 빠른 시작 (3분)

### 1. 설치

```bash
# 원라인 설치 (uv 자동 설치 포함)
curl -fsSL https://git.pilab.co.kr/pi/c4/raw/main/install-remote.sh | bash
```

설치 스크립트가 자동으로:
- **uv 자동 설치** (없는 경우)
- `~/.c4`에 C4 설치
- 의존성 설치 (`uv sync`)
- 글로벌 `c4` 명령어 생성
- **플랫폼 자동 감지** (Claude Code, Cursor)
- 슬래시 명령어 등록 (Claude Code: `~/.claude/commands/`, Cursor: `~/.cursor/commands/`)
- Cursor MCP 서버 설정 (`~/.cursor/mcp.json`)

### 2. 프로젝트 시작

```bash
cd /your/project
c4
```

이 명령어 하나로 C4가 초기화되고 기본 IDE(Claude Code 또는 Cursor)가 시작됩니다.

### 3. 작업 실행

Claude Code 또는 Cursor에서:

```
/c4-plan       # 문서 분석 → 태스크 생성
/c4-run        # 자동 실행 시작
/c4-status     # 진행 상황 확인
```

---

## 핵심 개념

### 워크플로우

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ↔ CHECKPOINT → COMPLETE
```

> **v0.6.6 신규**: `/c4-run`이 **Smart Auto Mode**를 기본으로 사용합니다. 태스크 의존성을 분석하여 자동으로 병렬 Worker를 스폰합니다.

| 상태 | 설명 |
|------|------|
| **INIT** | 프로젝트 초기화 |
| **DISCOVERY** | 요구사항 수집 (EARS 패턴) |
| **DESIGN** | 아키텍처 설계 |
| **PLAN** | 태스크 생성 |
| **EXECUTE** | 워커가 태스크 실행 |
| **CHECKPOINT** | Supervisor 리뷰 |
| **COMPLETE** | 프로젝트 완료 |

### 태스크 라이프사이클 (Review-as-Task)

```
T-001-0 (구현) → R-001-0 (리뷰)
                    ↓
          ├─ APPROVE → 완료
          └─ REQUEST_CHANGES → T-001-1 → R-001-1 → ...
```

| 태스크 유형 | ID 형식 | 설명 |
|-------------|---------|------|
| **Implementation** | T-XXX-N | 구현 태스크 (N=버전) |
| **Review** | R-XXX-N | 코드 리뷰 태스크 |
| **Checkpoint** | CP-XXX | Phase 통합 검증 |

- 구현 태스크 완료 시 리뷰 태스크 자동 생성
- REQUEST_CHANGES 시 다음 버전 태스크 생성
- Phase 내 모든 리뷰 APPROVE 시 체크포인트 태스크 생성

### Economic Mode (비용 최적화)

태스크별 모델 선택으로 비용을 최적화합니다:

| 모델 | 용도 | 예시 |
|------|------|------|
| **opus** | 복잡한 아키텍처, 설계 결정 | 시스템 설계, 어려운 버그 |
| **sonnet** | 일반 구현, 리뷰 (기본값) | 기능 구현, 코드 리뷰 |
| **haiku** | 단순 반복, 린트 수정 | 포맷팅, 간단한 수정 |

```yaml
# 태스크 추가 시 모델 지정
c4_add_todo:
  task_id: "T-001-0"
  title: "아키텍처 설계"
  model: "opus"        # 복잡한 작업은 opus

c4_add_todo:
  task_id: "T-002-0"
  title: "린트 수정"
  model: "haiku"       # 단순 작업은 haiku
```

```bash
# Smart Auto Mode (권장) - 자동으로 최적 Worker 수 결정
/c4-run

# 수동 모드 (필요 시) - 모델별 Worker 수 지정
/c4-run --parallel 4  # 4개 Worker
```

### 슬래시 명령어

| 명령어 | 설명 |
|--------|------|
| `/c4-init` | 현재 디렉토리에서 C4 초기화 |
| `/c4-status` | 프로젝트 상태 및 큐 확인 |
| `/c4-plan` | 문서 분석 후 태스크 생성 |
| `/c4-run` | 실행 시작 (**Smart Auto Mode**: 자동 병렬화) |
| `/c4-stop` | 실행 중지 |
| `/c4-validate` | 검증 실행 (lint, test) |
| `/c4-submit` | 완료된 태스크 제출 |
| `/c4-checkpoint` | 체크포인트 리뷰 |
| `/c4-add-task` | 새 태스크 추가 |
| `/c4-swarm` | (Deprecated) 수동 병렬 Worker 스폰 - `/c4-run` 사용 권장 |

---

## 개발자 경험 (DX)

C4는 개발 워크플로우를 향상시키는 여러 기능을 제공합니다:

### Git Hooks 통합

자동 설치되는 Git hooks:
- **pre-commit**: Lint 검증, staged 파일 체크
- **commit-msg**: Task ID 형식 검증 (`[T-XXX-N] message`)
- **post-commit**: C4 이벤트 로깅

### IDE 통합 (LSP) & 코드 분석

C4는 내장 LSP 서버와 코드 분석 도구를 제공합니다:

**LSP 기능**:
- **Hover**: `T-XXX` 위에 마우스를 올리면 태스크 정보 표시
- **Completion**: `T-` 입력 시 사용 가능한 태스크 자동완성
- **Definition**: 심볼 정의 위치로 이동
- **References**: 모든 참조 찾기
- **Document/Workspace Symbols**: 심볼 검색

**코드 분석 MCP 도구** (Serena 대체):

| 도구 | 설명 |
|------|------|
| `c4_find_symbol` | 심볼 검색 (클래스, 함수, 메서드) |
| `c4_get_symbols_overview` | 파일 내 심볼 개요 |
| `c4_read_file` | 파일 읽기 (라인 범위 지원) |
| `c4_find_file` | 파일 검색 (glob 패턴) |
| `c4_search_for_pattern` | 정규식 패턴 검색 |
| `c4_replace_symbol_body` | 심볼 본문 교체 |
| `c4_insert_before_symbol` | 심볼 앞에 코드 삽입 |
| `c4_insert_after_symbol` | 심볼 뒤에 코드 삽입 |
| `c4_rename_symbol` | 심볼 이름 변경 (전체 참조 업데이트) |

> **Note**: C4의 코드 분석 도구는 Serena MCP 서버를 완전히 대체합니다.
> 추가 장점: Task ID 통합, C4 프로젝트 컨텍스트 자동 인식

### Health Monitoring (Daemon)

백그라운드 상태 모니터링:
- MCP 서버 연결 상태
- Worker 상태 추적
- 태스크 큐 모니터링

### 초기화 옵션

```bash
# 전체 기능 (기본)
c4 init

# 선택적 비활성화
c4 init --no-git-hooks    # Git hooks 제외
c4 init --no-lsp          # LSP 서버 제외
c4 init --no-daemon       # Daemon 모드 제외
```

자세한 내용: [Developer Experience Guide](./docs/developer-experience.md)

---

## 설정

### 기본 설정 (.c4/config.yaml)

```yaml
project_id: my-project
default_branch: main
work_branch_prefix: "c4/w-"

# Worktree 격리 (v0.6.6 신규)
worktree:
  enabled: true              # Worker별 독립 worktree 사용
  base_branch: main          # worktree 기준 브랜치
  completion_action: merge   # pr: PR 생성, merge: 직접 병합

# 검증 명령어
validation:
  commands:
    lint: "uv run ruff check"
    unit: "uv run pytest tests/unit"
  required: ["lint", "unit"]

# 체크포인트 (자동 생성됨)
checkpoints:
  - id: CP-REVIEW
    name: "코드 리뷰"
    required_validations: ["lint"]
    auto_approve: true      # 기본값: AI 자동 리뷰
  - id: CP-FINAL
    name: "최종 검토"
    required_validations: ["lint", "unit"]
    auto_approve: false     # 사람 리뷰 필수 (/c4-checkpoint)

# Review-as-Task (기본: 활성화)
review_as_task: true        # 리뷰를 태스크로 생성
checkpoint_as_task: true    # 체크포인트를 태스크로 처리
max_revision: 3             # 최대 수정 횟수
```

### 플랫폼 설정

C4는 여러 IDE를 지원합니다. 설치 시 자동 감지되며, 수동 변경도 가능합니다:

```bash
# 현재 설정 확인
c4 config

# 글로벌 기본값 변경
c4 config --global --platform gemini

# 프로젝트별 설정
c4 config --platform claude
```

**지원 플랫폼**: Claude Code, Cursor, Gemini CLI, Codex CLI, OpenCode

> **플랫폼별 팁**:
> - **Claude Code**: 기준 플랫폼, 모든 슬래시 커맨드 및 훅 지원.
> - **Cursor**: Composer 모드 권장. 25회 Tool Call 제한 유의 (MAX 모드 사용 권장).
> - **Gemini CLI**: MCP 도구 호출에 최적화. `.gemini/commands/`에서 슬래시 커맨드 확인 가능.
> - **Codex CLI**: 자동화 트리거 및 워커 루프 실행에 최적화.

### LLM Provider 설정

C4는 다양한 LLM Provider를 지원합니다:

```yaml
# 기본값: Claude Code (사용자 구독)
llm:
  model: claude-cli

# OpenAI
llm:
  model: gpt-4o
  api_key_env: OPENAI_API_KEY

# Anthropic API (별도 API 키)
llm:
  model: claude-3-opus-20240229
  api_key_env: ANTHROPIC_API_KEY

# Azure OpenAI
llm:
  model: azure/gpt-4o-deployment
  api_base: https://my-resource.openai.azure.com
  api_key_env: AZURE_OPENAI_API_KEY

# Ollama (로컬)
llm:
  model: ollama/llama3
  api_base: http://localhost:11434

# ZhipuAI (GLM)
llm:
  model: zhipuai/glm-4
  api_key_env: ZHIPUAI_API_KEY
```

**지원 Provider**: OpenAI, Anthropic, Azure, Ollama, Bedrock, Groq, Together, ZhipuAI 등 100+ ([전체 목록](https://docs.litellm.ai/docs/providers))

### GitLab/GitHub 통합

C4는 GitLab과 GitHub의 MR/PR 웹훅을 받아 AI 코드 리뷰를 자동 실행할 수 있습니다.

**GitLab 환경 변수**:
```bash
# 인증 (택일)
GITLAB_PRIVATE_TOKEN=<personal_access_token>
GITLAB_OAUTH_TOKEN=<oauth_token>

# 웹훅 검증 (권장)
GITLAB_WEBHOOK_SECRET=<webhook_secret>

# Self-hosted GitLab (선택)
GITLAB_URL=https://gitlab.example.com
```

**GitHub 환경 변수**:
```bash
GITHUB_APP_ID=<app_id>
GITHUB_APP_PRIVATE_KEY=<private_key>
GITHUB_WEBHOOK_SECRET=<webhook_secret>
```

**웹훅 엔드포인트**:
- GitLab: `POST /webhooks/gitlab`
- GitHub: `POST /webhooks/github`

---

## 프로젝트 구조

```
your-project/
└── .c4/
    ├── c4.db              # SQLite 데이터베이스
    ├── config.yaml        # 프로젝트 설정
    ├── specs/             # EARS 요구사항
    ├── designs/           # 설계 문서
    ├── bundles/           # 체크포인트 번들
    └── worktrees/         # Worker별 격리된 작업 공간 (v0.6.6)
        └── worktree-{id}/ # 독립된 git worktree
```

---

## 핵심 기능

| 기능 | 설명 |
|------|------|
| **State Machine** | 구조화된 워크플로우 관리 (v0.6.6 최적화) |
| **Smart Auto Mode** | 태스크 의존성 분석 후 자동 병렬화 ⭐ NEW |
| **Worktree 격리** | Worker별 독립 Git worktree로 충돌 방지 ⭐ NEW |
| **Economic Mode** | 태스크별 모델 선택 (opus/sonnet/haiku) |
| **Code Analysis** | 내장 LSP + 심볼 편집 도구 (Serena 대체) |
| **Multi-Platform** | Claude Code, Cursor, Gemini CLI, Codex, OpenCode 완벽 지원 |
| **Multi-Worker** | SQLite WAL 기반 병렬 실행 및 동기화 |
| **Agent Routing** | GraphRouter 기반 스킬 매칭 및 에이전트 체이닝 |
| **Git Automation** | 자동 커밋, 체크포인트 태깅, 롤백 시스템 |
| **Advanced MCP** | 코드 분석, 시맨틱 검색, 호출 그래프 분석 등 30+ 도구 |
| **Team Collab** | Supabase 기반 상태 공유 및 팀 관리 (Phase 6) |

### Smart Auto Mode (v0.6.6)

`/c4-run` 실행 시 자동으로 병렬 Worker를 스폰합니다:

```bash
/c4-run
# 자동으로:
# 1. 태스크 의존성 분석
# 2. 병렬화 가능한 태스크 식별
# 3. 적절한 수의 Worker 스폰 (기본: 2~4)
# 4. 각 Worker는 독립된 Worktree에서 작업
```

**Worktree 격리**:
- 각 Worker는 `.c4/worktrees/worktree-{id}/`에서 독립 작업
- 브랜치 충돌 없이 병렬 개발 가능
- 체크포인트에서 자동 병합

```yaml
# .c4/config.yaml에서 활성화
worktree:
  enabled: true
  base_branch: main
  completion_action: merge  # pr 또는 merge
```

---

## 설정 및 플랫폼 가이드

### 플랫폼별 최적화 (Slash Commands)

C4는 각 플랫폼의 특성에 맞춰 슬래시 커맨드 경로를 자동 관리합니다:
- **Claude Code**: `~/.claude/commands/` (Standard)
- **Cursor**: `.cursor/commands/` (Composer-ready)
- **Gemini CLI**: `.gemini/commands/` (Native integration)

```bash
# 특정 플랫폼 설정 예시
c4 config --platform gemini
```

> **Tip**: Gemini CLI 사용 시 `/c4-status`를 먼저 입력하여 현재 단계와 다음에 할 일을 확인하세요.

---

## 개발 및 검증

C4는 3,000개 이상의 테스트 케이스로 안정성을 보장합니다.

```bash
# 테스트 실행
uv run pytest tests/ -v

# 특정 도메인 검증 (예: Gemini)
uv run python tests/debug/debug_gemini_slash_command.py
```
---

## 문서

### 시작하기

| 문서 | 설명 |
|------|------|
| [설치 가이드](docs/getting-started/설치-가이드.md) | 상세 설치 방법 |
| [빠른 시작](docs/getting-started/빠른-시작.md) | 5분 퀵스타트 |
| [예제](docs/getting-started/예제-C4-셀프호스팅.md) | C4로 C4 개발하기 |

### 사용자 가이드

| 문서 | 설명 |
|------|------|
| [워크플로우 개요](docs/user-guide/워크플로우-개요.md) | 전체 흐름 설명 |
| [명령어 레퍼런스](docs/user-guide/명령어-레퍼런스.md) | 슬래시 명령어 상세 |
| [LLM 설정](docs/user-guide/LLM-설정.md) | Multi-LLM Provider 설정 |
| [문제 해결](docs/user-guide/문제-해결.md) | FAQ 및 트러블슈팅 |

### 개발자 가이드

| 문서 | 설명 |
|------|------|
| [아키텍처](docs/developer-guide/아키텍처.md) | 시스템 구조 |
| [StateStore 확장](docs/developer-guide/StateStore-확장.md) | 커스텀 저장소 구현 |
| [SupervisorBackend 확장](docs/developer-guide/SupervisorBackend-확장.md) | 커스텀 LLM 백엔드 |

### API 레퍼런스

| 문서 | 설명 |
|------|------|
| [MCP 도구](docs/api/MCP-도구-레퍼런스.md) | 19개 MCP 도구 스펙 |
| [CHANGELOG](CHANGELOG.md) | 버전별 변경 사항 |

---

## 개발

```bash
# 테스트 실행
uv run pytest tests/ -v

# 린터
uv run ruff check c4/ tests/

# 타입 체크
uv run mypy c4/
```

---

## 라이선스

**Business Source License 1.1** (BSL)

- **무료**: 개인 사용, 평가, 비상업적 프로젝트
- **라이선스 필요**: 상업적 사용, 프로덕션 배포

자세한 내용은 [LICENSE](./LICENSE.md) 참조.
