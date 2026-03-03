# 설치

## 요구 사항

- macOS Apple Silicon (arm64) 또는 Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) CLI 설치됨 — [여기서 받기](https://docs.anthropic.com/en/docs/claude-code/getting-started)
- 셸에서 `curl` 사용 가능

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

## 특정 티어 설치

```sh
# connected — Supabase, LLM Gateway, EventBus 추가
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected

# full — Hub, Drive, CDP, GPU 포함 전체 기능
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
