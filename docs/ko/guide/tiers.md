# 포함된 기능

CQ는 **단일 바이너리**로 모든 기능이 포함되어 배포됩니다. 티어 선택이 필요 없습니다.

::: tip v1.34.0부터 단일 티어
이전에는 CQ가 세 가지 티어(solo, connected, full)로 배포되었습니다. 바이너리 크기 차이가 295KB에 불과해서 모든 기능을 하나로 합쳤습니다.
:::

## 전체 기능

모든 CQ 설치에 포함됩니다:

- **169+ MCP 도구** (기본 ~40개 표시, `CQ_TOOL_TIER=full`로 전체 노출)
- **39개 스킬** (★ core / [internal] 마커)
- **태스크 관리** — plan → run → review → finish
- **Polish & Refine 게이트** — Go 레벨 품질 강제
- **로컬 SQLite** 데이터베이스 + **Supabase** 클라우드 동기화
- **Git 워크트리** 워커당 격리
- **LLM Gateway** — Anthropic, OpenAI, Gemini, Ollama 통합 API (클라우드 관리 키)
- **C3 EventBus** — 실시간 알림을 위한 gRPC 이벤트 버스
- **C0 Drive** — Supabase Storage 파일 스토리지
- **C9 Knowledge** — 크로스 프로젝트 지식 공유를 위한 시맨틱 검색 + pgvector
- **페르소나/Soul 진화** — 코딩 스타일 패턴 학습
- **C6 Secret Central** — 암호화 시크릿 동기화 (Supabase 기반, cache-first)
- **Relay** — WSS 기반 NAT 트래버설. 토큰 없는 `.mcp.json` — `cq serve`가 JWT 자동 주입
- **Telegram 봇** — 잡 완료 알림 + 슬래시 명령
- **분산 워커** — Supabase LISTEN/NOTIFY 큐 (NAT-safe)
- **3계층 온톨로지** — L1 로컬 → L2 프로젝트 → L3 집단 패턴 학습
- **워커 생존성** — systemd `Restart=always`, WSL2 작업 스케줄러, auto-linger
- **Learn Loop** — 4개 와이어: submit→learn, reject→warning, get_task→inject, hook_deny→inject
- **자동 라우팅** — Small (직접 수정), Medium (/c4-quick), Large (/pi → plan → run → finish)

## 도구 티어링

CQ에는 169+ 도구가 있지만 도구 목록 관리를 위해 기본적으로 ~40개만 표시됩니다. 전체를 보려면:

```sh
export CQ_TOOL_TIER=full
```

## 클라우드 기능

클라우드 기능 (Supabase 동기화, LLM Gateway, Knowledge, Relay)은 로그인 후 자동 활성화됩니다:

```sh
cq    # 로그인 + 서비스 설치 자동 처리
```

API 키 불필요. 수동 설정 불필요. 클라우드가 태스크, 지식, LLM 호출의 SSOT가 됩니다.

## 설정 파일 위치

CQ는 `~/.c4/config.yaml`에서 설정을 찾습니다. 대부분의 사용자는 설정이 불필요합니다 — 로그인 후 `cq`가 모든 것을 자동으로 처리합니다.

고급 사용자 지정:

```yaml
# ~/.c4/config.yaml

# 태스크 스토리지
task_store:
  type: supabase  # 로컬 전용은 sqlite

# 백그라운드 데몬 (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 초기화
    interval_seconds: 60

# Hub — 분산 워커 큐 (Supabase LISTEN/NOTIFY 사용)
hub:
  enabled: true

# 권한 리뷰어 (bash 훅)
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true
```

## 빌드 태그 (고급)

소스에서 빌드하는 기여자를 위해, 기능은 빌드 태그로 제어됩니다:

| 태그 | 기능 |
|------|------|
| `c0_drive` | 파일 스토리지 |
| `c3_eventbus` | gRPC 이벤트 버스 |
| `hub` | 분산 워커 큐 |
| `llm_gateway` | LLM 프록시 |
| `cdp` | Chrome DevTools Protocol |
| `gpu` | GPU 잡 스케줄러 |
| `c1_messenger` | Telegram 봇 |
| `research` | ML 실험 루프 |
| `skills_embed` | 내장 스킬 |
| `c7_observe` | Observe 트레이스 시스템 |

기본 빌드에는 모든 태그가 포함됩니다. 상세는 `Makefile` 참조.
