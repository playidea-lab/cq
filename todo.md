# C4D Self Review Tasks

## Phase 0: 다중 플랫폼 지원 (완료)

### T-000: 플랫폼 추상화
- **Scope**: c4/platforms/, c4/cli.py
- **DoD**: 
  - [x] c4/platforms/ 모듈 생성 (SPEC.md, __init__.py)
  - [x] CLI --platform 옵션 추가
  - [x] c4 config 명령 추가 (글로벌/프로젝트 설정)
  - [x] 플랫폼 커맨드 검증 + 템플릿 생성
  - [x] tests/unit/test_platforms.py 테스트 작성
- **Validations**: lint, unit
- **Refs**: [c4/platforms/SPEC.md](c4/platforms/SPEC.md)

### T-006: Cursor 커맨드 준비
- **Scope**: .cursor/commands/, .claude/commands/
- **DoD**:
  - [x] .cursor/commands/ 디렉토리 생성
  - [x] 단순 커맨드 6개 복제 (status, init, stop, clear, validate, add-task)
  - [x] 복잡한 커맨드 4개 복제 (plan, run, checkpoint, submit)
  - [x] Cursor 커맨드 검증 (c4 platforms --validate cursor)
  - [x] tests/unit/test_cursor_commands.py 테스트 작성
- **Validations**: unit
- **Refs**: [c4/platforms/SPEC.md](c4/platforms/SPEC.md), [.claude/commands/](.claude/commands/), [.cursor/commands/](.cursor/commands/), [tests/unit/test_cursor_commands.py](tests/unit/test_cursor_commands.py)

### T-007: Cursor 커맨드 테스트 실행
- **Scope**: .cursor/commands/, tests/unit/test_cursor_commands.py
- **DoD**:
  - [x] c4 platforms --validate cursor 실행
  - [x] uv run pytest tests/unit/test_cursor_commands.py -v 실행
- **Validations**: unit
- **Refs**: [.cursor/commands/](.cursor/commands/), [tests/unit/test_cursor_commands.py](tests/unit/test_cursor_commands.py)

---

## Phase 1: 코드 리뷰

### T-001: MCP Server 리뷰
- **Scope**: c4d/mcp_server
- **DoD**: MCP Server 코드 리뷰 완료, 개선점 문서화
- **Validations**: lint, unit

### T-002: State Machine 리뷰
- **Scope**: c4d/state_machine
- **DoD**: State Machine 코드 리뷰 완료, 상태 전이 검증
- **Validations**: lint, unit

### T-003: Supervisor 리뷰
- **Scope**: c4d/supervisor
- **DoD**: Supervisor 코드 리뷰 완료, JSON 파싱 로직 검증
- **Validations**: lint, unit

---

## Phase 2: 문서 리뷰

### T-004: 문서 리뷰
- **Scope**: docs
- **DoD**: 문서 일관성 검토, 누락 항목 확인
- **Validations**: lint

### T-005: README 업데이트
- **Scope**: root
- **DoD**: README.md에 설치 및 사용법 추가
- **Validations**: lint, unit
