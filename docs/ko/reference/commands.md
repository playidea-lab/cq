# 커맨드 레퍼런스

CQ는 프로젝트 설정, 세션 관리, 환경 제어를 위한 CLI(`cq`)와 Claude Code 및 호환 AI 도구 내부에서 사용되는 Skill(슬래시 커맨드)을 제공합니다.

---

## cq CLI

### `cq` / `cq claude`

CQ 프로젝트 초기화와 함께 Claude Code를 실행합니다.

```sh
cq                          # claude 실행 (telegram 없음)
cq -t <name>                # 이름이 있는 세션 (--session-id로 고정된 UUID)
cq --bot                    # 봇 메뉴 표시 → telegram 연결
cq --bot <name>             # 특정 telegram 봇 연결
cq -t <name> --bot <name>   # 이름 있는 세션 + telegram
```

### 다른 AI 도구

```sh
cq cursor    # Cursor 실행
cq codex     # OpenAI Codex CLI 실행
cq gemini    # Gemini CLI 실행
```

각 커맨드는 해당 도구의 `CLAUDE.md`, `.c4/`, Skill, MCP 설정을 생성합니다:

| 커맨드 | MCP 설정 | Agent 지침 |
|--------|---------|----------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI 설정) | `CLAUDE.md` |

> [AGENTS.md 표준](https://agents.md)을 지원하는 모든 도구(예: Gemini Code Assist)는 전용 `cq` 커맨드 없이 `CLAUDE.md`를 직접 읽을 수 있습니다.

---

### `cq auth`

CQ Cloud와 인증합니다 (GitHub OAuth).

```sh
cq auth login            # GitHub OAuth 플로우를 위해 브라우저 열기
cq auth login --device   # 헤드리스/SSH: user_code 표시 → 브라우저에서 입력 (RFC 8628)
cq auth login --link     # 인증 URL 직접 출력 → 수동으로 열기
cq auth logout           # 저장된 자격증명 지우기 (~/.c4/session.json)
cq auth status           # 현재 인증 상태 표시
```

---

### `cq secret`

API 키와 시크릿 관리 (`~/.c4/secrets.db`에 AES-256-GCM으로 저장).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret set hub.api_key <your-hub-key>   # Worker 큐 API 키
cq secret get anthropic.api_key
cq secret list
cq secret delete anthropic.api_key
```

키는 설정 파일에 저장되지 않습니다.

---

### `cq doctor`

환경 상태를 확인합니다 (13개 항목).

```sh
cq doctor           # 전체 보고서
cq doctor --json    # JSON 출력 (CI용)
cq doctor --fix     # 안전한 문제 자동 수정
```

| 확인 항목 | 검증 내용 |
|---------|---------|
| cq binary | 바이너리 존재 + 버전 |
| .c4 directory | 데이터베이스 파일 존재 |
| .mcp.json | 유효한 JSON + 바이너리 경로 존재 |
| CLAUDE.md | 파일 존재 + 심볼릭 링크 유효 |
| hooks | Gate + permission-reviewer 훅 설치됨 |
| Python sidecar | `uv` 사용 가능 |
| Supabase Worker Queue | Supabase 연결 + Worker 큐 상태 |
| Supabase | 클라우드 설정 + 연결 |
| os-service | LaunchAgent / systemd 서비스 설치 및 실행 |
| tool-socket | UDS 소켓 응답 (`cq serve` 실행 중) |
| zombie-serve | 고아 serve 프로세스 없음 |
| sidecar | Python 사이드카 멈추지 않음 |
| skill-health | 평가된 모든 Skill이 트리거 임계값(>= 0.90) 통과 |

---

### `cq serve`

백그라운드 서비스를 실행합니다 (EventBus, GPU 스케줄러, Agent 리스너).

```sh
cq serve              # :4140에서 시작
cq serve --port 4141  # 커스텀 포트
cq serve install      # OS 서비스로 설치 (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall    # OS 서비스 제거
cq serve start        # OS 서비스 시작
cq serve stop         # OS 서비스 중지
cq serve status       # OS 서비스 상태 및 수동 serve 프로세스 상태 표시
```

헬스 엔드포인트: `GET http://127.0.0.1:4140/health` — 모든 등록된 컴포넌트 상태 반환.

```sh
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"eventbus":{"status":"ok"}}}
```

`cq serve`로 관리하는 컴포넌트:

| 컴포넌트 | 설정 키 | 설명 |
|---------|--------|------|
| eventbus | `serve.eventbus.enabled` | C3 gRPC EventBus 데몬 (UDS) |
| gpu | `serve.gpu.enabled` | GPU/CPU 작업 스케줄러 |
| agent | `serve.agent.enabled` | Supabase Realtime @cq 언급 디스패처 |
| hub | `serve.hub.enabled` | Hub 분산 작업 큐 폴러 |
| relay | `relay.enabled` | WebSocket relay 클라이언트 (NAT 통과) |
| stale_checker | `serve.stale_checker.enabled` | 멈춘 태스크 감지기 |
| cron | (hub와 함께) | Cron 스케줄 폴러 |

---

### `cq hub`

분산 Hub 작업과 Worker를 관리합니다.

```sh
cq hub status             # Hub 연결 및 큐 상태 표시
cq hub workers            # 연결된 Worker 목록 (affinity 점수 포함)
cq hub list               # 최근 작업 목록
```

---

### `cq sessions`

이름이 있는 Claude Code 세션을 나열합니다.

```sh
cq sessions
```

예시 출력:

```
  my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  [2 unread]
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    훈련 데이터 파이프라인 분석 중
```

- `[N unread]`는 읽지 않은 세션 간 메일 수를 표시
- 메모(설정된 경우)는 다음 줄에 표시

---

### `cq session`

이름이 있는 Claude Code 세션을 관리합니다.

```sh
cq session name <session-name>              # 현재 세션에 이름 붙이기
cq session name <session-name> -m "메모"   # 메모와 함께 이름 붙이기
cq session rm <session-name>               # 이름 있는 세션 제거
```

세션은 `cq -t <session-name>`으로 재개할 수 있습니다.

에디터를 떠나지 않고 세션에 이름을 붙이려면 Claude Code 안에서 `/c4-attach`를 사용하세요.

---

### `cq mail`

Claude Code 세션 간 메시지를 전달하는 세션 간 메일.

```sh
cq mail send <to> <body>   # 이름 있는 세션에 메시지 보내기
cq mail ls                 # 메시지 목록 (읽지 않은 수 표시)
cq mail read <id>          # 메시지 읽기 (읽음으로 표시)
cq mail rm <id>            # 메시지 삭제
```

---

### `cq ls`

등록된 Telegram 봇을 나열합니다.

```sh
cq ls
```

---

### `cq setup`

알림을 위한 Telegram 봇을 페어링합니다.

```sh
cq setup
```

Telegram 봇 페어링 안내: 봇 토큰 입력, 봇 시작, CQ가 채팅 ID를 자동으로 감지합니다.

---

### `cq pop`

Personal Ontology Pipeline 상태 및 제어.

```sh
cq pop status   # 게이지 값, 파이프라인 상태, 지식 통계 표시
```

---

### `cq version`

현재 바이너리 버전과 빌드 티어를 출력합니다.

```sh
cq version
```

---

### `cq update`

CQ 바이너리를 최신 버전으로 업데이트합니다.

```sh
cq update
```

---

### `cq completion`

셸 자동완성 스크립트를 생성합니다.

```sh
cq completion zsh   # zsh 자동완성 스크립트
cq completion bash  # bash 자동완성 스크립트
cq completion fish  # fish 자동완성 스크립트
```

`cq init`과 `install.sh`가 `~/.zshrc` / `~/.bashrc`에 자동으로 자동완성을 추가합니다.

---

## Skill (Claude Code 슬래시 커맨드)

Skill은 Claude Code 안에서 호출하는 슬래시 커맨드입니다. 모든 Skill은 CQ 바이너리에 내장되어 있어 설치 후 인터넷이 필요 없습니다.

### 핵심 워크플로우

| Skill | 트리거 | 사용 가능한 상태 | 설명 |
|-------|--------|---------------|------|
| `/pi` | play idea, ideation | 모든 상태 | 계획 전 브레인스토밍. 발산/수렴/조사/토론. 자동으로 `/c4-plan` 실행. |
| `/c4-plan` | plan, design | INIT, HALTED | Discovery -> Design -> Lighthouse 계약 -> 태스크 생성. |
| `/c4-run` | run, execute | PLAN, HALTED, EXECUTE | 대기 중인 태스크를 위한 Worker 스폰. 큐가 빌 때까지 지속. |
| `/c4-finish` | finish, complete | 구현 후 | 빌드 -> 테스트 -> 문서 -> 커밋. 구현 후 완료 처리. |
| `/c4-status` | status | 모든 상태 | 시각적 태스크 그래프, 큐 요약, Worker 상태. |
| `/c4-quick` | quick | PLAN, HALTED, EXECUTE | 태스크 즉시 생성 + 할당, 계획 건너뜀. |

### 품질 루프

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-checkpoint` | (자동) | 4렌즈 리뷰: 전체적/사용자 흐름/파급 효과/출시 준비. |
| `/c4-validate` | validate | lint + 테스트 실행. CRITICAL은 커밋 차단, HIGH는 리뷰 필요. |
| `/c4-review` | review | 6축 평가를 통한 종합적 3-pass 코드 리뷰. |
| `/c4-polish` | polish | *(Deprecated — `/c4-finish`에 내장됨)* |
| `/c4-refine` | refine | *(Deprecated — `/c4-finish`에 내장됨)* |

### 태스크 관리

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-add-task` | add task | DoD, 범위, 도메인과 함께 태스크 대화형 추가. |
| `/c4-submit` | submit | 자동 유효성 검사와 함께 완료된 태스크 제출. |
| `/c4-interview` | interview | 깊이 있는 요구사항 인터뷰 (PM/아키텍트 모드). |
| `/c4-stop` | stop | 실행 중지, HALTED로 전환. 진행 상황 보존. |
| `/c4-clear` | clear | C4 상태 초기화. 태스크, 이벤트, 잠금 지우기. |

### 협업

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-swarm` | swarm | 코디네이터 주도 Agent 팀 스폰. 모드: 구현, 리뷰, 조사. |
| `/c4-standby` | standby | Supabase를 통해 세션을 분산 Worker로 전환. *full 티어만* |

### 세션 및 유틸리티

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/init` | init | 현재 프로젝트에 C4 초기화. |
| `/c4-attach` | attach | 현재 세션에 이름 붙이기. |
| `/c4-reboot` | reboot | 현재 이름 있는 세션 재부팅. |
| `/c4-release` | release | git 이력에서 CHANGELOG 생성. |
| `/c4-help` | help | 모든 Skill과 MCP 도구 빠른 참조. |

### 연구

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/research-loop` | research loop | 논문-실험 개선 루프. |
| `/c2-paper-review` | paper review | *(Deprecated — `/c4-review` 사용)* |

### C9 Research Loop (ML)

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c9-init` | c9-init | C9 ML 연구 프로젝트 초기화. `state.yaml` 생성. |
| `/c9-loop` | c9-loop | 메인 루프 드라이버 — 현재 단계를 읽고 다음 단계 자동 실행. |
| `/c9-run` | c9-run | 실험 YAML을 Supabase Worker 큐에 제출. |
| `/c9-check` | c9-check | 결과 파싱 + 수렴 확인. |
| `/c9-standby` | c9-standby | RUN 단계에서 대기; 훈련 완료 시 CHECK 자동 트리거. |
| `/c9-finish` | c9-finish | 최적 모델 저장 + 결과 문서화. |
| `/c9-steer` | c9-steer | `state.yaml` 직접 수정 없이 단계 변경. |
| `/c9-survey` | c9-survey | Gemini Google Search grounding으로 arXiv + SOTA 탐색. |
| `/c9-report` | c9-report | 원격 서버에서 실험 결과 수집. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) 토론 모드. |
| `/c9-deploy` | c9-deploy | 엣지 서버에 최적 모델 배포. |

---

## 상태 머신

Skill은 프로젝트 상태 머신을 준수합니다:

```
INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> REFINE -> POLISH -> COMPLETE
```

| 상태 | 사용 가능한 Skill |
|------|---------------|
| INIT | `/init`, `/c4-plan` |
| DISCOVERY / DESIGN | `/c4-plan` (자동 진행) |
| PLAN | `/c4-run`, `/c4-quick`, `/c4-status` |
| EXECUTE | `/c4-run`, `/c4-quick`, `/c4-stop`, `/c4-status`, `/c4-validate`, `/c4-submit`, `/c4-add-task`, `/c4-swarm` |
| CHECKPOINT | `/c4-checkpoint`, `/c4-add-task` |
| HALTED | `/c4-run`, `/c4-quick`, `/c4-plan` |
| COMPLETE | `/c4-finish`, `/c4-release` |
