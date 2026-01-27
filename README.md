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

### 슬래시 명령어

| 명령어 | 설명 |
|--------|------|
| `/c4-init` | 현재 디렉토리에서 C4 초기화 |
| `/c4-status` | 프로젝트 상태 및 큐 확인 |
| `/c4-plan` | 문서 분석 후 태스크 생성 |
| `/c4-run` | 실행 시작 (자동 루프) |
| `/c4-stop` | 실행 중지 |
| `/c4-validate` | 검증 실행 (lint, test) |
| `/c4-submit` | 완료된 태스크 제출 |
| `/c4-checkpoint` | 체크포인트 리뷰 |
| `/c4-add-task` | 새 태스크 추가 |

---

## 설정

### 기본 설정 (.c4/config.yaml)

```yaml
project_id: my-project
default_branch: main
work_branch_prefix: "c4/w-"

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
    └── bundles/           # 체크포인트 번들
```

---

## 핵심 기능

| 기능 | 설명 |
|------|------|
| **State Machine** | 구조화된 워크플로우 관리 (v0.6.6 최적화) |
| **Multi-Platform** | Claude Code, Cursor, Gemini CLI, Codex, OpenCode 완벽 지원 |
| **Multi-Worker** | SQLite WAL 기반 병렬 실행 및 동기화 |
| **Agent Routing** | GraphRouter 기반 스킬 매칭 및 에이전트 체이닝 |
| **Git Automation** | 자동 커밋, 체크포인트 태깅, 롤백 시스템 |
| **Advanced MCP** | 코드 분석, 시맨틱 검색, 호출 그래프 분석 등 30+ 도구 |
| **Team Collab** | Supabase 기반 상태 공유 및 팀 관리 (Phase 6) |

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
