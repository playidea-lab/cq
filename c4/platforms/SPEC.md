# C4 Platform Command Interface Specification

이 문서는 각 플랫폼에서 구현해야 할 C4 커맨드의 인터페이스를 정의합니다.

## 개요

C4는 MCP(Model Context Protocol) 서버를 통해 플랫폼 독립적인 기능을 제공합니다.
각 플랫폼은 MCP 도구를 호출하는 슬래시 커맨드(또는 동등한 기능)를 구현해야 합니다.

## 지원 플랫폼

| 플랫폼 | 커맨드 위치 | 형식 |
|--------|------------|------|
| Claude Code | `.claude/commands/` | Markdown |
| Cursor | `.cursor/commands/` | Markdown |
| Codex CLI | `.codex/agents/` | AGENTS.md 형식 |
| Gemini CLI | `.gemini/commands/` | Markdown |

## 필수 커맨드

### c4-status

**목적**: 프로젝트 상태 확인

**MCP 도구**: `c4_status()`

**출력 정보**:
- Project ID
- Current state (INIT/PLAN/EXECUTE/CHECKPOINT/COMPLETE/HALTED)
- Task queue (pending, in_progress, done counts)
- Active workers
- Metrics

**복잡도**: 낮음 (단일 MCP 호출)

---

### c4-init

**목적**: 프로젝트에 C4 초기화

**MCP 도구**: 없음 (CLI 명령 사용)

**동작**:
1. `.c4/` 디렉토리 생성
2. `.mcp.json` 생성
3. `.claude/settings.json` (또는 플랫폼별 설정) 생성
4. Hooks 설치 (Claude Code만 해당)

**복잡도**: 낮음

---

### c4-plan (복잡)

**목적**: Discovery -> Design -> Plan 워크플로우 실행

**MCP 도구**:
- `c4_status()` - 상태 확인
- `c4_save_spec()` - EARS 요구사항 저장
- `c4_discovery_complete()` - Discovery 완료
- `c4_save_design()` - 설계 저장
- `c4_design_complete()` - Design 완료
- `c4_add_todo()` - 태스크 추가

**워크플로우**:
1. Phase 0: 상태 확인
2. Phase 1: 기획 문서 스캔 (docs/*.md)
3. Phase 2: 문서 해석 및 핵심 정보 추출
4. Phase 2.5: Discovery (도메인 감지, EARS 요구사항 수집)
5. Phase 2.6: Design (아키텍처 옵션, 컴포넌트 설계)
6. Phase 3: 구조화된 인터뷰 (개발 환경)
7. Phase 4: 태스크 생성
8. Phase 5: 계획 확정

**복잡도**: 매우 높음 (인터뷰, 조건분기, 여러 MCP 호출)

**참조**: `.claude/commands/c4-plan.md` (1000+ 줄)

---

### c4-run (복잡)

**목적**: Worker Loop 실행

**MCP 도구**:
- `c4_status()` - 상태 확인
- `c4_start()` - EXECUTE 상태 전환
- `c4_get_task(worker_id)` - 태스크 할당
- `c4_run_validation(names)` - 검증 실행
- `c4_submit(task_id, commit_sha, validation_results)` - 태스크 제출

**워크플로우**:
```
1. Worker ID 생성 (UUID)
2. 상태 확인
3. PLAN/HALTED -> c4_start() -> EXECUTE
4. Worker Loop:
   a. c4_get_task(worker_id)
   b. 태스크 없으면 종료
   c. DoD에 따라 구현
   d. c4_run_validation()
   e. git commit
   f. c4_submit()
   g. next_action에 따라 분기
5. 종료 또는 checkpoint 대기
```

**플랫폼별 주의사항**:
- Claude Code: Accept Edits 모드 필요
- Cursor: Composer Agent 모드 권장
- Codex: triggers 설정 필요

**복잡도**: 높음 (루프, 조건분기, 여러 MCP 호출)

**참조**: `.claude/commands/c4-run.md` (240+ 줄)

---

### c4-checkpoint (복잡)

**목적**: 체크포인트 리뷰 및 결정

**MCP 도구**:
- `c4_status()` - 상태 확인
- `c4_checkpoint(checkpoint_id, decision, notes, required_changes)` - 결정 기록

**결정 옵션**:
- APPROVE: 다음 단계 진행
- REQUEST_CHANGES: 수정 태스크 생성
- REPLAN: PLAN 상태로 복귀
- REDESIGN: DESIGN 상태로 복귀

**복잡도**: 중간 (사용자 인터랙션, 조건분기)

**참조**: `.claude/commands/c4-checkpoint.md`

---

### c4-submit (복잡)

**목적**: 현재 작업 제출

**MCP 도구**:
- `c4_submit(task_id, commit_sha, validation_results, worker_id)`

**워크플로우**:
1. 현재 태스크 확인
2. 검증 실행 (아직 안했으면)
3. git commit
4. c4_submit() 호출
5. next_action 처리

**복잡도**: 중간

**참조**: `.claude/commands/c4-submit.md`

---

### c4-stop

**목적**: 실행 중단

**MCP 도구**: `c4_stop()` 또는 상태 전이

**복잡도**: 낮음

---

### c4-validate

**목적**: 검증 명령 실행

**MCP 도구**: `c4_run_validation(names)`

**기본 검증**: `["lint", "unit"]`

**복잡도**: 낮음

---

### c4-add-task

**목적**: 태스크 추가

**MCP 도구**: `c4_add_todo(task_id, title, scope, dod, dependencies, domain, priority)`

**DoD 작성 원칙**:
- 검증 가능해야 함
- 구체적이어야 함
- 독립적이어야 함

**복잡도**: 낮음

---

### c4-clear

**목적**: C4 상태 초기화

**MCP 도구**: `c4_clear(confirm, keep_config)`

**복잡도**: 낮음

---

## 플랫폼별 구현 가이드

### Claude Code

기준 구현입니다. `.claude/commands/` 디렉토리의 파일들을 참조하세요.

### Cursor

Claude Code와 유사한 마크다운 슬래시 커맨드를 지원합니다.
`.cursor/commands/` 디렉토리에 동일한 형식으로 구현합니다.

**차이점**:
- MCP 도구 호출 문법이 다를 수 있음
- Composer Agent 모드에서 실행 권장

### Codex CLI

AGENTS.md frontmatter 형식을 사용합니다.

```markdown
---
name: c4-worker
description: C4 Worker Agent
triggers:
  - c4 run
  - start c4 worker
---

# Workflow

1. Call c4_status()
2. ...
```

### Gemini CLI

슬래시 커맨드 대신 자연어 요청과 MCP 도구를 사용합니다.
`.gemini/` 디렉토리에는 참조용 가이드 문서를 작성합니다.

---

## 검증

`c4 init --platforms <platform>` 명령으로 필수 커맨드 존재 여부를 검증합니다.
누락된 커맨드는 템플릿이 자동 생성됩니다.

```bash
$ c4 init --platforms cursor

[!] Cursor 커맨드 검증 중...
[+] c4-status.md - 존재
[+] c4-init.md - 존재
[!] c4-plan.md - 누락 -> 템플릿 생성
[!] c4-run.md - 누락 -> 템플릿 생성
...
```

복잡한 커맨드(c4-plan, c4-run 등)는 Claude Code 버전을 복사하여 템플릿으로 제공합니다.
**반드시 해당 플랫폼에 맞게 커스터마이즈하세요.**
