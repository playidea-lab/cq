# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.16.0] - 2026-02-22

### ✨ Features
- **lsp**: `c4_find_symbol` 결과의 `symbols[]` 각 항목에도 `_edit_hint` 주입 — Agent가 심볼 항목을 꺼내 처리할 때도 편집 제약 인식 (`f0f023b`)

### 🐛 Bug Fixes
- **lsp**: `get_symbols_overview` overview 그룹 항목(`functions[]`/`methods[]`/`structs[]` 등)에 `_edit_hint` 누락 수정 (`9ae2846`)
- **lsp**: Dart `handleDartSymbolsOverview` 키 목록을 `dartast.kindGroup` 실제 키와 일치시킴 — `typedefs` 누락 + 존재하지 않는 키 제거 (`bfc5c0b`)
- **lsp**: `languageGuardedProxy` 응답에 `success:false` 추가 — 차단 응답을 성공과 명시적으로 구분 (`bfc5c0b`)
- **hook**: `.c4/` 디렉토리 walk-up 탐지 + fallback 패턴 + doctor --fix hook 업데이트 (`6d6f101`)

### 🧪 Tests
- **lsp**: Go overview 테스트에 `structs`/`methods`/`constants` 카테고리 검증 추가 (`ed8385b`)
- **lsp**: Dart overview 테스트에 `classes[]`/`functions[]` 항목 순회 검증 추가 (`bfc5c0b`)

### 📚 Documentation
- **skills**: polish 누락 방지 — run/refine/plan 스킬 3곳에 플로우 명시 (`1371548`)
- **skills**: TDD 원칙 강화 — 3-layer 안전망 + C1 테스트 커버리지 인프라 (`3cdbbe1`)

### 🔧 Chores
- **user**: submodule 업데이트 (workflow page cross-links, checkpoint explanation, workflow overview)

---

## [0.15.0] - 2026-02-22

### ✨ Features
- **lsp**: Go/Dart native handler 응답에 `_edit_hint` 필드 주입 — Agent가 find/overview 결과에서 편집 도구 제약을 즉시 인식 (`76d80cb`)

### 🐛 Bug Fixes
- **lsp**: `languageGuardedProxy` 도구명 PascalCase 버그 수정 — `toolName` 명시적 snake_case 전달, 미사용 `rootDir` 파라미터 제거 (`8dbfc5c`)

