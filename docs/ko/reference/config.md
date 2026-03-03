# 설정 레퍼런스

설정 파일 위치: `~/.c4/config.yaml` (전역) 또는 `.c4/config.yaml` (프로젝트별).

## 설정이 필요한 경우

- **solo 티어** — 설정 불필요. CQ가 바로 작동합니다.
- **connected / full 티어** — 팀 또는 조직에서 설정을 제공합니다. `~/.c4/config.yaml`에 위치시키세요.

아래 섹션은 참고용 사용 가능 옵션을 문서화합니다.

## 섹션

### `hub` — C5 Hub (full 티어)

```yaml
hub:
  enabled: true
  url: "http://localhost:8585"
  api_key: ""   # 사용: cq secret set hub.api_key <value>
```

### `llm_gateway` — LLM 프로바이더 (connected / full 티어)

```yaml
llm_gateway:
  default_provider: anthropic
  providers:
    anthropic:
      enabled: true
      default_model: claude-sonnet-4-6
    openai:
      enabled: false
    ollama:
      enabled: true
      base_url: "http://localhost:11434"
```

::: tip API 키
API 키를 설정 파일에 넣지 마세요. 시크릿 스토어를 사용하세요:
```sh
cq secret set anthropic.api_key sk-ant-...
```
:::

### `permission_reviewer` — 셸 명령 보안 훅

```yaml
permission_reviewer:
  enabled: true
  mode: hook        # "hook" (정규식) 또는 "model" (LLM 리뷰)
  auto_approve: true
  allow_patterns: []
  block_patterns: []
```

### `serve` — 백그라운드 서비스

```yaml
serve:
  hub:
    enabled: false       # C5 Hub를 서브프로세스로 시작 (full 티어)
    binary: "c5"         # PATH에서 찾을 바이너리명
    port: 8585           # c5에 --port로 전달할 포트
    args: []             # 추가 CLI 인자
  eventbus:
    enabled: false
  gpu:
    enabled: false
  agent:
    enabled: false   # cloud.url + cloud.anon_key 필요
  eventsink:
    enabled: false
    port: 4141       # C5 Hub → C4 이벤트 수신 엔드포인트
    token: ""
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 stale 판정
    interval_seconds: 60
  ssesubscriber:
    enabled: false   # full 티어 전용 (C5 Hub + C3 EventBus 빌드 태그 필요)
```

`serve.hub.enabled: true`일 때 `cq serve`가 `c5` 바이너리를 자식 프로세스로 자동 시작하여 라이프사이클을 관리합니다:
- `cq serve` 실행 시 시작 (PATH에서 바이너리를 찾지 못하면 gracefully skip)
- `http://127.0.0.1:{port}/v1/health`에서 몇 초마다 health check
- `cq serve` 종료 시 깔끔하게 중지 (SIGTERM → 5초 대기 → SIGKILL)

### `worktree`

```yaml
worktree:
  auto_cleanup: true   # 태스크 submit 후 워크트리 자동 삭제
```

### `validation` — 빌드 및 테스트 명령

자동 감지된 명령을 오버라이드합니다:

```yaml
validation:
  build_command: "make build"    # 기본값: 언어별 자동 감지
  test_command: "make test"      # 기본값: 언어별 자동 감지
```

자동 감지: Go → `go build ./...`, Python → `uv run pytest`, Node → `npm run build`, Rust → `cargo build`.

### `cloud` — Supabase (connected / full 티어)

조직에서 설정합니다. 이 섹션을 직접 수정할 필요가 없습니다.

```yaml
cloud:
  url: "https://xxxx.supabase.co"
  anon_key: ""
```
