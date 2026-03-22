# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## Nonstop 원칙

스킬(`/c4-*`, `/pi` 등) 또는 Worker 실행 중에는 **사용자에게 확인을 구하지 않고 끝까지 진행**한다.

- "이 접근 방식으로 진행할까요?" → 질문하지 않는다. 바로 진행.
- "다음 단계로 갈까요?" → 묻지 않는다. 다음 단계를 실행.
- 파일 편집, git 명령, 빌드, 테스트 → 자동 진행. 도구 권한은 훅이 관리.
- **멈춰야 하는 유일한 경우**: 빌드/테스트 실패, 또는 스킬이 명시적으로 사용자 입력을 요구하는 단계.

> 안전은 `c4-permission-reviewer` 훅이 보장한다. 에이전트는 판단하지 않고 실행한다.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
