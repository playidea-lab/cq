# 티어

CQ는 세 가지 티어로 배포됩니다. 환경에 맞는 티어를 선택하세요.

## solo (기본값)

**로컬 전용. 외부 서비스 불필요.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
# 또는 명시적으로:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier solo
```

포함 기능:
- 전체 태스크 관리 (plan → run → review → finish)
- 로컬 SQLite 데이터베이스
- 워커당 git 워크트리 격리
- 바이너리에 내장된 스킬
- `cq doctor` 환경 점검
- 시크릿 스토어 (`~/.c4/secrets.db`)
- Personal Ontology Pipeline (POP)

적합한 경우: 개인 프로젝트, 오프라인 환경, 처음 시작할 때.

---

## connected

**클라우드 동기화, LLM Gateway, EventBus, Telegram 알림 추가.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

`solo`에 추가되는 기능:
- **Supabase** 클라우드 스토리지 (태스크, 문서, 팀 데이터)
- **LLM Gateway** — Anthropic, OpenAI, Gemini, Ollama 통합 API
- **C3 EventBus** — 실시간 알림을 위한 gRPC 이벤트 버스
- **C0 Drive** — Supabase Storage 파일 스토리지
- **C9 Knowledge** — 크로스 프로젝트 지식 공유를 위한 시맨틱 검색 + pgvector
- **페르소나/Soul 진화** — 코딩 스타일 패턴 학습
- **C6 Secret Central** — 암호화 시크릿 동기화 (Supabase 기반, cache-first)
- **Telegram 봇** — 잡 완료 알림 + BotFather를 통한 슬래시 명령 (`cq setup`)
- **지식 자동 pull** — 세션 시작 시 지식 베이스 자동 동기화

팀 또는 조직에서 제공하는 클라우드 설정이 필요합니다. 첫 사용 전 `~/.c4/config.yaml`에 위치시키세요.

적합한 경우: 팀, 다중 머신 설정, AI 기반 워크플로우.

---

## full

**분산 워커 큐와 데스크톱 앱을 포함한 전체 기능.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

`connected`에 추가되는 기능:
- **Supabase 워커 큐** — LISTEN/NOTIFY 기반 분산 워커 큐 (NAT-safe, 아웃바운드 연결만)
- **CDP** — Chrome DevTools Protocol 자동화
- **GPU** — 로컬 GPU 잡 스케줄러
- **C1 Messenger** — Tauri 데스크톱 대시보드
- **Research** — 논문/실험 추적 루프
- **연구 루프 (C9)** — c9-* 스킬 11개로 ML 실험 루프 자동화

적합한 경우: 프로덕션 배포, ML 워크플로우, 대규모 팀.

---

## 비교

| 기능 | solo | connected | full |
|------|:----:|:---------:|:----:|
| 태스크 관리 | ✅ | ✅ | ✅ |
| 로컬 SQLite | ✅ | ✅ | ✅ |
| 스킬 내장 | ✅ | ✅ | ✅ |
| 시크릿 스토어 | ✅ | ✅ | ✅ |
| Supabase 동기화 | — | ✅ | ✅ |
| LLM Gateway | — | ✅ | ✅ |
| EventBus | — | ✅ | ✅ |
| C9 Knowledge (시맨틱) | — | ✅ | ✅ |
| Telegram 봇 | — | ✅ | ✅ |
| 지식 자동 pull | — | ✅ | ✅ |
| 분산 워커 (LISTEN/NOTIFY) | — | — | ✅ |
| CDP 자동화 | — | — | ✅ |
| GPU 스케줄러 | — | — | ✅ |
| POP (개인 온톨로지) | ✅ | ✅ | ✅ |
| 페르소나/Soul 진화 | ✅ | ✅ | ✅ |
| 스킬 헬스 파이프라인 | — | ✅ | ✅ |
| 연구 루프 (c9-*) | — | — | ✅ |
| C6 Secret Central | — | ✅ | ✅ |

## 설정 파일 위치

CQ는 `~/.c4/config.yaml`에서 설정을 찾습니다. `solo` 티어는 설정이 불필요합니다 — 바로 사용 가능합니다.

`connected` 및 `full` 티어는 팀 또는 조직에서 설정 파일을 제공합니다. `cq claude` 실행 전에 `~/.c4/config.yaml`에 위치시키세요.

## 설정 템플릿

`~/.c4/config.yaml`에 복사 후 수정하세요.

### solo

```yaml
# CQ Solo 티어 설정 템플릿
# ~/.c4/config.yaml에 복사 후 수정

# 태스크 스토리지
task_store:
  type: sqlite
  path: ~/.c4/tasks.db

# LLM Gateway (선택 — c4_llm_call 도구용)
# llm_gateway:
#   default_provider: anthropic
#   providers:
#     anthropic:
#       api_key_env: ANTHROPIC_API_KEY

# 권한 리뷰어 (bash 훅)
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true
```

### connected

```yaml
# CQ Connected 티어 설정 템플릿
# 요구사항: Supabase 계정, C3 EventBus (선택)

# 태스크 스토리지
task_store:
  type: supabase  # 로컬 fallback은 sqlite

# Cloud (Supabase)
cloud:
  url: https://your-project.supabase.co
  anon_key: your-anon-key

# 권한 리뷰어
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true

# 백그라운드 데몬 (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 초기화
    interval_seconds: 60
```

### full

```yaml
# CQ Full 티어 설정 템플릿
# 요구사항: connected 티어 설정 + Supabase (cloud.url)

# (connected 설정 포함, 추가:)

# Hub — 분산 워커 큐 (Supabase LISTEN/NOTIFY 사용)
hub:
  enabled: true
  # api_key: cq secret set hub.api_key <값>  (Supabase service role 키)
  # 키 미설정 시 cloud session JWT를 자동으로 사용

# 백그라운드 데몬 (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30
    interval_seconds: 60
```
