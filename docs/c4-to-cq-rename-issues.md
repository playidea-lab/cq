# c4 → cq 이름 변경 후 점검 목록

**반영 완료**: 아래 항목들은 2026-02-19에 수정 반영되었습니다.

프로젝트/CLI 이름을 c4에서 cq로 바꾼 뒤 문제되거나 일관성이 깨진 부분 정리.
(**c4-core**, **cmd/c4** 디렉터리/패키지명, MCP 도구 접두사 **c4_*** 는 C4 엔진 쪽이라 유지.)

---

## 1. 반드시 수정 (경로/실행 파일 오류)

| 파일 | 현재 | 수정 |
|------|------|------|
| **.mcp.json** (루트) | `"command": "/Users/changmin/.local/bin/c4"`, `--dir","/Users/changmin/git/c4"`, `C4_PROJECT_ROOT":"/Users/changmin/git/c4"` | `cq`, `/Users/changmin/git/cq`, `C4_PROJECT_ROOT": "/Users/changmin/git/cq"` |
| **c4-core/.mcp.json** | command/args/env에 `git/c4` | `git/cq` |
| **install.sh** | `C4_REPO=".../pi/c4.git"`, `C4_DEFAULT_DIR="$HOME/c4"` | 리포가 cq로 이전했으면 `cq.git`, `$HOME/cq` (또는 유지하고 URL만 실제 리포에 맞게) |
| **.claude/skills/c4-finish/SKILL.md** | `go build -o ~/.local/bin/c4 ./cmd/c4/` | `~/.local/bin/cq` |
| **scripts/codex/setup-mcp.sh** | `C4_BIN="$PROJECT_ROOT/c4-core/bin/c4"`, `go build -o bin/c4` | `bin/cq` |

---

## 2. 문서/가이드 — CLI·바이너리 이름 (c4 → cq)

다음 파일에서 **CLI 명령·바이너리 경로**를 `c4` → `cq`로 통일하는 것이 좋습니다.

| 파일 | 수정 내용 |
|------|-----------|
| **docs/user-guide/Cursor-가이드.md** | `~/.local/bin/c4` → `cq`, `"command": "c4"` → `"cq"`, `c4 cursor` → `cq cursor` 등 |
| **docs/user-guide/플랫폼-지원.md** | `c4-core/bin/c4 mcp` → `c4-core/bin/cq mcp` |
| **docs/user-guide/문제-해결.md** | `/path/to/c4/c4-core/bin/c4` → `cq`/경로, `c4 status` → `cq status` |
| **docs/user-guide/워커-가이드.md** | `go build -o ~/.local/bin/c4` → `~/.local/bin/cq` (2곳) |
| **docs/user-guide/OpenCode-가이드.md** | command `c4` → `cq` |
| **docs/getting-started/설치-가이드.md** | `bin/c4`, `~/.local/bin/c4`, `c4-core/bin/c4` 전부 → `cq` |
| **docs/getting-started/팀-온보딩.md** | `c4-core/bin/c4`, `c4 status` → `cq` |
| **docs/deployment-topology.md** | `~/.local/bin/c4`, `c4-core/bin/c4`, 빌드 예시 `-o ~/.local/bin/c4` → `cq` |
| **docs/ops/배포-체크리스트.md** | `~/.local/bin/c4`, `ExecStart=.../bin/c4` → `cq` |
| **docs/developer-guide/아키텍처.md** | `c4-core/bin/c4` → `c4-core/bin/cq` |
| **docs/ROADMAP.md** | `c4-core/bin/c4` → `c4-core/bin/cq` |
| **docs/reviews/usability-improvement-backlog-2026-02-16.md** | 설치 가이드 문구 `c4-core/bin/c4`, `~/.local/bin/c4` → `cq` |
| **llms.txt** | `go build -o ~/.local/bin/c4` → `~/.local/bin/cq` |

---

## 3. 문서 — CLI 명령어 문구 (c4 status → cq status)

| 파일 | 수정 |
|------|------|
| **c4-core/cmd/c4/main.go** | 주석 `c4 status` → `cq status` |
| **c4-core/cmd/c4/run.go** | 에러 메시지 `'c4 status'` → `'cq status'` |
| **.gemini/playbook.md**, **.gemini/tools.md**, **.gemini/GEMINI.md** | `c4 status` → `cq status` |
| **.codex/README.md**, **.codex/agents/c4-status.md** | `c4 status` → `cq status` |
| **docs/developer-experience.md** | `c4 status` → `cq status` |
| **.claude/hooks/permission-rules.md** | `~/.local/bin/c4` → `~/.local/bin/cq` |

---

## 4. 하드코딩된 로컬 경로 (/Users/changmin/git/c4 → cq)

다음은 **개인 환경 경로**가 들어가 있어, 팀 공용으로 쓸 때는 상대 경로나 환경 변수로 바꾸는 편이 좋습니다.

| 파일 | 현재 | 비고 |
|------|------|------|
| **.claude/settings.json** | `"/Users/changmin/git/c4/**"` | `git/cq` 또는 변수화 |
| **.claude/settings.local.json** | `"/Users/changmin/git/c4/..."`, `bin/c4` | `cq` 경로·바이너리로 수정 |
| **.claude/hooks.json** | `cd /Users/changmin/git/c4/c4-core` | `git/cq` |
| **.gemini/settings.json** | `"/Users/changmin/git/c4/**"` | `git/cq` |
| **c1/src-tauri/src/scanner.rs** | `Path::new("/Users/changmin/git/c4")` | 테스트/예시용이면 상대 경로 또는 env |
| **c4-core/internal/knowledge/sync_test.go** | `"/Users/changmin/git/c4/main.go"` | 테스트용 경로만 수정 |

---

## 5. Go 코드 — 주석만 (동작 변경 없음)

| 파일 | 수정 |
|------|------|
| **c4-core/cmd/c4/init.go** | 주석 "c4-core/bin/c4" → "c4-core/bin/cq" (경로 추론 로직은 그대로 두어도 됨: `.../c4-core/bin/<binary>` 구조) |

---

## 6. 선택 사항 (의도에 따라 결정)

- **MCP 서버 키**: `"cq"`로 통일 완료 (`.mcp.json`, `.cursor/mcp.json`, install.sh, Codex `[mcp_servers.cq]`).

- **install.sh 원격 URL**  
  - `https://git.pilab.co.kr/pi/c4/raw/main/install.sh` → 실제 리포가 cq로 이전했으면 `pi/cq/...`로 변경.

- **.c4-install-path, C4_PROJECT_ROOT**  
  - 변수명은 그대로 두고, **값**만 새 리포 경로(`.../cq`)로 두면 됩니다.

---

## 요약

- **즉시 손봐야 할 것**: 루트·c4-core·infra의 `.mcp.json` 경로/command, `c4-finish` 스킬·`setup-mcp.sh`의 바이너리 이름, (리포 이전 시) `install.sh`의 리포 URL·기본 디렉터리.
- **문서 일관성**: 모든 가이드/스킬/주석에서 CLI·바이너리를 `cq`로 통일.
- **개인 경로**: `git/c4` → `git/cq` 또는 환경 변수/상대 경로로 정리.

이 파일은 점검용이므로, 수정 반영 후 필요 없으면 삭제하거나 "완료" 표시만 해두면 됩니다.