### 🧪 Tests
- **c1**: C1 Messenger-first 재설계 신규 컴포넌트 테스트 추가 (`d4efa3f`)
- **handlers**: `language_guard_test.go` (5개) + `lsp_hint_test.go` (4개) = Go +9 tests (`76d80cb`, `8dbfc5c`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,262 → ~1,271 반영 (`59ca0df`)

### 🔧 Chores
- **user**: submodule 업데이트 (README user scenarios, distributed experiment scenario, Examples section, refine+polish workflow pages)

---

## [0.14.0] - 2026-02-22

### ✨ Features
- **eventbus**: C7 Observe → C3 EventBus 연결 — package-level publisher setter (`SetEventBus`), `tool.called` 이벤트 발행 (`c2f217d`)
- **eventbus**: C6 Guard → C3 EventBus 연결 — ActionDeny 시 `guard.denied` 이벤트 발행 (`2ce1df8`)
- **eventbus**: C8 Gate → C3 EventBus bridge — `task.completed`/`hub.job.completed` 구독 후 WebhookManager 트리거 (`adc7cdc`)
- **workflow**: c4-finish quality gate system — DB 기반 polish gate 검증, 세션 메모리 의존 제거 (`91192ff`)

### 📚 Documentation
- **agents**: C3 EventBus 이벤트 종류 16종 → 18종 (`tool.called`, `guard.denied` 추가) (`de3cf84`)

### 🔧 Chores
- **user**: submodule 업데이트 (docs: cloud config, polish 설치/사용법, 전체 문서 리뷰 수정)

---

## [0.13.0] - 2026-02-22

### ✨ Features

#### C1 Messenger — Messenger-first UI/UX 완전 재설계 (14 tasks)

- **3-column 레이아웃**: `WorkspaceNav`(48px 아이콘 네비) + `ChannelListArea`(240px) + `ContentArea`(flex-grow)
  - `WorkspaceNav.tsx`: 5개 워크스페이스 모드 아이콘 (`messenger/documents/settings/team/search`)
  - `MainLayout.tsx`: BEM 3-column CSS 레이아웃, `main-layout__nav/channel-list/content`
- **`channel_type` 스키마 정립** (migration `00024_c1_channel_types.sql`):
  - 5가지 타입: `general | project | knowledge | session | dm` (CHECK constraint)
  - `c1_channel_pins` 테이블: ProductView 버전 히스토리용
  - `agent_work_id` 컬럼: AgentThread 그루핑용
  - session 채널 전용 UNIQUE INDEX
- **`ChannelListSidebar v2`**: 5개 섹션 (General `#` / Projects `📂` / Knowledge `🧠` / Sessions `💬` / Direct `✉`)
  - 섹션별 collapse/expand 토글, 빈 섹션 표시
- **`ChannelContent`**: `productSlot` + `ConversationArea` 2-column 레이아웃 컴포넌트
- **`ProductView`**: 채널 핀 마크다운 렌더링 + 버전 히스토리 드롭다운
  - `useChannelPins(channelId)` 훅, Rust `create/list/delete_channel_pin` 명령어
- **`AgentThread`**: `agent_work_id` 기반 메시지 그루핑 컴포넌트
  - in-progress → 자동 확장, completed/failed/cancelled → 자동 접기
  - `MessageList.tsx`: `groupMessages()` 알고리즘으로 연속 동일 `agent_work_id` 묶음
- **`@mention` UI**: `@` 트리거 → `MentionPopup` 자동완성 (↑↓ 키보드 네비)
  - `metadata.mention = {agent, task: ''}` 메타데이터 주입
- **A2UI (Agent-to-UI) 시스템**: 에이전트가 UI 액션 버튼을 주입하는 프로토콜
  - `types/a2ui.ts`: `A2UISpec`, `A2UIAction`, `isA2UISpec()` 타입 가드
  - `A2UIRenderer.tsx`: primary/secondary/danger 스타일 액션 버튼 렌더링
- **`sync_session_channels`**: 로컬 Claude 세션을 Supabase session 채널로 자동 동기화 (Rust)
  - 이름 형식: `claude-{MMDD}-{session_uuid_8}`
  - Supabase REST upsert (`Prefer: resolution=merge-duplicates`)
  - `useSessionChannels()` 훅: `channel_type === 'session'` 필터링

#### MCP

- **`c4_task_list` `include_dod` 파라미터** 추가: 기본 `false` (brief listing), `true`일 때만 DoD 필드 포함

### 🐛 Bug Fixes

- **`c4_task_list`**: `include_dod` 기본값 `true` → `false`로 수정 (대용량 DoD로 응답 크기 급증 방지)

### 📚 Documentation

- AGENTS.md: Rust 테스트 수 85 → 92 (C1 재설계 +7 tests)
- AGENTS.md: LSP 언어별 지원 범위 명확화 — Go/Dart native 추가, `c4_find_symbol` 사용 원칙 섹션

---

## [0.12.0] - 2026-02-21

### ✨ Features

- **`skills_embed` 빌드 태그**: 배포 바이너리에 `.claude/skills/` 임베딩 지원
  - `make embed-skills` → `cmd/c4/skills_src/` 복사 (symlink deref) + SHA 버전 파일 생성
  - `//go:build skills_embed`: `embed.FS → fs.FS` 래퍼 + `!skills_embed` stub (기본 빌드 무영향)
  - 3단계 폴백: 소스 루트 심링크(개발) → 임베디드 추출(설치) → graceful skip
  - 버전 인식 추출: `~/.c4/skills/.version` SHA 비교, 동일 버전 재추출 생략
- **LLM Gateway API 키 보안 강화**: `config.yaml`에서 `api_key`/`api_key_env` 필드 제거
  - 키 해석 우선순위: `secrets.db (<provider>.api_key)` → 환경 변수 (`ANTHROPIC_API_KEY` 등)
  - 구버전 YAML 설정 감지 시 `slog.Warn` deprecation 경고 출력 (`config.Manager.IsSet()` 활용)
  - Ollama 예외 처리: 키 없어도 활성화 유지 (`name != "ollama"` 조건)
- **공개 배포 서브모듈** (`user/` → [PlayIdea-Lab/cq](https://github.com/PlayIdea-Lab/cq)):
  - `install.sh`: POSIX sh, `--tier solo|connected|full` (기본: solo), `--dry-run` 지원
  - `configs/solo.yaml`, `configs/connected.yaml`, `configs/full.yaml` 샘플 설정 포함
- **GitLab CI 크로스 컴파일 파이프라인** (`.gitlab-ci.yml`):
  - 3단계: `check-env` → 9개 병렬 빌드 → `release-github`
  - 9개 바이너리: 3 tier (solo/connected/full) × 3 platform (linux/darwin/windows-amd64)
  - `gh release create` → `PlayIdea-Lab/cq` GitHub Releases 자동 업로드

### 🔧 Build

- **Makefile**: `TIER_TAGS_*` 변수 + `build-cross` 타겟 추가
  - `make build-cross GOOS=linux GOARCH=amd64 TIER=solo` → `dist/cq-solo-linux-amd64`
  - `build-solo/connected/full/nightly` 타겟이 `embed-skills`에 의존 + `skills_embed` 태그 포함

### 📚 Documentation

- AGENTS.md: `user/` submodule, `.gitlab-ci.yml`, LLM key 변경 사항 반영

---

## [0.11.0] - 2026-02-21

### ✨ Features

- **`secrets`: AES-256-GCM 암호화 시크릿 스토어** (`~/.c4/secrets.db`)
  - 마스터 키 자동 생성 (`~/.c4/master.key`, 0400) / CI: `C4_MASTER_KEY=<64 hex>` env var 지원
  - CLI: `cq secret set/get/list/delete` — `set`은 stdin echo off (ioctl 기반)
  - MCP: `c4_secret_set`, `c4_secret_get`, `c4_secret_list`, `c4_secret_delete`
  - LLM Gateway 키 자동 해석: `config.yaml api_key` > `api_key_env` > `secrets.db (name.api_key)` 순

### 🛡️ Security (secrets store 강화 — 5라운드 polish)

- **O_EXCL 원자 키 생성** — `os.WriteFile` 대신 `O_EXCL|O_CREATE` + 실패 시 `os.Remove` 정리
- **TOCTOU 방지** — `os.Open` FD 후 동일 FD의 `Stat()` 호출로 파일 교체 경쟁 제거
- **EACCES vs ENOENT 구분** — 권한 오류 시 즉시 전파, ErrNotExist 시만 키 생성
- **마스터 키 zeroing** — `Close()`에서 인메모리 키 바이트 초기화
- **`io.LimitReader(f, 33)`** — 마스터 키 파일 읽기 크기 바운드
- **이중 shutdown 방지** — `sync.Once`로 signal + stdin EOF 동시 발생 시 race 제거
- `c4_secret_get`: plaintext 반환 경고 문구 handler description에 명시
- MCP 핸들러: key 길이 상한(256B) + value 크기 상한(64KB) 적용

### 🧪 Tests

- `secrets`: 10개 테스트 — `TestCorruptMasterKey`, `TestWrongKeyDecryptionFailure`, `TestMasterKeyEnvVarPersistence` 신규 추가

### 📚 Documentation

- Go 테스트 수 현행화: c4-core ~1,262 + c5 174 = ~1,436

---

## [0.10.0] - 2026-02-21

### ✨ Features

- **`hub`: Worker standby 자동 lease 갱신** — standby 루프에서 lease를 주기적으로 자동 갱신, 장시간 대기 시 lease 만료 방지
- **`llm`: `api_key` 직접 값 지원** — `LLMProviderConfig`에 환경변수 외 인라인 API 키 설정 가능

### 🐛 Bug Fixes

- **`task`**: Task ID 문법 regex + `ReviewID` last-hyphen split 수정 (CR-027)
- **`submit`**: `validation_results` 정책 통합 — optional, non-empty, status enum 검증 일관성 (CR-013/014)
- **`store`**: `files_changed` 컬럼 누락 수정 — `c4_tasks` 테이블에 추가 (CR-017)
- **`review`**: `max_revision` 경계 조건 수정 (`>` → `>=`) 및 정책 문서화
- **`security`**: `SubmitTask`에서 worker ownership 항상 검증 (CR-012)
- **`checkpoint`**: `APPROVE_FINAL`을 유효한 결정 값으로 추가
- **`handlers`**: `MarkBlocked` not-found 에러 계약 명확화 및 테스트 동기화 (CR-021)

### 🛡️ Security (CDP element-ref API 강화)

- `config.yaml` allow_patterns에 `( |$)` 끝 앵커 추가 — compound 명령 우회(`cmd&&evil`) 방지
- `find` 패턴을 두 개로 분리 — `./../../` 경로 순회 공격 차단
- `TypeByRef` 응답에서 `Value` 필드 제거 — 자격증명 에코 방지
- JS text sanitization에 C1 제어문자(U+0080–U+009F) 추가
- `TypeByRef` 라이브러리 레이어 empty-text 가드 — 브라우저 연결 전 조기 검증

### 🧪 Tests

- `test(worker)`: `handleWorkerComplete` status enum 검증 테스트 추가 (CR-006)
- `test(worktree)`: `SubmitTask` worktree 자동 cleanup 테스트 추가
- CDP ref-based API 검증 오류 테스트 — remote URL, invalid ref, empty text, credentials

### 📚 Documentation

- Go 테스트 수 업데이트: ~1,651개 (c4-core ~1,477 + c5 174), 26 패키지

---

## [0.9.2] - 2026-02-21

### ✨ Features

- **`c4_cdp_action` MCP 도구 추가** — Element-ref 기반 DOM 인터랙션
  - `scan_elements`: DOM 스캔 → `data-cdp-ref` 속성 부여 → `ElementRef` 배열 반환
  - `click` / `type` / `get_text`: ref ID로 요소 조작 (해상도 독립)
  - SPA에서 ref가 DOM 업데이트 후에도 유지되어 raw JS보다 안정적
  - Chrome 미연결 시 명확한 오류 메시지 + `CDP_DEBUG_URL` 환경변수 안내

- **`c4_run_validation` → `config.yaml` SSOT 연결** (T-SSOT-002)
  - `validation.lint` / `validation.unit` 설정이 `c4_run_validation`에 즉시 반영
  - `SetValidationConfig` 패턴: init-time 주입, 기존 파일 기반 자동 탐지를 fallback으로 유지
  - `strings.Fields` 파싱: `parts[0]` → Command, `parts[1:]` → Args (shell 미경유 → injection 안전)

### 🔧 Chores

- **`resolveHookModel` → `llm.ResolveAlias` 위임** (T-SSOT-001)
  - 별도 alias 테이블 제거, 모델 ID가 `llm/models.go` 단일 소스에서 관리됨
  - 이전 하드코딩: `sonnet-4-5`, `opus-4-5` → 현재: `sonnet-4-6`, `opus-4-6`

- **`.mcp.json` gitignore** (T-SSOT-004)
  - `infra/supabase/.mcp.json` → `**/.mcp.json` (전역 패턴)
  - `.mcp.json`은 개발자별 절대 경로를 포함하므로 버전 관리에서 제외

- **`config.yaml` 템플릿 개선** (T-SSOT-003)
  - `serve` 섹션에 v0.9.0 신규 컴포넌트 추가: `eventsink`, `sse_subscriber`, `agent`
  - `validation` 섹션에 따옴표 포함 인자 미지원 주의사항 주석 추가
  - `allow_patterns` 기본값 추가: git/go/uv/ls/grep/cq 등 안전한 개발 명령 즉시 허용

### 📚 Documentation

- **`AGENTS.md`**: `.mcp.json` 개발자별 파일 안내 추가 (`git rm --cached` 마이그레이션 가이드)
- **`mcp_init.go`**: `validCfg` 스냅샷 특성 주석 추가 (c4_config_set 변경은 재시작 후 반영)

## [0.9.1] - 2026-02-21

### 🐛 Bug Fixes

- **`ssesubscriber`**: X-API-Key 헤더 사용 (`Authorization: Bearer` → `X-API-Key`) — C5 auth 스펙 준수
- **`ssesubscriber`**: 백오프 지수 오버플로우 방지 (`exp > 30` 상한 설정)
- **`ssesubscriber`**: `bufio.Scanner` 토큰 버퍼 1MiB로 확장 (기본 64KiB → 대용량 SSE 페이로드 처리)
- **`ssesubscriber`**: `http.Client` 재사용 (reconnect loop마다 재생성 → 구조체 필드로 유지)
- **`eventsink` / `hubpoller`**: NoopPublisher → `eb.Publisher()` 연결 수정 (pub wiring)
- **`detect.go`**: `isServeRunningWith` 코드 중복 제거 → `isServeRunningWithCtx` 위임

### 📚 Documentation

- **AGENTS.md**: `cq serve` 컴포넌트 테이블에 `ssesubscriber` 항목 추가
  - 활성화 조건 (`serve.ssesubscriber.enabled: true`, `c5_hub && c3_eventbus` 빌드 태그) 명시

## [0.9.0] - 2026-02-21

### ✨ Features

- **`permission_reviewer` Config SSOT 완성**
  - `.c4/config.yaml` → `.c4/hook-config.json` → hook 스크립트 전체 파이프라인 연결
  - `mode` (`hook` / `model`), `auto_approve`, `allow_patterns`, `block_patterns` 필드 추가
  - `PermissionReviewerConfig` Go struct에 `Mode` 필드 추가 — 하드코딩 제거
  - `hookConfigFromC4Config()`: config에서 모든 필드 읽도록 전환

- **`cq init` 기본 `config.yaml` 자동 생성**
  - 신규 사용자가 별도 설정 없이 hook이 즉시 동작 (`enabled: true`, `mode: hook`)
  - 기존 파일 보존 (덮어쓰지 않음)
  - optional 섹션 (cloud, hub, llm_gateway) 주석 처리로 포함 — 필요 시 uncomment

- **C5 SSE 구독 컴포넌트 (`SSESubscriberComponent`)** (T-924-0)
  - `cq serve`에서 C5 Hub SSE 스트림 구독 지원
  - C5 → C4 이벤트 실시간 수신 경로 구현

- **Agent lazy-start MCP lifecycle 통합** (T-925-0)
  - `cq serve` Agent 컴포넌트를 MCP 서버 시작 시 자동 초기화
  - Supabase Realtime 구독 → `claude -p` 디스패치

- **`cq serve` 컴포넌트 안정화**
  - lazyPublisher 패턴: EventSink / HubPoller에 EventBus publisher 지연 초기화 (T-930-0)
  - EventBusComponent Health 5s TTL 캐시 추가 (T-934-0)
  - GPU handler `/daemon/` prefix로 serve mux에 마운트 (T-931-0)

### 🐛 Bug Fixes

- **`fix(hook)`**: `2>/dev/null` false positive 수정 (T-hook)
  - `>/dev/null`, `>/dev/stderr`, `>/dev/stdin`, `>/dev/stdout`, `>/dev/fd` 를 안전한 리다이렉션으로 허용
  - 실제 위험 패턴 (`>/dev/sda` 등 블록 디바이스)만 차단 유지
- **`fix(serve)`**: `Agent.Stop()` ctx 취소 미반영 수정 (T-933-0)
  - `wg.Wait()` → `select { case <-done: case <-ctx.Done(): }` 교체 — graceful shutdown 보장
- **`fix(serve)`**: 데드 코드 `status.go` 상수 제거 (T-932-0)

### 🔧 Chores

- `.mcp.json` command path `~/.local/bin/cq` (운영 바이너리)로 통일

### 📚 Documentation

- `AGENTS.md`: `permission_reviewer` 전체 스키마 + mode별 동작 + 설정 변경 절차 문서화

---

## [0.8.0] - 2026-02-21

### ✨ Features

- **멀티 세션 격리 개선 3종** (T-ISO-001~003)
  - **State Machine BEGIN IMMEDIATE**: `Transition()` 에 `db.Conn` + `BEGIN IMMEDIATE` 트랜잭션 적용
    - `ErrStateChanged` / `ErrInvalidTransition` / `ErrDatabase` 에러 3종 분류
    - 동시성 테스트 3개 추가 (`-race` 통과): `TestTransitionBeginImmediate`, `TestTransitionConcurrentStateChange`, `TestTransitionConcurrentRecovery`
  - **Advisory Phase Lock** (`c4_phase_lock_acquire` / `c4_phase_lock_release`, 신규 MCP 도구 2개)
    - `.c4/phase_locks/{phase}.lock` JSON 파일 기반, `polish` / `finish` 단계 보호
    - Stale 판정 5개 시나리오 (PID 생존/사망/EPERM + cross-host 2h 임계)
    - `/c4-polish` Phase 0 · `/c4-finish` Step 1에 lock check 통합
  - **Makefile atomic install**: `install-*` 타겟 `cp` → `cp .tmp && mv || rm .tmp` 원자적 교체

- **`cq serve` 컴포넌트 연동 완성**
  - EventBus / GPU Scheduler / EventSink / HubPoller config 로드 + 자동 등록
  - HubPoller: `NoopPublisher` fallback으로 nil publisher panic 제거

- **`c4-plan` Phase 4.5 Worker 기반 Plan Critique Loop**
  - 인라인 자가 비판 → 매 라운드 새 Worker(Task agent) 스폰 (confirmation bias 제거)
  - 수렴 조건: CRITICAL == 0 AND HIGH == 0 (최대 3라운드)

- **`c4-run` R-task 리뷰 Worker 자동 스폰**
  - 준비된 R- 태스크는 `review` 모델(opus)로 별도 Worker 할당

- **Hook Config SSOT 전환** (`.c4/hook-config.json`)
  - `permission_reviewer` 설정을 MCP 서버 시작 시 자동 내보내기
  - `hook-config.json` 우선, `~/.claude/hooks/*.conf` fallback
  - `AutoApprove` / `AllowPatterns` / `BlockPatterns` 필드 추가

### 🐛 Bug Fixes

- **`serve/realtime.go`**: 클린 disconnect 시 backoff가 계속 증가하는 버그 수정
  - backoff 증가를 error 분기 내로 이동 → 클린 재연결은 항상 1s로 리셋
- **`serve.go`**: `config.New()` 실패 시 `cfgMgr` nil 역참조 패닉 방지 (nil guard 추가)
- **`handlers/phase_lock.go`**: phase 파라미터 서버 측 allowlist 검증 누락 수정
- **`state/phase_lock.go`**: `lockFile()` path traversal 취약점 — `validPhase()` allowlist 강제
- **`fix(hook)`**: `mapfile` → `while-read` 교체 (bash 3.2 호환)
- **`fix(review)`**: R-task auto-cascade 제거 — 실제 리뷰 Worker가 처리하도록

### ♻️ Refactoring

- `cq init`: `.conf` embed/생성 제거, `hook-config.json` 패턴으로 통일

### 📚 Documentation

- MCP 도구 수: 154 → 156 (`c4_phase_lock_acquire/release` +2)
- Go 테스트 수: ~1,423 → ~1,430, 합계: ~2,205 → ~2,212

---

## [0.7.0] - 2026-02-21

### ✨ Features

- **`cq serve` 데몬 명령어**: ComponentManager 기반 상시 실행 서버
  - PID 락 파일, `/health` 엔드포인트, 컴포넌트 라이프사이클 관리
  - EventBus, EventSink, HubPoller, GPU Scheduler, Agent 컴포넌트 wrapping
  - Agent 컴포넌트: Supabase Realtime WebSocket → `@cq` mention 감지 → `claude -p` 디스패치
  - `cq serve` 실행 중이면 데몬 내장 컴포넌트 자동 skip (중복 방지)

- **Tiered Build System** (`solo` / `connected` / `full` / `nightly`)
  - Build tag 기반 컴포넌트 선택적 포함 (stub 파일 패턴)
  - `cq init --tier solo|connected|full` → `.c4/config.yaml` tier 저장
  - Makefile: `build-solo/connected/full/nightly`, `install-*` 타겟
  - C7 Observe / C6 Guard / C8 Gate 조건부 빌드 (nightly 태그)

- **C7 Observe**: Logger(slog) + Metrics + Registry Middleware 자동 계측
- **C6 Guard**: RBAC + Audit + Policy + Middleware (`c6_guard` 빌드 태그)
- **C8 Gate**: Webhook + Scheduler + Connector (`c8_gate` 빌드 태그)

- **Registry Middleware 체인**: `Registry.Use()` / `UseContextual()` 지원
  - `ToolNameFromContext()` 로 도구 이름 접근 가능

- **C5 Hub Artifact Pipeline**:
  - Signed Upload/Download (Supabase Storage), Worker input/output artifact 흐름
  - `LocalBackend` HTTP 핸들러 (`/v1/storage/upload/{path}`)
  - Long-poll Lease Acquire (20s server-side)

- **LLM Gateway 캐싱 개선**:
  - Anthropic Prompt Caching (ephemeral cache_control 블록)
  - `cache_hit_rate` + `cache_savings_rate` 노출 (`c4_llm_costs`)
  - `cache_by_default` 설정 지원

- **`cq doctor` 자가진단**: 8개 항목 (binary, .c4, .mcp.json, CLAUDE.md, hooks, Python, C5, Supabase)
- **c4-bash-security hook UX 개선**: 차단 메시지 가독성 향상

### 🐛 Bug Fixes

- `serve/agent.go`: `a.Status()` → `a.status` (미존재 메서드 수정)
- `serve/agent_test.go`: 중복 `mockComponent` 선언 제거, 잘못된 API 테스트 제거
- `c5`: LocalBackend HTTP 핸들러 디렉토리 리스팅 차단, partial file 정리
- LLM body limit `io.LimitReader(resp.Body, 1<<22)` 적용 (4 MiB)
- `guard/gate` SQLite + scheduler 마이너 수정

### 📚 Documentation

- `CLAUDE.md` / `AGENTS.md`: Tiered Build System, `cq serve` 섹션, C7/C6/C8 문서화
- `README.md`: CQ 브랜딩 통일, `cq doctor` 섹션, 154 tools / 2,205 tests / 141.7K LOC 수치 현행화

---

## [0.6.0] - 2026-01-26

### Added

- **DDD-CLEANCODE Worker Packet (Phase 6)**: 구조화된 태스크 명세
  - `Goal`: 완료 조건 + 범위 외 명시
  - `ContractSpec`: API 계약 (input/output/errors) + 테스트 요구사항 (success/failure/boundary)
  - `BoundaryMap`: DDD 레이어 제약 (target_domain, allowed/forbidden_imports)
  - `CodePlacement`: 파일 위치 명세 (create/modify/tests)
  - `QualityGate`: 검증 명령어 (name/command/required)
  - `CheckpointDefinition`: CP1/CP2/CP3 마일스톤 정의
  - `c4_add_todo` MCP 도구에 DDD-CLEANCODE 필드 지원 추가
  - `dod` 필드 deprecated 경고 (goal 미사용 시)
  - 12개 통합 테스트로 저장/로드 검증 완료

- **c4-plan/c4-submit 스킬 강화**
  - Worker Packet 구조화된 입력 UI (4.1~4.5절)
  - 경계 검증 자동 실행 (forbidden imports)
  - 작업 분해 검증 (max 2일, max 3 APIs)
  - ContractSpec 검증 (테스트 커버리지)

- **Semantic Search Engine (Phase 6.5)**: TF-IDF 기반 자연어 코드 검색
  - `SemanticSearcher`: 자연어 쿼리로 코드 검색
  - 프로그래밍 동의어 확장 (auth → authentication, db → database 등)
  - 범위 지정 검색 (symbols, docs, code, files)
  - 관련 심볼 찾기 및 타입별 검색

- **Call Graph Analyzer (Phase 6.5)**: 함수 호출 관계 분석
  - `CallGraphAnalyzer`: 호출자/피호출자 분석
  - 함수 간 호출 경로 찾기
  - 호출 그래프 통계 (핫스팟, 진입점, 고립 함수)
  - Mermaid 다이어그램 생성

- **Enhanced MCP Tools (Phase 6.5)**: 12개 새 코드 분석 도구
  - `c4_semantic_search`: 자연어 코드 검색
  - `c4_find_related_symbols`: 관련 심볼 찾기
  - `c4_search_by_type`: 타입별 심볼 검색
  - `c4_get_callers`: 호출자 찾기
  - `c4_get_callees`: 피호출자 찾기
  - `c4_find_call_paths`: 호출 경로 찾기
  - `c4_call_graph_stats`: 호출 그래프 통계
  - `c4_call_graph_diagram`: Mermaid 다이어그램
  - `c4_find_definition`: 심볼 정의 찾기
  - `c4_find_references`: 참조 찾기
  - `c4_analyze_file`: 파일 심볼 분석
  - `c4_get_dependencies`: 의존성 분석

- **Long-Running Worker Detection (Phase 6.5)**: Heartbeat 기반 이상 탐지
  - Worker heartbeat 모니터링
  - 장기 실행 태스크 자동 감지
  - Stale worker 복구 메커니즘

- **Team Collaboration (Phase 6)**: Supabase 기반 팀 협업 지원
  - `SupabaseStateStore`: 분산 프로젝트 상태 관리
  - `SupabaseLockStore`: 분산 잠금 (RLS 적용)
  - `TeamService`: 팀 생성/수정/삭제, 멤버 RBAC
  - `CloudSupervisor`: 팀 전체 리뷰 및 체크포인트 관리
  - `TaskDispatcher`: 우선순위 기반 태스크 분배
  - 6개 Supabase 마이그레이션 (`00001` ~ `00006`)

- **Branding Middleware**: 화이트라벨 커스텀 도메인 지원
  - `BrandingMiddleware`: Host 헤더 기반 브랜딩 적용
  - `BrandingCache`: TTL 캐시 (기본 60초)
  - 팀별 로고, 색상, 도메인 설정

- **Code Analysis Engine**: Python/TypeScript 코드 분석
  - `PythonParser`: Python AST 분석
  - `TypeScriptParser`: TypeScript 구문 분석
  - 심볼 테이블, 의존성 그래프, 호출 관계

- **Documentation Server (MCP)**: 문서화 자동화
  - `query_docs`: 문서 검색/쿼리
  - `create_snapshot`: 코드베이스 스냅샷
  - `get_usage_examples`: 사용 예시 추출
  - Context7 스타일 REST API (`/api/docs`)

- **Gap Analyzer (MCP)**: 명세-구현 매핑
  - `analyze_spec_gaps`: EARS 요구사항 갭 분석
  - `generate_tests_from_spec`: 명세→테스트 생성
  - `link_impl_to_spec`: 구현-명세 연결
  - `verify_spec_completion`: 완료 검증

- **GitHub Integration 강화**
  - `GitHubClient`: 팀 권한 동기화
  - `GitHubAutomation`: 자동 PR/Issue 생성
  - 웹훅 이벤트 처리

- **Review-as-Task**: 리뷰가 태스크로 관리됩니다
  - 태스크 ID에 버전 번호 추가 (T-XXX → T-XXX-0)
  - 구현 태스크 완료 시 자동으로 리뷰 태스크(R-XXX-N) 생성
  - REQUEST_CHANGES 시 다음 버전 태스크 자동 생성 (T-XXX-1)
  - `max_revision` 설정으로 최대 수정 횟수 제한 (기본값: 3)
  - **리뷰 태스크 자동 라우팅**: `task_type="review"` 설정으로 `code-reviewer` 에이전트 자동 할당

- **Checkpoint-as-Task**: 체크포인트가 태스크로 처리됩니다
  - Phase의 모든 리뷰가 APPROVE되면 CP-XXX 태스크 자동 생성
  - Worker가 체크포인트 검증 수행 (E2E, HTTP 등)
  - APPROVE 시 Phase 완료 및 main 머지
  - REQUEST_CHANGES 시 문제 태스크의 다음 버전 생성
  - `checkpoint_as_task: true` 설정으로 활성화

- **TaskType Enum**: 태스크 유형 구분
  - `IMPLEMENTATION`: 구현 태스크 (T-XXX-N)
  - `REVIEW`: 리뷰 태스크 (R-XXX-N)
  - `CHECKPOINT`: 체크포인트 태스크 (CP-XXX)

- **Task Model 확장**
  - `base_id`: 기본 태스크 ID ("001")
  - `version`: 버전 번호 (0, 1, 2...)
  - `type`: TaskType enum
  - `task_type`: 스킬 매칭용 태스크 유형 ("review", "debug", "security" 등)
  - `phase_id`: Phase 식별자
  - `required_tasks`: CP가 검증할 태스크 목록
  - `review_decision`: 리뷰 결정 (APPROVE/REQUEST_CHANGES)

- **GraphRouter.use_legacy_fallback** 속성 추가
  - skill matcher와 rule engine 미설정 시 도메인 기반 라우팅만 사용

- **GitLab Integration**: GitLab MR 웹훅 및 AI 코드 리뷰
  - `GitLabClient`: REST API 클라이언트 (diff 조회, 노트/토론 생성, 라벨)
  - `GitLabProvider`: 통합 프로바이더 (OAuth, 웹훅 검증)
  - `MRReviewService`: AI 기반 코드 리뷰 서비스 (LiteLLM/Anthropic)
  - `/webhooks/gitlab` 엔드포인트 추가
  - X-Gitlab-Token 헤더 기반 웹훅 검증
  - 환경 변수: `GITLAB_PRIVATE_TOKEN`, `GITLAB_WEBHOOK_SECRET`, `GITLAB_URL`

### Changed

- MCP 도구 25개 이상으로 확장
- `c4_add_todo`가 정규화된 태스크 ID 반환 (T-XXX → T-XXX-0)
- SupervisorLoop이 `checkpoint_as_task` 모드에서 큐 항목만 정리 (직접 처리 안함)
- 체크포인트 검증이 Design 단계 요구사항 기반으로 DoD 자동 생성

### Fixed

- `test_add_todo` 관련 10개 테스트 수정 (정규화된 ID 사용)
- GraphRouter 누락 속성 `use_legacy_fallback` 추가

## [0.1.0] - 2026-01-15

### Added

- 초기 릴리스
- State Machine 워크플로우 (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- MCP Server 통합 (19개 도구)
- Multi-Worker SQLite WAL 기반 병렬 실행
- Agent Routing (도메인별 에이전트 자동 선택)
- EARS 요구사항 수집 (5가지 패턴)
- Multi-LLM Provider 지원 (LiteLLM 기반 100+)
- Checkpoint Gates (단계별 리뷰 포인트)
- Auto-Validation (자동 lint/test 실행)
- 다중 플랫폼 지원 (Claude Code, Cursor, Codex CLI, Gemini CLI, OpenCode)
