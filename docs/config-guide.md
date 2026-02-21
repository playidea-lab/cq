# Config Guide

> CQ 생태계의 설정 파일 사용 가이드 (C4 + C5)

---

## C4 config.yaml

C4 Engine의 설정 파일은 `.c4/config.yaml`에 위치합니다.

### 전체 필드 설명

```yaml
# 프로젝트 기본 설정
project_id: "my-project"
default_branch: "main"
work_branch_prefix: "c4/w-"

# 경제 모드 (모델 비용 최적화)
economic_mode:
  enabled: false
  preset: "standard"          # standard | economic | quality | speed
  model_routing:
    implementation: "sonnet"
    review: "opus"
    checkpoint: "opus"
    scout: "haiku"
    debug: "haiku"
    planning: "sonnet"

# 클라우드 (Supabase) 연동
cloud:
  enabled: false
  url: ""                     # Supabase 프로젝트 URL
  anon_key: ""                # C4_CLOUD_ANON_KEY 환경변수로도 설정 가능
  project_id: ""
  bucket_name: "c4-drive"
  oauth_timeout: 120          # 초 단위

# LLM 게이트웨이
# API 키: cq secret set openai.api_key (권장) 또는 api_key_env / api_key 필드
# 키 해석 우선순위: api_key > api_key_env > ~/.c4/secrets.db
llm_gateway:
  enabled: false
  default: "openai"
  cache_by_default: true
  providers:
    openai:
      enabled: true
      default_model: "gpt-4o-mini"
      # cq secret set openai.api_key 로 저장 시 자동 조회
    anthropic:
      enabled: false
      # cq secret set anthropic.api_key 로 저장 시 자동 조회

# Worktree 설정
worktree:
  enabled: true
  auto_cleanup: true

# 검증 명령어
validation:
  lint: ""
  unit: ""

# C3 EventBus 연동
eventbus:
  enabled: false
  auto_start: false
  socket_path: ""
  data_dir: ""

# EventSink (C5→C4 이벤트 수신)
# C5 Hub에서 발행하는 이벤트를 C4 로컬 EventBus로 중계합니다.
eventsink:
  enabled: false              # true로 설정하면 이벤트 수신 활성화
  port: 4141                  # C5_EVENTBUS_URL이 가리킬 포트
  token: ""                   # 인증 토큰 (C5 --eventbus-token과 일치해야 함)

# Permission Reviewer (권한 자동 승인)
# model 모드 사용 시 api_key_env 또는 api_key 로 Anthropic 키 설정
permission_reviewer:
  enabled: false
  model: "haiku"
  api_key_env: "ANTHROPIC_API_KEY"  # 환경변수 참조 (hook 프로세스 특성상 secrets.db 미지원)
  fail_mode: "ask"            # ask | allow
  timeout: 10                 # API 호출 타임아웃 (초)

# 기타
review_as_task: false
checkpoint_as_task: false
```

### eventsink 섹션

C5 Hub가 작업 완료/실패 이벤트를 C4로 전달하는 HTTP 엔드포인트입니다.

| 환경변수 | 설명 | 우선순위 |
|----------|------|---------|
| `C4_EVENTSINK_PORT` | 포트 오버라이드 (`0`이면 비활성) | 환경변수 > config.yaml |
| `C4_EVENTSINK_TOKEN` | 인증 토큰 오버라이드 | 환경변수 > config.yaml |

```yaml
# .c4/config.yaml
eventsink:
  enabled: true
  port: 4141
  token: "secret-token"
```

---

## C5 c5.yaml

C5 Hub 서버의 설정 파일은 기본적으로 `~/.config/c5/c5.yaml`에 위치합니다.

### 기본 설정 생성

```bash
c5 serve --print-config > ~/.config/c5/c5.yaml
```

### 전체 필드 설명

