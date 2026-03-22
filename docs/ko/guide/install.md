# 설치

## Step 1 — 터미널 열기

::: code-group

```sh [Mac]
# ⌘ + Space → "Terminal" 입력 → Enter
# 또는: 응용 프로그램 → 유틸리티 → 터미널
```

```sh [Windows]
# 시작 → "Windows Terminal" 검색, 또는 Git Bash 실행
# Git Bash 다운로드: https://git-scm.com/downloads
```

```sh [Linux]
# Ctrl + Alt + T  (대부분의 배포판)
```

:::

## Step 2 — AI 코딩 어시스턴트 설치

CQ는 다음 도구들과 함께 동작합니다. 하나를 선택하세요:

- **[Claude Code](https://docs.anthropic.com/en/docs/claude-code/getting-started)** — 권장
- **[Gemini CLI](https://github.com/google-gemini/gemini-cli)** — `npm install -g @google/gemini-cli`
- **[Codex CLI](https://github.com/openai/codex)** — `npm install -g @openai/codex`

## Step 3 — CQ 설치

**지원 플랫폼**: macOS (Apple Silicon / Intel), Linux (x86\_64 / ARM64), Windows (Git Bash 경유)

## 한 줄 설치

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

설치 과정:
1. OS와 아키텍처 감지
2. GitHub Releases에서 `solo` 티어 바이너리 다운로드
3. `~/.local/bin/cq`에 설치
4. PATH에 `~/.local/bin` 추가 (`.zshrc` / `.bashrc` / `.profile`)
5. RC 파일에 셸 자동완성 추가 (`cq completion zsh/bash/fish`)
6. `cq doctor`로 환경 검증

새 터미널을 열고 확인:

```sh
cq --help
```

::: tip 환경 자동 수정
뭔가 제대로 설정되지 않았다면:
```sh
cq doctor --fix
```
CLAUDE.md, hooks, .mcp.json, Hub 인증을 한 번에 자동 패치합니다.
:::

## Step 4 — 첫 프로젝트 시작

```sh
cd 프로젝트-폴더
cq claude   # 또는: cq cursor / cq codex / cq gemini
```

무엇을 만들고 싶은지 말하면 됩니다. → [예시 보기](/ko/examples/first-task)

## 특정 티어 설치

```sh
# connected — Supabase, LLM Gateway, EventBus 추가
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected

# full — Supabase 워커 큐, Drive, CDP, GPU 포함 전체 기능
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

어떤 티어를 선택할지는 [티어](/ko/guide/tiers)를 참고하세요.

## 사용자 지정 디렉토리에 설치

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --install-dir /usr/local/bin
```

## 드라이 런 (미리보기)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --dry-run
```

## 업데이트

설치 명령을 다시 실행합니다. 인스톨러는 `~/.local/bin/cq` 바이너리만 교체하며, `~/.c4/`의 설정과 프로젝트 데이터는 수정되지 않습니다.

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 수동 설치

모든 바이너리 (3 티어 × 3 플랫폼 = 9개 파일)는 [GitHub Releases](https://github.com/PlayIdea-Lab/cq/releases/latest)에서 제공됩니다.

파일명 규칙: `cq-{tier}-{os}-{arch}`

| 플랫폼 | 예시 |
|--------|------|
| macOS Apple Silicon | `cq-solo-darwin-arm64`, `cq-connected-darwin-arm64`, `cq-full-darwin-arm64` |
| Linux x86_64 | `cq-solo-linux-amd64`, `cq-connected-linux-amd64`, `cq-full-linux-amd64` |
| Linux ARM64 | `cq-solo-linux-arm64`, `cq-connected-linux-arm64`, `cq-full-linux-arm64` |

```sh
chmod +x cq-solo-darwin-arm64
mv cq-solo-darwin-arm64 ~/.local/bin/cq
```
