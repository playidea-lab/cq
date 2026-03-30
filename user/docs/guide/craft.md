# CQ Craft — Build and Install Your Own AI Tools

> Design your AI agent's behavior directly — from GPU experiment workflows to code review checklists.

---

## 왜 Craft인가

Claude Code는 skill, agent, rule 세 가지로 에이전트 행동을 커스터마이즈한다.
CQ Craft는 이 세 가지를 **설치하고, 만들고, 관리하는 통합 인터페이스**다.

```
내가 반복하는 패턴이 있다
→ 프리셋에 있나? → cq add
→ 없다? → /craft로 대화하며 만든다
→ 커뮤니티에 있나? → cq add owner/repo:name
```

---

## 4가지 도구 타입

| 타입 | 역할 | 저장 경로 | 사용법 |
|------|------|----------|--------|
| **Skill** | 워크플로우 (여러 단계) | `~/.claude/skills/{name}/SKILL.md` | `/{name}` 또는 트리거 키워드 |
| **Agent** | 전문가/페르소나 | `~/.claude/agents/{name}.md` | 자동 감지 (트리거 키워드) |
| **Rule** | 항상 적용되는 제약 | `~/.claude/rules/{name}.md` | 모든 대화에 자동 적용 |
| **CLAUDE.md** | 프로젝트 지침 | `./CLAUDE.md` | 프로젝트 열 때 자동 로딩 |

---

## 빠른 시작

### 1. TUI로 프리셋 브라우징

```bash
cq add
```

53개 내장 프리셋을 카테고리별로 탐색. 화살표로 이동, Enter로 설치.

### 2. 이름으로 직접 설치

```bash
cq add code-review        # 코드 리뷰 체크리스트 스킬
cq add go-pro             # Go 전문가 에이전트
cq add no-console-log     # console.log 금지 룰
cq add python-project     # Python 프로젝트 CLAUDE.md
```

### 3. GitHub에서 설치

```bash
# Anthropic 공식 스킬
cq add anthropics/skills:pdf
cq add anthropics/skills:pptx
cq add anthropics/skills:xlsx

# Superpowers 커뮤니티 스킬
cq add obra/superpowers:brainstorming
cq add obra/superpowers:test-driven-development

# 풀 URL도 가능
cq add https://github.com/anthropics/skills/tree/main/skills/webapp-testing
```

### 4. 대화형으로 새로 만들기

Claude Code에서 `/craft` 입력:

```
> 스킬 만들어줘 — 커밋 전에 항상 go vet + 테스트 실행

[판단] 이건 skill입니다 — 여러 단계 워크플로우이기 때문.

이름: pre-commit-check
트리거: "커밋 전 확인", "pre-commit"
동작: go vet → go test → 결과 보고

이렇게 만들까요?
```

---

## 관리

```bash
# 설치된 도구 목록
cq list --mine

# 원격 설치 스킬 업데이트
cq update pdf

# 삭제
cq remove brainstorming
cq remove --force pdf     # 확인 없이
```

---

## CQ 워크플로우에서 Craft의 위치

CQ의 전체 개발 루프:

```
/pi (아이디어) → /plan (설계) → /run (실행) → /finish (마무리)
```

Craft는 이 루프를 **커스터마이즈하는 메타 도구**다:

| 시점 | Craft로 할 수 있는 것 |
|------|---------------------|
| `/pi` 전 | `/craft`로 나만의 브레인스토밍 스킬 추가 |
| `/plan` 시 | `cq add api-design`으로 API 설계 체크리스트 설치 |
| `/run` 중 | `cq add go-pro`로 Go 전문가 에이전트 추가 |
| `/finish` 후 | `cq add release-notes`로 릴리즈 노트 자동화 |
| 코드 리뷰 시 | `cq add code-review`로 6축 리뷰 체크리스트 |
| 새 프로젝트 | `cq add python-project`로 CLAUDE.md 자동 생성 |

### 예시: 새 프로젝트 셋업

