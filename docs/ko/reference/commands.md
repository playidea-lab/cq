# 명령어 레퍼런스

## cq CLI

### `cq` / `cq claude`

Claude Code를 CQ 프로젝트 초기화와 함께 실행합니다.

```sh
cq                          # Claude 실행 (텔레그램 없음)
cq -t <name>                # Named 세션 (--session-id로 UUID 고정)
cq --bot                    # 봇 메뉴 → 텔레그램 연결
cq --bot <name>             # 특정 봇으로 텔레그램 연결
cq -t <name> --bot <name>   # Named 세션 + 텔레그램
```

### 기타 AI 도구

```sh
cq cursor    # Cursor
cq codex     # OpenAI Codex CLI
cq gemini    # Gemini CLI
```

각 명령은 `CLAUDE.md`, `.c4/`, 스킬, 그리고 해당 도구의 MCP 설정 파일을 생성합니다:

| 명령 | MCP 설정 | 에이전트 지침 |
|------|---------|--------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI 설정) | `CLAUDE.md` |

> [AGENTS.md 표준](https://agents.md)을 지원하는 모든 도구 (예: Gemini Code Assist)는 전용 `cq` 명령 없이도 `CLAUDE.md`를 직접 읽을 수 있습니다.

### `cq doctor`

환경 상태를 점검합니다 (8가지 항목).

```sh
cq doctor           # 전체 리포트
cq doctor --json    # JSON 출력 (CI용)
cq doctor --fix     # 안전한 이슈 자동 수정
```

| 점검 항목 | 확인 내용 |
|---------|---------|
| cq binary | 바이너리 존재 + 버전 |
| .c4 directory | 데이터베이스 파일 존재 |
| .mcp.json | 유효한 JSON + 바이너리 경로 존재 |
| CLAUDE.md | 파일 존재 + 심링크 유효 |
| hooks | 게이트 + 권한 리뷰어 훅 설치됨 |
| Python sidecar | `uv` 사용 가능 |
| Supabase 워커 큐 | Supabase 연결 + 워커 큐 상태 |
| Supabase | 클라우드 설정 + 연결 |
| os-service | LaunchAgent / systemd 서비스 설치 및 실행 중 |
| tool-socket | UDS 소켓 응답 (`cq serve` 실행 중) |
| zombie-serve | 고아 serve 프로세스 없음 |
| sidecar | Python sidecar 행 없음 |
| skill-health | 평가된 모든 스킬 트리거 임계값 통과 (≥ 0.90) |

### `cq secret`

API 키와 시크릿 관리 (`~/.c4/secrets.db`에 저장, AES-256-GCM).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret set hub.api_key <hub-키>          # 워커 큐 API 키 (config.yaml 평문보다 우선)
cq secret get anthropic.api_key
cq secret list
cq secret delete anthropic.api_key
```

키는 설정 파일에 저장되지 않습니다.

### `cq auth`

C4 Cloud 인증 (GitHub OAuth).

```sh
cq auth login    # GitHub OAuth 흐름을 위해 브라우저 열기
cq auth logout   # 저장된 자격증명 삭제 (~/.c4/session.json)
cq auth status   # 현재 인증 상태 표시
```

### `cq ls`

등록된 텔레그램 봇 목록을 표시합니다.

```sh
cq ls
```

### `cq sessions`

이름 붙은 Claude Code 세션 목록을 표시합니다.

```sh
cq sessions
```

출력 예시:

```
  my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  ✉2
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    데이터 파이프라인 분석 중
```

- `✉N` 미읽은 세션 간 메일 수를 표시
- 메모(설정된 경우)가 아래 줄에 표시됨

### `cq session`

이름 붙은 Claude Code 세션 관리.

```sh
cq session name <session-name>              # 현재 세션에 이름 붙이기
cq session name <session-name> -m "메모"   # 메모와 함께 이름 붙이기
cq session rm <session-name>               # 이름 붙은 세션 삭제
```

`cq -t <session-name>`으로 세션을 재개할 수 있습니다.

Claude Code 내에서 편집기를 벗어나지 않고 세션 이름을 붙이려면 `/c4-attach`를 사용합니다.

### `cq mail`

Claude Code 세션 간 메시지를 주고받는 세션 간 메일.

```sh
cq mail send <to> <body>   # 이름 붙은 세션에 메시지 전송
cq mail ls                 # 메시지 목록 (미읽은 수 표시)
cq mail read <id>          # 메시지 읽기 (읽음 표시)
cq mail rm <id>            # 메시지 삭제
```

### `cq setup`

Telegram 봇 알림을 페어링합니다.

```sh
cq setup
```

Telegram 봇 페어링을 안내합니다: 봇 토큰 입력, 봇 시작, CQ가 채팅 ID를 자동 감지합니다. 설정 후 실험 및 태스크 알림이 Telegram으로 전송됩니다.

### `cq serve`

백그라운드 서비스 실행 (EventBus, GPU 스케줄러, Agent 리스너).

```sh
cq serve              # :4140에서 시작
cq serve --port 4141
cq serve install      # OS 서비스로 설치 (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall    # OS 서비스 삭제
cq serve status       # OS 서비스 상태 및 수동 serve 프로세스 상태 표시
```

Health 엔드포인트: `GET http://127.0.0.1:4140/health` — 등록된 모든 컴포넌트 상태를 반환합니다.

```sh
# 컴포넌트 health 확인
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"eventbus":{"status":"ok"}}}
```

### `cq pop`

Personal Ontology Pipeline 상태 확인.

```sh
cq pop status   # gauge 값, 파이프라인 상태, 지식 통계 표시
```

### `cq version`

현재 바이너리 버전과 빌드 티어를 출력합니다.

---

## 스킬 (Claude Code 슬래시 명령)

스킬은 Claude Code 내에서 `/skill-name`으로 호출합니다.

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/pi` | "play idea", "아이디어", "ideation" | 계획 전 아이디어 발산·수렴 |
| `/c4-plan` | "계획", "plan", "설계" | Discovery → Design → Tasks |
| `/c4-run` | "실행", "run", "ㄱㄱ" | pending 태스크를 위한 워커 스폰 |
| `/c4-finish` | "마무리", "finish" | 빌드 → 테스트 → 문서 → 커밋 |
| `/c4-status` | "상태", "status" | 시각적 태스크 진행 상황 |
| `/c4-quick` | "quick", "빠르게" | 단일 태스크, 계획 없이 |
| `/c4-polish` | "polish" | *(Deprecated — `/c4-finish`에 내장됨)* |
| `/c4-refine` | "refine" | *(Deprecated — `/c4-finish`에 내장됨)* |
| `/c4-checkpoint` | 체크포인트 도달 시 | 승인 / 변경 요청 / 재계획 |
| `/c4-validate` | "검증", "validate" | lint + 테스트 실행 |
| `/c4-review` | "review" | 6축 평가 3-pass 코드 리뷰 |
| `/c4-add-task` | "태스크 추가" | 대화형 태스크 추가 |
| `/c4-submit` | "제출", "submit" | 완료된 태스크 제출 |
| `/c4-interview` | "interview" | 심층 요구사항 인터뷰 |
| `/c4-stop` | "stop", "중단" | 실행 중단, 진행 상황 보존 |
| `/c4-clear` | "clear" | 디버깅용 C4 상태 초기화 |
| `/c4-swarm` | "swarm" | 코디네이터 주도 에이전트 팀 스폰 |
| `/c4-standby` | "대기", "standby" | Supabase 기반 분산 워커로 변환 (full 티어) |
| `/c4-attach` | "세션 이름", "attach" | 나중에 재개할 현재 세션 이름 붙이기 |
| `/c4-reboot` | "reboot", "재시작" | 현재 이름 붙은 세션 재부팅 |
| `/init` | "init", "초기화" | 현재 프로젝트에 C4 초기화 |
| `/c4-release` | "release" | git 히스토리에서 CHANGELOG 생성 |
| `/c4-help` | "help" | 모든 스킬 빠른 레퍼런스 |
| `/c2-paper-review` | "논문 리뷰", "paper review" | 학술 논문 리뷰 (C2 라이프사이클) |
| `/research-loop` | "research loop" | 논문-실험 개선 루프 |
| `/c9-init` | "c9-init" | C9 연구 프로젝트 초기화 |
| `/c9-loop` | "c9-loop" | 메인 루프 드라이버 |
| `/c9-run` | "c9-run" | 실험 YAML을 Supabase 워커 큐에 제출 |
| `/c9-check` | "c9-check" | 실험 결과 파싱 + 수렴 판정 |
| `/c9-standby` | "c9-standby" | RUN phase 대기 |
| `/c9-finish` | "c9-finish" | 연구 루프 완료 + 문서화 |
| `/c9-steer` | "c9-steer" | phase 전환 |
| `/c9-survey` | "c9-survey" | 최신 논문·SOTA 수집 |
| `/c9-report` | "c9-report" | 원격 실험 결과 수집 |
| `/c9-conference` | "c9-conference" | 합의 토론 시뮬레이션 |
| `/c9-deploy` | "c9-deploy" | best model edge 배포 |
