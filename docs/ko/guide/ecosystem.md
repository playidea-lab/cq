# C 시리즈 생태계

CQ는 **C4 Engine**의 CLI 및 배포 레이어이며, **C 시리즈**라는 상호 연결된 컴포넌트 생태계의 일부입니다.

## 철학

> "유일한 진실은 데이터다. 최소한으로 구현하고, 결과로 말한다."

C 시리즈는 세 가지 원칙을 기반으로 합니다:

1. **데이터가 유일한 진실** — 모든 결정은 의견이 아닌 데이터로 검증됩니다
2. **최소 구현 우선** — 작동하는 것부터 시작하고, 필요할 때만 복잡도를 추가합니다
3. **모든 것이 버전화되고 재현 가능** — 데이터, 코드, 실험이 모두 추적됩니다

사람이든 에이전트든 품질 기준은 동일합니다. 작업은 항상 리뷰됩니다.

---

## 컴포넌트

```
C0 Drive      — 클라우드 파일 스토리지 (Supabase Storage)
C1 Messenger  — Tauri 2.x 통합 대시보드 (4탭: Messenger, Docs, Settings, Team)
C2 Docs       — 문서 라이프사이클 (파싱, 워크스페이스, 프로필)
C3 EventBus   — gRPC 이벤트 버스 (UDS + WebSocket + DLQ)
C4 Engine     — MCP 오케스트레이션 엔진  ← 여기
C5 Hub        — Supabase 네이티브 워커 큐 (pgx LISTEN/NOTIFY, lease 기반)
C6 Guard      — RBAC 접근 제어 (정책, 감사, 역할 할당)
C7 Observe    — 관측 가능성 레이어 (메트릭, 로그, 트레이싱 미들웨어)
C8 Gate       — 외부 통합 (웹훅, 스케줄러, 커넥터)
C9 Knowledge  — 지식 관리 (FTS5 + pgvector + 임베딩 + 수집)
```

각 컴포넌트는 독립적으로 또는 함께 실행할 수 있습니다. CQ의 티어가 이를 반영합니다:

| 티어 | 활성 컴포넌트 |
|------|-------------|
| solo | C4만 |
| connected | C4 + C0 + C3 + C9 + LLM Gateway |
| full | 전체 컴포넌트 (+ C1, C5 워커 큐, C6, C7, C8) |

C6/C7/C8은 빌드 태그(`c6_guard`, `c7_observe`, `c8_gate`)로 활성화됩니다 — `full` 티어 바이너리에는 항상 컴파일되어 있습니다.

---

## C4 Engine (이 프로젝트)

C4는 오케스트레이션 코어입니다. Claude Code에 **100개 이상의 MCP 도구 (티어별로 다름)** (`c4_*`)를 노출하고 다음을 관리합니다:

- **태스크 라이프사이클** — 생성, 할당, 리뷰, 체크포인트, 완료
- **워커 격리** — 각 워커는 새 git 워크트리를 받습니다
- **지식 누적** — 발견 사항이 자동으로 기록되어 미래 태스크에 주입됩니다
- **시크릿 스토어** — AES-256-GCM, 설정 파일에 저장하지 않음
- **LLM Gateway** — Anthropic, OpenAI, Gemini, Ollama 통합 API
- **스킬** — 바이너리에 내장된 36개 슬래시 명령 (/pi, c9-* 연구 루프 등)

---

## C1 Messenger

4가지 뷰를 가진 Tauri 2.x 데스크톱 앱:

- **Messenger** — Supabase Realtime을 통한 실시간 팀 채팅, 에이전트 프레즌스
- **Documents** — C2를 통한 로컬 파일 파싱
- **Settings** — `.claude/` 및 `.c4/` 설정 뷰어/편집기
- **Team** — Supabase 기반 프로젝트 대시보드

구성원은 통합되어 있습니다: 사람, 에이전트, 시스템이 모두 동등한 참여자입니다.

---

## C2 Docs

문서 라이프사이클 관리:

- PDF, EPUB, HTML, Markdown을 구조화된 워크스페이스로 파싱
- 프로필/페르소나 시스템 — 사용자 편집에서 학습
- `/c2-paper-review` 및 `/c4-review` 스킬을 지원

---

## C3 EventBus

모든 컴포넌트를 연결하는 gRPC 이벤트 버스:

