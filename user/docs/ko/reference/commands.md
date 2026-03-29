# 명령어 레퍼런스

## cq CLI

> 현재 버전: **v1.46**

### `cq` / `cq claude`

CQ 프로젝트 초기화와 함께 Claude Code를 실행합니다.

```sh
cq                          # claude 실행 (telegram 없이)
cq -t <name>                # 이름 붙인 세션 (--session-id로 고정 UUID 사용)
cq --bot                    # 봇 메뉴 표시 → telegram 연결
cq --bot <name>             # 특정 telegram 봇 연결
cq -t <name> --bot <name>   # 이름 붙인 세션 + telegram
```

### 다른 AI 도구

```sh
cq cursor    # Cursor
cq codex     # OpenAI Codex CLI
cq gemini    # Gemini CLI
```

각 명령은 해당 도구를 위한 `CLAUDE.md`, `.c4/`, 스킬, MCP 설정을 생성합니다:

| 명령 | MCP 설정 | 에이전트 지침 |
|------|---------|-------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI 설정) | `CLAUDE.md` |

> [AGENTS.md 표준](https://agents.md)을 지원하는 모든 도구(예: Gemini Code Assist)는 별도의 `cq` 명령 없이 `CLAUDE.md`를 직접 읽을 수 있습니다.

### `cq doctor`

환경 상태 확인 (13개 항목).

```sh
cq doctor           # 전체 리포트
cq doctor --json    # JSON 출력 (CI용)
cq doctor --fix     # 안전한 문제 자동 수정
```

| 확인 항목 | 검증 내용 |
|---------|---------|
| cq binary | 바이너리 존재 + 버전 |
| .c4 directory | 데이터베이스 파일 존재 여부 |
| .mcp.json | 유효한 JSON + 바이너리 경로 존재 |
| CLAUDE.md | 파일 존재 + 심볼릭 링크 유효 |
| hooks | Gate + permission-reviewer hook 설치 여부 |
| Python sidecar | `uv` 사용 가능 여부 |
| Supabase Worker Queue | Supabase 연결 + Worker 큐 상태 |
| Supabase | 클라우드 설정 + 연결 |
| os-service | LaunchAgent / systemd 서비스 설치 및 실행 여부 |
| tool-socket | UDS 소켓 응답 여부 (`cq serve` 실행 중) |
| zombie-serve | 고아 serve 프로세스 없음 |
| sidecar | Python sidecar 멈춤 없음 |
| skill-health | 평가된 모든 스킬이 트리거 임계값 통과 (≥ 0.90) |

### `cq update`

CQ를 최신 릴리즈로 자가 업데이트합니다.

```sh
cq update           # 최신 버전 확인 및 설치
cq update --check   # 확인만, 설치 안 함
```

### `cq secret`

API 키 및 시크릿 관리 (`~/.c4/secrets.db`에 저장, AES-256-GCM).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret set hub.api_key <your-hub-key>   # Worker 큐 API 키 (config.yaml보다 권장)
cq secret get anthropic.api_key
cq secret list
cq secret delete anthropic.api_key
```

키는 설정 파일에 저장되지 않습니다.

### `cq auth`

C4 Cloud 인증 (GitHub OAuth).

```sh
cq auth login    # GitHub OAuth 플로우를 위해 브라우저 열기
cq auth logout   # 저장된 자격증명 초기화 (~/.c4/session.json)
cq auth status   # 현재 인증 상태 표시
```

### `cq ls`

등록된 Telegram 봇 목록 조회.

```sh
cq ls
```

### `cq sessions`

이름 붙인 Claude Code 세션 목록 조회.

```sh
cq sessions
```

출력 예시:

```
  my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  ✉2
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    Analyzing training data pipeline
```

- `✉N`은 읽지 않은 세션 간 메일 수를 표시합니다
- 메모(설정된 경우) 아래 줄에 표시됩니다

### `cq session`

이름 붙인 Claude Code 세션 관리.

```sh
cq session name <session-name>              # 현재 세션에 이름 부여
cq session name <session-name> -m "memo"   # 메모와 함께 이름 부여
cq session rm <session-name>               # 이름 붙인 세션 삭제
```

세션은 `cq -t <session-name>`으로 재개할 수 있습니다.

에디터를 떠나지 않고 세션 이름을 붙이려면 Claude Code 내에서 `/c4-attach`를 사용하세요.

### `cq mail`

Claude Code 세션 간 메시지를 전달하는 세션 간 메일.

```sh
cq mail send <to> <body>   # 이름 붙인 세션에 메시지 전송
cq mail ls                 # 메시지 목록 (읽지 않은 수 표시)
cq mail read <id>          # 메시지 읽기 (읽음 표시)
cq mail rm <id>            # 메시지 삭제
```

### `cq jobs`

Hub 작업 모니터링을 위한 BubbleTea TUI.

```sh
cq jobs          # 대화형 작업 모니터 열기
cq jobs --watch  # 자동 새로고침 모드
```

기능: 작업별 메트릭이 있는 상세 패널, 터미널 높이에 맞는 적응형 멀티행 차트, 작업 간 나란히 비교하는 compare 모드.

### `cq workers`

Worker Connection Board TUI — 연결된 Hub Worker를 확인하고 관리합니다.

```sh
cq workers       # Worker 보드 열기
```

Worker별 상태, heartbeat, 현재 작업 할당, 리소스 사용률을 표시합니다.

### `cq dashboard`

보드 메뉴가 포함된 통합 TUI 대시보드 — 모든 모니터링 뷰의 진입점.

```sh
cq dashboard     # 대시보드 열기 (보드 메뉴 → jobs / workers / pop / sessions)
```

### `cq setup`

알림을 위한 Telegram 봇 연결.

```sh
cq setup
```

Telegram 봇 연결 과정을 안내합니다: 봇 토큰 입력, 봇 시작, CQ가 채팅 ID 자동 감지. 설정 후 CQ는 Telegram을 통해 실험 및 태스크 알림을 전송합니다.

### `cq serve`

백그라운드 서비스 실행 (EventBus, GPU 스케줄러, 에이전트 리스너, Relay).

```sh
cq serve                  # :4140 포트로 시작
cq serve --port 4141
cq serve --watchdog       # Watchdog이 포함된 OS 서비스: relay 장애 시 자동 재시작, heartbeat 자가 복구
cq serve install          # OS 서비스로 설치 (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall        # OS 서비스 제거
cq serve status           # OS 서비스 상태 및 수동 serve 프로세스 상태 표시
```

설정에서 hub가 활성화되면 Worker가 hub와 함께 자동으로 활성화됩니다.

헬스 엔드포인트: `GET http://127.0.0.1:4140/health` — 등록된 모든 컴포넌트의 상태 반환.

```sh
# 컴포넌트 상태 확인
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"eventbus":{"status":"ok"}}}
```

### `cq pop`

Personal Ontology Pipeline 상태 및 제어.

```sh
cq pop status   # 게이지 값, 파이프라인 상태, 지식 통계 표시
```

### `cq craft`

스킬 마켓플레이스 — 커뮤니티 스킬을 게시, 검색, 설치합니다.

```sh
cq craft publish   # 마켓플레이스에 스킬 게시
cq craft search    # 사용 가능한 스킬 검색
cq craft install   # 마켓플레이스에서 스킬 설치
```

### `cq version`

현재 바이너리 버전 및 빌드 티어 출력.

---

## 스킬 (Claude Code 슬래시 명령)

스킬은 Claude Code 내에서 `/skill-name`으로 호출합니다.

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/pi` | "play idea", "아이디어", "ideation" | 계획 전 아이디어 발산; `/c4-plan` 자동 실행 |
| `/c4-plan` | "계획", "plan", "설계" | Discovery → Design → 태스크 생성 |
| `/c4-run` | "실행", "run", "ㄱㄱ" | 대기 중인 태스크에 Worker 스폰 |
| `/c4-finish` | "마무리", "finish" | 빌드 → 테스트 → 문서 → 커밋 |
| `/c4-status` | "상태", "status" | 시각적 태스크 진행 상황 |
| `/c4-quick` | "quick", "빠르게" | 단일 태스크, 계획 단계 없음 |
| `/c4-polish` | "polish" | *(폐기됨 — `/c4-finish`에 통합)* |
| `/c4-refine` | "refine" | *(폐기됨 — `/c4-finish`에 통합)* |
| `/c4-checkpoint` | 체크포인트 도달 시 | 승인 / 변경 요청 / 재계획 |
| `/c4-validate` | "검증", "validate" | lint + 테스트 실행 |
| `/c4-review` | "review" | 6축 평가로 3단계 코드 리뷰 |
| `/company-review` | "PR 리뷰", "diff 리뷰" | PI Lab 표준 코드 리뷰 |
| `/c4-submit` | "제출", "submit" | 완료된 태스크 제출 |
| `/simplify` | "simplify", "단순화" | 변경된 코드의 품질 및 효율성 검토 |
| `/c4-add-task` | "태스크 추가" | 대화형으로 태스크 추가 |
| `/c4-stop` | "stop", "중단" | 실행 중단, 진행 상황 보존 |
| `/c4-clear` | "clear" | 디버깅을 위한 C4 상태 초기화 |
| `/c4-swarm` | "swarm" | 코디네이터 주도 에이전트 팀 스폰 |
| `/c4-standby` | "대기", "standby" | Supabase를 통해 분산 Worker로 전환 (full 티어) |
| `/done` | "done", "세션 종료" | 완전 캡처와 함께 세션 완료 표시 |
| `/c4-attach` | "세션 이름", "attach" | 나중에 재개할 수 있도록 현재 세션에 이름 부여 |
| `/c4-reboot` | "reboot", "재시작" | 현재 이름 붙인 세션 재시작 |
| `/session-distill` | "session distill", "세션 요약" | 세션을 지속적 지식으로 정제 |
| `/init` | "init", "초기화" | 현재 프로젝트에 C4 초기화 |
| `/c4-release` | "release" | git 이력에서 CHANGELOG 생성 |
| `/c4-help` | "help" | 전체 스킬 빠른 참조 |
| `/claude-md-improver` | "CLAUDE.md 개선" | CLAUDE.md 분석 및 개선 |
| `/skill-tester` | "skill tester", "스킬 테스트" | 스킬 품질 테스트 및 평가 |
| `/pr-review` | "PR 만들어", "PR 체크리스트" | PR/MR 생성 체크리스트 및 리뷰 가이드 |
| `/craft` | "craft", "스킬 만들어줘" | 대화형으로 스킬, 에이전트, 규칙 생성 |
| `/tdd-cycle` | "TDD", "RED-GREEN-REFACTOR" | TDD 사이클 가이드 |
| `/debugging` | "debugging", "디버깅" | 체계적 디버깅 워크플로우 |
| `/spec-first` | "spec-first", "설계 문서" | Spec-First 개발 가이드 |
| `/incident-response` | "incident", "장애", "서버 다운" | 프로덕션 장애 대응 워크플로우 |
| `/c2-paper-review` | "논문 리뷰", "paper review" | 학술 논문 리뷰 (폐기됨) |
| `/research-loop` | "research loop" | 논문-실험 개선 루프 |
| `/experiment-workflow` | "experiment workflow" | 엔드투엔드 실험 생명주기 |
| `/c9-init` | "c9-init" | C9 ML 연구 프로젝트 초기화 |
| `/c9-loop` | "c9-loop" | ML 연구 메인 루프 드라이버 |
| `/c9-survey` | "c9-survey" | Gemini로 arXiv + SOTA 조사 |
| `/c9-conference` | "c9-conference" | Claude + Gemini 연구 토론 |
| `/c9-steer` | "c9-steer" | YAML 편집 없이 페이즈 전환 |
| `/c9-report` | "c9-report" | 원격 실험 결과 수집 |
| `/c9-finish` | "c9-finish" | 최고 모델 저장 + 문서화 |
| `/c9-deploy` | "c9-deploy" | 최고 모델을 엣지 서버에 배포 |