```yaml
# C5 Hub Server configuration
# Default path: ~/.config/c5/c5.yaml

server:
  # HTTP 서버 바인드 호스트
  host: "0.0.0.0"
  # 리슨 포트
  port: 8585

eventbus:
  # C3 EventBus (또는 C4 EventSink) 베이스 URL.
  # 비어 있으면 이벤트 발행 비활성화.
  url: ""
  # EventBus 인증 Bearer 토큰
  token: ""

storage:
  # C5 데이터 로컬 저장 디렉토리
  path: "~/.local/share/c5"
```

### 로딩 우선순위

설정값은 다음 순서로 적용됩니다 (뒤쪽이 앞쪽을 덮어씀):

```
1. c5.yaml 파일 (기본값 포함)
2. 환경변수 override
3. CLI 플래그 override (명시적으로 지정한 경우만)
```

### 환경변수

| 환경변수 | c5.yaml 필드 | 설명 |
|----------|-------------|------|
| `C5_EVENTBUS_URL` | `eventbus.url` | EventBus/EventSink URL |
| `C5_EVENTBUS_TOKEN` | `eventbus.token` | 인증 토큰 |
| `C5_STORAGE_PATH` | `storage.path` | 데이터 저장 경로 |
| `C5_API_KEY` | (CLI `--api-key`) | C5 API 인증 키 |

### CLI 플래그

```
c5 serve [flags]

Flags:
  --config string         설정 파일 경로 (기본: ~/.config/c5/c5.yaml)
  --print-config          예시 설정 YAML 출력 후 종료
  --port int              HTTP 리슨 포트 (설정 파일 override)
  --db string             SQLite DB 경로 (기본: ./c5.db)
  --api-key string        API 인증 키 (기본: C5_API_KEY 환경변수)
  --eventbus-url string   EventBus URL (설정 파일/환경변수 override)
  --eventbus-token string EventBus 토큰 (설정 파일/환경변수 override)
```

**주의**: `--port`, `--eventbus-url`, `--eventbus-token`은 명시적으로 지정한 경우에만 설정 파일/환경변수를 override합니다.

---

## C5_EVENTBUS_URL → c5.yaml 이전 가이드

기존에 환경변수로 관리하던 설정을 `c5.yaml`로 이전하는 방법입니다.

### 기존 방식 (환경변수)

```bash
export C5_EVENTBUS_URL=http://localhost:4141
export C5_EVENTBUS_TOKEN=my-secret
c5 serve
```

### 새 방식 (config 파일)

```bash
# 1. 설정 디렉토리 생성
mkdir -p ~/.config/c5

# 2. 기본 설정 파일 생성
c5 serve --print-config > ~/.config/c5/c5.yaml

# 3. eventbus 섹션 편집
# ~/.config/c5/c5.yaml
# eventbus:
#   url: "http://localhost:4141"
#   token: "my-secret"

# 4. 환경변수 없이 실행
c5 serve
```

### 하이브리드 방식 (config 파일 + 환경변수 override)

민감한 값(토큰)은 환경변수로 관리하고, 나머지는 config 파일에 저장할 수 있습니다.

```yaml
# ~/.config/c5/c5.yaml
eventbus:
  url: "http://localhost:4141"
  token: ""   # C5_EVENTBUS_TOKEN 환경변수로 주입
```

```bash
export C5_EVENTBUS_TOKEN=my-secret
c5 serve
```

---

## C4 + C5 연동 설정 예시

C5 Hub 이벤트를 C4 EventSink로 전달하는 전체 설정입니다.

### C4 측 (.c4/config.yaml)

```yaml
eventsink:
  enabled: true
  port: 4141
  token: "shared-secret"
```

### C5 측 (~/.config/c5/c5.yaml)

```yaml
eventbus:
  url: "http://localhost:4141"
  token: "shared-secret"
```

### 실행 순서

```bash
# 1. C4 MCP 서버 시작 (Claude Code가 자동으로 시작)
# 2. C5 서버 시작
c5 serve
```

발행되는 이벤트: `hub.job.completed`, `hub.job.failed`, `hub.worker.started`, `hub.worker.offline`