- **19개 이상의 이벤트 유형**: `task.created/started/completed/blocked/stale`, `checkpoint.approved/rejected`, `review.changes_requested`, `validation.passed/failed`, `knowledge.recorded/searched`, `lighthouse.promoted`, `llm.cache_miss_alert`, `persona.evolved`, `soul.updated`, `research.recorded/started`
- 실패 전달을 위한 **DLQ** (dead letter queue)
- **Filter v2**: `$eq`, `$ne`, `$gt`, `$in`, `$regex`, `$exists`
- 외부 통합을 위한 **HMAC-SHA256 웹훅**

---

## C5 Hub (Supabase 워커 큐)

대규모 워커 실행을 위한 Supabase 네이티브 분산 잡 큐:

- **pgx LISTEN/NOTIFY** — 워커가 Supabase Postgres를 구독하여 실시간 잡 수신
- **Lease 기반** — 잡은 타임아웃이 있는 lease로 관리, 실패 시 자동 재큐
- **VRAM 인식 스케줄링** — GPU 워커를 사용 가능한 VRAM으로 매칭, CPU fallback 설정 가능
- **아티팩트 파이프라인** — 워커가 서명된 URL로 입력 다운로드, 출력 업로드
- **로그 보존** — 자동 로테이션 (5만 행) + 7일 정리
- Supabase PostgREST + RPC API

워커는 Supabase에 직접 연결됩니다 — 별도 Hub 서버 프로세스 시작 및 관리 불필요.

---

## C6 Guard

C4 도구를 위한 역할 기반 접근 제어:

- **정책 엔진** — 도구별, 역할별 허용/거부 규칙
- **감사 로그** — 행위자와 결정이 포함된 모든 도구 호출 기록
- **역할 할당** — 에이전트나 사용자에게 역할 부여
- `c6_guard` 빌드 태그로 활성화

---

## C7 Observe

C4 엔진을 위한 관측 가능성 레이어:

- **메트릭** — 도구별 요청 수, 지연 시간, 오류율
- **구조화 로그** — `slog` 기반, 설정 가능한 레벨과 형식
- **미들웨어** — 모든 MCP 도구 호출을 자동으로 계측
- `c7_observe` 빌드 태그로 활성화

---

## C8 Gate

외부 통합 허브:

- **웹훅** — 엔드포인트 등록, 페이로드 테스트, HMAC-SHA256 서명
- **스케줄러** — C4 태스크를 트리거하는 cron 스타일 잡
- **커넥터** — Telegram과 GitHub 기본 지원
- `c8_gate` 빌드 태그로 활성화

---

## 페르소나 & Soul 진화

CQ가 코딩 패턴을 학습하고 시간에 따라 행동 방식을 진화시킵니다:

- **패턴 추출** — AI 초안과 최종 수정본 사이의 diff를 분석
- **Soul 지속성** — 패턴이 `.c4/souls/{user}/raw_patterns.json`에 누적
- **진화** — `scripts/soul-evolve.sh`가 누적 패턴을 `soul-developer.md`로 합성
- `c4_persona_learn` / `c4_soul_get` / `c4_soul_set` MCP 도구

---

## POP (개인 온톨로지 파이프라인)

대화에서 지식 제안을 자동 추출하여 Soul에 결정화합니다:

- **5단계 파이프라인** — Extract → Consolidate → Propose → Validate → Crystallize
- **신뢰도 게이팅** — HIGH 신뢰도(≥0.8) 제안만 Soul에 반영
- **Gauge 추적** — merge_ambiguity / avg_fan_out / contradictions / temporal_queries
- **원자적 쓰기** — soul_backup/ 유지 (10개 스냅샷)
- `c4_pop_extract` / `c4_pop_status` / `c4_pop_reflect` MCP 도구
- `cq pop status` CLI 커맨드

---

## C9 Knowledge

다층 지식 스토어:

- **FTS5** 전문 검색 (SQLite)
- **pgvector** 시맨틱 검색 (Supabase)
- **임베딩** 파이프라인 (사용 추적 포함)
- 문서, 웹 페이지, 실험에서 **수집**
- 크로스 프로젝트 지식 공유를 위한 **publish/pull**

---

## 연결 방식

```
Claude Code
    │
    ▼ MCP (stdio)
C4 Engine ──────────────── C9 Knowledge (검색 + 기록)
    │                              ▲
    ├── C3 EventBus ───────────────┘ (task.completed → 자동 기록)
    │       │
    │       └── C1 Messenger (실시간 알림)
    │
    ├── Supabase (pgx LISTEN/NOTIFY 워커 큐)
    │       └── C0 Drive를 통한 아티팩트 스토리지
    │
    └── LLM Gateway (Anthropic / OpenAI / Gemini / Ollama)
```
