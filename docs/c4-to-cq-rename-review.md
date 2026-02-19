# c4 → cq 이름 변경 전체 리뷰 (2026-02-19)

프로젝트 이름 변경에 따른 잔여 영향 점검 및 수정 내역.

---

## 1. 리뷰 범위

- **프로젝트/CLI 이름**: c4 → **cq** (바이너리 `cq`, 기본 경로 `$HOME/cq`, 리포 `pi/cq`)
- **MCP 서버 키**: `mcpServers.cq` (이미 반영됨)
- **유지**: `c4-core` 디렉터리, `cmd/c4` 패키지, **c4_*** MCP 도구명, C4 엔진/C 시리즈 명칭

---

## 2. 이번 리뷰에서 수정한 항목

### 2.1 로컬 설정·스크립트

| 파일 | 수정 내용 |
|------|-----------|
| **.claude/settings.local.json** | `bin/c4` → `bin/cq`, `~/.local/bin/c4` → `~/.local/bin/cq`, `git/c4` → `git/cq` (Bash 퍼미션/테스트 스니펫 전반) |
| **c4-core/cmd/c4/init.go** | 주석 `c4 claude, c4 codex, c4 cursor` → `cq claude, cq codex, cq cursor` |
| **c1/src-tauri/tauri.conf.json** | `identifier` `com.c4.c1` → `com.cq.c1` (C1 빌드 경로/캐시와 cq 리포 일치) |

### 2.2 문서 — 리포/설치 URL 및 경로

| 파일 | 수정 내용 |
|------|-----------|
| **docs/getting-started/설치-가이드.md** | `pi/c4` → `pi/cq`, `cd c4` → `cd cq`, `C4_INSTALL_DIR=/opt/c4` → `/opt/cq` |
| **docs/getting-started/팀-온보딩.md** | `git clone ... pi/c4.git` → `pi/cq.git` |
| **docs/user-guide/OpenCode-가이드.md** | `pi/c4/raw/main/install-remote.sh` → `pi/cq/...` |
| **docs/user-guide/문제-해결.md** | 이슈 링크 `pi/c4/-/issues` → `pi/cq/-/issues` |
| **docs/ops/배포-체크리스트.md** | `pi/c4`, `/opt/c4` → `pi/cq`, `/opt/cq` |
| **docs/deployment-topology.md** | install.sh URL `pi/c4` → `pi/cq` |

### 2.3 문서 — CLI 문구

| 파일 | 수정 내용 |
|------|-----------|
| **docs/ROADMAP.md** | `c4 run` → `cq run` |
| **.gemini/tools.md** | `c4 add-task` → `cq add-task` |
| **.gemini/GEMINI.md** | `c4 add-task` → `cq add-task` |
| **.codex/agents/c4-run.md** | `c4 run` → `cq run` |
| **.codex/hosted-worker/README.md** | `c4 run` → `cq run` |

---

## 3. 의도적 유지 (변경 안 함)

| 대상 | 이유 |
|------|------|
| **docs/c4-to-cq-rename-issues.md** | 점검 목록 문서. "before/after" 예시로 이전 값(c4) 표기 유지. |
| **c4-core/CHANGELOG.md** | 과거 배포 안내(`releases.c4.dev`, `c4-core` 바이너리명). 히스토리 보존. |
| **c4_* 도구명, c4-core, cmd/c4** | C4 엔진/패키지 명칭. 프로젝트 이름(cq)과 구분. |
| **OpenCode uv run c4.mcp_server** | Python 모듈명 `c4` (pyproject.toml). 런타임 경로만 cq로 통일됨. |

---

## 4. 요약

- **이번 라운드**: 로컬 설정(`settings.local.json`), init 주석, **C1 Tauri identifier com.c4.c1 → com.cq.c1**, **문서 내 pi/c4 → pi/cq**, **/opt/c4 → /opt/cq**, **CLI 예시 문구(c4 run / c4 add-task → cq)** 반영.
- **영향 없음**: MCP 도구 접두사 `c4_*`, 디렉터리 `c4-core`, 패키지 `cmd/c4`, C 시리즈(C0~C9) 명칭은 그대로 두는 것이 맞음.
- **추가 점검**: 실제 리포가 아직 `pi/c4`라면, clone/issue URL은 현재 리포에 맞게 유지할 수 있음. 리포를 `pi/cq`로 이전했으면 위 수정이 맞음.

이 문서는 2026-02-19 전체 리뷰 결과입니다.
