# 설정 레퍼런스

설정 파일 위치: `~/.c4/config.yaml` (전역) 또는 `.c4/config.yaml` (프로젝트별).

## 설정이 필요한 경우

- **solo 티어** — 설정 불필요. CQ가 바로 작동합니다.
- **connected / full 티어** — 팀 또는 조직에서 설정을 제공합니다. `~/.c4/config.yaml`에 위치시키세요.

아래 섹션은 참고용 사용 가능 옵션을 문서화합니다.

## 섹션

### `pop` — Personal Ontology Pipeline (solo+)

```yaml
pop:
  enabled: true
  confidence_threshold: 0.8   # Soul에 반영할 최소 신뢰도
  max_proposals_per_cycle: 20
  soul_path: ""               # 기본값: .c4/souls/{user}/soul-developer.md
```

### `hub` — 분산 워커 큐 (full 티어)

워커는 `pgx LISTEN/NOTIFY`를 통해 Supabase에 직접 연결합니다 — 로컬 Hub 프로세스 없음, Hub URL 설정 불필요.

```yaml
hub:
  enabled: true
  # api_key: cq secret set hub.api_key <값>  (Supabase service role 키)
  # 키 미설정 시 cloud session JWT를 자동으로 사용
```

`hub` 섹션은 LISTEN/NOTIFY 연결에 `cloud.url`과 `cloud.direct_url` (포트 5432)을 사용합니다. `hub.url`은 필요 없습니다.

::: tip Telegram 알림
잡 완료 알림은 Telegram으로 전송됩니다. `cq setup`을 실행해 BotFather를 통해 Telegram 봇을 페어링하세요.
:::

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
  eventbus:
    enabled: false
  gpu:
    enabled: false
  agent:
    enabled: false   # cloud.url + cloud.anon_key 필요
  eventsink:
    enabled: false
    port: 4141
    token: ""
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 stale 판정
    interval_seconds: 60
```

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
  # direct_url: "postgresql://..."   # 선택: hub의 LISTEN/NOTIFY에 사용 (포트 5432)
```