```bash
# 1. CLAUDE.md 생성
cq add go-project                    # Go 프로젝트 기본 지침

# 2. 팀 규칙 설치
cq add strict-types                  # any 타입 금지
cq add test-naming                   # 테스트 함수명 규칙
cq add error-handling                # 에러 무시 금지

# 3. 리뷰 워크플로우 설치
cq add code-review                   # 6축 코드 리뷰

# 4. 커뮤니티 스킬 추가
cq add anthropics/skills:pdf         # PDF 생성

# 5. 나만의 스킬 만들기
/craft                               # 대화형으로 커스텀 스킬 생성
```

---

## 내장 프리셋 카탈로그

### Skills (16개)

| 이름 | 설명 |
|------|------|
| code-review | 6축 코드 리뷰 체크리스트 |
| pr-template | PR 생성 + 자동 체크 |
| daily-standup | 데일리 스탠드업 정리 (git log 기반) |
| deploy-checklist | 배포 전 확인 체크리스트 |
| test-first | TDD 사이클 가이드 |
| hotfix-flow | 긴급 수정 워크플로우 |
| git-cleanup | 브랜치 정리 워크플로우 |
| migration-guide | DB 마이그레이션 안전 실행 |
| incident-runbook | 장애 대응 런북 |
| api-design | REST API 설계 체크리스트 |
| security-audit | 보안 점검 워크플로우 |
| onboarding | 새 팀원 온보딩 가이드 |
| release-notes | 릴리즈 노트 자동 생성 |
| refactor-plan | 리팩토링 계획 수립 |
| perf-check | 성능 점검 체크리스트 |
| doc-review | 문서 리뷰 체크리스트 |

### Agents (17개)

| 이름 | 설명 |
|------|------|
| rust-pro | Rust 전문가 |
| python-pro | Python 전문가 |
| go-pro | Go 전문가 |
| ts-pro | TypeScript 전문가 |
| kotlin-pro | Kotlin 전문가 |
| swift-pro | Swift 전문가 |
| reviewer | 시니어 코드 리뷰어 |
| architect | 시스템 아키텍트 |
| mentor | 주니어 멘토 |
| sql-expert | SQL/DB 전문가 |
| devops-pro | DevOps 전문가 |
| security-pro | 보안 전문가 |
| api-designer | API 설계 전문가 |
| tech-writer | 기술 문서 작성 전문가 |
| data-engineer | 데이터 엔지니어 |
| ux-reviewer | UX 리뷰어 |
| perf-expert | 성능 전문가 |

### Rules (12개)

| 이름 | 설명 |
|------|------|
| no-console-log | console.log/print 디버그 출력 금지 |
| korean-comments | 주석 한국어 필수 |
| strict-types | any 타입 금지 |
| no-magic-numbers | 매직 넘버 금지 |
| import-order | import 순서 규칙 |
| error-handling | 에러 무시 금지 |
| no-todo-comments | TODO 주석 금지 → 이슈 트래커 |
| no-hardcoded-urls | URL 하드코딩 금지 |
| max-function-length | 함수 50줄 이하 |
| test-naming | 테스트 함수명 규칙 |
| no-god-objects | God Object 금지 |
| log-level-policy | 로그 레벨 정책 |

### CLAUDE.md (8개)

| 이름 | 설명 |
|------|------|
| general | 범용 템플릿 |
| go-project | Go 프로젝트 |
| python-project | Python 프로젝트 |
| web-frontend | React/TypeScript 프론트엔드 |
| rust-project | Rust 프로젝트 |
| kotlin-android | Kotlin Android |
| ml-experiment | ML 실험 프로젝트 |
| monorepo | 모노레포 |

---

## 추천 GitHub 소스

| 소스 | 설명 | 설치 |
|------|------|------|
| [Anthropic 공식](https://github.com/anthropics/skills) | PDF, DOCX, PPTX, XLSX 등 문서 스킬 | `cq add anthropics/skills:<name>` |
| [Superpowers](https://github.com/obra/superpowers) | TDD, 브레인스토밍, 디버깅 방법론 | `cq add obra/superpowers:<name>` |
| [awesome-claude-skills](https://github.com/travisvn/awesome-claude-skills) | 커뮤니티 큐레이션 목록 | URL로 개별 설치 |
