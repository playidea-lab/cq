---
name: onboarding
description: 새 프로젝트에 piki 표준을 적용하는 온보딩 가이드. cq init 실행 후 프로젝트 구조를 검증한다.
triggers:
  - "온보딩"
  - "프로젝트 설정"
  - "새 프로젝트"
  - "onboarding"
  - "project setup"
---

# Project Onboarding

새 프로젝트에 PI Lab 표준을 적용하는 온보딩 워크플로우입니다.

## Step 1: 프로젝트 초기화

```bash
# 팀과 언어를 지정하여 초기화
cq init --team <팀> --lang <언어>

# 예시
cq init --team backend --lang go
cq init --team frontend --lang ts
cq init --team research --lang python
```

이 명령은 piki에서 다음을 복사합니다:
- `rules/common/*` → `.claude/rules/` (항상)
- `rules/<lang>/*` → `.claude/rules/` (선택한 언어)
- `teams/<team>/rules/*` → `.claude/rules/` (선택한 팀)
- `agents.md` → `CLAUDE.md`에 병합
- `settings/default.json` → `.claude/settings.json` (없을 때만)
- `config/default.yaml` → `.c4/config.yaml` (CQ 프로젝트인 경우)

## Step 2: 프로젝트 구조 검증

초기화 후 확인 사항:

### 필수 파일
- [ ] `.gitignore` — 시크릿, 빌드 산출물, IDE 파일 제외
- [ ] `CLAUDE.md` — 에이전트 행동 규칙 (agents.md 기반)
- [ ] `.claude/rules/` — 보안, git, 리뷰, 테스트 규칙

### 언어별 필수
| 언어 | 필수 파일 |
|------|----------|
| Go | `go.mod`, `cmd/`, `internal/` |
| Python | `pyproject.toml`, `src/`, `tests/` |
| TypeScript | `package.json`, `tsconfig.json`, `src/` |

### 보안 필수
- [ ] `.env` 가 `.gitignore`에 포함
- [ ] 시크릿 파일 패턴이 `.gitignore`에 포함
- [ ] pre-commit hook 설정 (gitleaks 등)

## Step 3: 프로젝트 고유 규칙

piki 표준 위에 프로젝트 고유 규칙을 추가할 수 있습니다:

```bash
# 프로젝트 고유 규칙 (piki sync 대상 아님)
.claude/rules/local-override.md
```

이 파일에 프로젝트 특유의 컨벤션, 도메인 용어, 아키텍처 결정 등을 기록합니다.

## Step 4: 검증 실행

```bash
# 구조 검증
cq doctor

# 출력 예시:
# ✅ CLAUDE.md 존재
# ✅ rules/security.md 최신
# ⚠️ rules/testing.md 버전 불일치 (piki v1.2 → 로컬 v1.1)
# ❌ .gitignore에 .env 패턴 없음
```

## Step 5: 팀 온보딩 체크리스트

새 팀원이 프로젝트에 합류할 때:

1. 리포 클론 + 의존성 설치
2. `cq doctor` 실행 → 경고 해결
3. 로컬 빌드 + 테스트 통과 확인
4. `.claude/rules/` 읽기 (15분)
5. `soul.md` 읽기 (5분)
6. 첫 MR: 작은 변경으로 워크플로우 체험
