# 빠른 시작

5분 안에 첫 번째 CQ 관리 태스크를 완성합니다.

## 1단계: 프로젝트 초기화

프로젝트 디렉토리에서 터미널을 열고 AI 도구에 맞는 명령을 실행합니다:

```sh
cd your-project
cq claude   # Claude Code
cq cursor   # Cursor
cq codex    # OpenAI Codex CLI
```

각 명령은 `.CLAUDE.md`, `.c4/`, 그리고 해당 도구의 MCP 설정 파일을 생성합니다:

| 명령 | MCP 설정 | 에이전트 지침 |
|------|---------|--------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |

그 다음 **AI 도구를 재시작**하여 새 MCP 서버를 불러옵니다.

::: tip 다른 AI 도구
[AGENTS.md 표준](https://agents.md)을 지원하는 모든 도구는 `CLAUDE.md`를 직접 읽을 수 있습니다 — `cq init` 불필요.
:::

## 1.5단계: 로그인 (connected / full 티어)

`connected` 또는 `full` 티어를 사용한다면 한 번 인증합니다:

```sh
cq auth login
```

브라우저에서 GitHub OAuth를 열고 `.c4/config.yaml`에 `cloud.enabled`, `url`, `anon_key`를 자동으로 설정합니다. 로그인 후 시작 시:

```
✓ Cloud: user@example.com (expires in 47h)
```

`solo` 티어는 이 단계를 건너뛰세요 — 로그인 불필요.

## 2단계: 연결 확인

Claude Code에서 실행:

```
/c4-status
```

프로젝트 상태와 빈 태스크 큐가 표시됩니다.

## 3단계: 기능 계획

만들고 싶은 것을 설명합니다:

```
/c4-plan "JWT 인증 추가"
```

CQ가:
1. 명확화 질문을 합니다 (Discovery 단계)
2. 접근 방식을 설계합니다 (Design 단계)
3. 완료 조건(DoD)이 있는 태스크로 분해합니다
4. 태스크 큐를 생성합니다

## 4단계: 실행

```
/c4-run
```

워커가 자동으로 시작됩니다 — 태스크당 하나씩, 각자 격리된 git 워크트리에서 실행됩니다. 큐가 비워지면 `/c4-run`이 자동으로 polish (변경 없을 때까지 수정)를 실행한 뒤 finish (빌드 · 테스트 · 문서 · 커밋)를 수행합니다.

진행 상황 확인:

```
/c4-status
```

이게 전부입니다. `/c4-run`이 구현, 리뷰, polish, finish를 end-to-end로 처리합니다.

---

이후 수동 변경이 필요하다면:

```
/c4-finish
```

---

## 최소 예시 (단일 태스크)

소규모 작업은 계획 단계를 건너뜁니다:

```
/c4-quick "모바일에서 로그인 버튼 클릭이 안 돼"
```

하나의 태스크를 생성하고, 워커에게 할당하여 즉시 실행합니다.

## 다음

- [티어 이해하기 →](/ko/guide/tiers)
- [전체 워크플로우 학습 →](/ko/workflow/)
