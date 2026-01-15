# C4D Self Review Tasks

## Hotfix

### HF-001: --path 옵션 버그 수정
- **Scope**: c4/cli.py
- **DoD**:
  - [x] `c4 --path /path status` 형태로 실행시 경로 인식 버그 수정
  - [x] `c4_main` 콜백에서 서브커맨드 호출 전 `C4_PROJECT_ROOT` 환경변수 설정
- **Validations**: 수동 테스트

---

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

### T-008: 미커밋 22개 항목 정리
- **Scope**: README.md, docs/, c4/supervisor/, tests/, pyproject.toml, uv.lock
- **DoD**:
  - [x] 변경 사항 22개 항목 확인 (diff 검토)
  - [x] 관련 파일 전부 커밋
  - [x] 원격 푸시 완료
- **Validations**: lint, unit
- **Refs**: [docs/](docs/), [README.md](README.md)

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

---

## Phase 3: Git 통합 강화

### T-301: Git 필수 설치
- **Scope**: install.sh
- **DoD**:
  - [ ] Git 설치 여부 체크
  - [ ] 없으면 OS별 자동 설치
  - [ ] 설치 실패 시 에러 메시지
- **Refs**: [install.sh](install.sh)

### T-302: c4 init Git 자동화
- **Scope**: c4/cli.py
- **DoD**:
  - [ ] `c4 init` 시 `git init` 자동 실행
  - [ ] `.gitignore` 생성
  - [ ] 초기 커밋 생성
- **Refs**: [c4/cli.py](c4/cli.py)

### T-303: 자동 커밋 시스템
- **Scope**: c4/daemon/workers.py, c4/hooks.py
- **DoD**:
  - [ ] 태스크 완료 시 자동 커밋
  - [ ] 체크포인트 통과 시 태그 생성
  - [ ] 수정 완료 시 커밋
- **Refs**: [c4/daemon/workers.py](c4/daemon/workers.py)

### T-304: 롤백 기능
- **Scope**: c4/cli.py
- **DoD**:
  - [ ] `c4 rollback <checkpoint>` 명령
  - [ ] 롤백 전 확인 프롬프트
- **Refs**: [c4/cli.py](c4/cli.py)

---

## Phase 4: 인증 시스템 (Supabase Auth)

### T-401: Supabase 프로젝트 설정
- **Scope**: infra/supabase/
- **DoD**:
  - [ ] Supabase 프로젝트 생성
  - [ ] Auth Provider 설정 (GitHub, Google)
  - [ ] 환경변수 관리
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-402: CLI 로그인 구현
- **Scope**: c4/cli.py, c4/auth/
- **DoD**:
  - [ ] `c4 login` 명령 (PKCE 플로우)
  - [ ] 세션 저장 (~/.c4/session.json)
  - [ ] `c4 logout` 명령
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-403: Supabase 클라이언트
- **Scope**: c4/auth/supabase_client.py
- **DoD**:
  - [ ] supabase-py 클라이언트 래퍼
  - [ ] 토큰 자동 갱신
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

---

## Phase 5: 채팅 UI

### T-501: Chat API 서버
- **Scope**: c4/api/
- **DoD**:
  - [ ] FastAPI 기반 Chat API
  - [ ] SSE 스트림 응답
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-502: 로컬 UI 서버
- **Scope**: c4/cli.py, c4/ui/
- **DoD**:
  - [ ] `c4 ui` 명령
  - [ ] 로컬 웹 서버 시작
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-503: 웹 프론트엔드
- **Scope**: web/
- **DoD**:
  - [ ] Next.js 프로젝트
  - [ ] 채팅 UI 컴포넌트
  - [ ] 프로젝트 목록/상세 페이지
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

---

## Phase 6: 팀 협업 (Supabase)

### T-601: Supabase 스키마 구축
- **Scope**: infra/supabase/migrations/
- **DoD**:
  - [ ] teams, team_members, projects 테이블
  - [ ] c4_state, c4_tasks, c4_workers, c4_events 테이블
  - [ ] RLS 정책 설정
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-602: SupabaseStateStore
- **Scope**: c4/store/supabase.py
- **DoD**:
  - [ ] `SupabaseStateStore` 클래스
  - [ ] StateStore 프로토콜 준수
  - [ ] Realtime 구독 지원
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md), [c4/store/protocol.py](c4/store/protocol.py)

### T-603: SupabaseLockStore
- **Scope**: c4/store/supabase.py
- **DoD**:
  - [ ] `SupabaseLockStore` 클래스
  - [ ] Row-level lock
  - [ ] TTL 자동 해제
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-604: 팀/프로젝트 관리
- **Scope**: c4/cli.py, c4/team/
- **DoD**:
  - [ ] `c4 team create/list/invite` 명령
  - [ ] `c4 init --team` 옵션
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-605: 중앙 Supervisor
- **Scope**: c4/supervisor/cloud_supervisor.py
- **DoD**:
  - [ ] 팀장 개인키로 리뷰 실행
  - [ ] 체크포인트 리뷰 처리
  - [ ] GitHub PR 리뷰 코멘트
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-606: 태스크 분배 로직 (Peer Review)
- **Scope**: c4/daemon/task_distributor.py
- **DoD**:
  - [ ] 우선순위 기반 태스크 할당
  - [ ] 수정 태스크는 original_worker 제외
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-607: GitHub 권한 관리
- **Scope**: c4/integrations/github.py
- **DoD**:
  - [ ] Organization 멤버십 확인
  - [ ] Collaborator 자동 초대
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

---

## Phase 7: 클라우드 실행

### T-701: 클라우드 워커 인프라
- **Scope**: infra/workers/
- **DoD**:
  - [ ] Fly.io Machines 설정
  - [ ] 워커 Docker 이미지
  - [ ] 동적 스케일링
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-702: 샌드박스 환경
- **Scope**: c4/sandbox/
- **DoD**:
  - [ ] gVisor 런타임
  - [ ] 네트워크 Egress 제한
  - [ ] 리소스 제한
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-703: 과금 시스템
- **Scope**: c4/billing/
- **DoD**:
  - [ ] Stripe 연동
  - [ ] BYOK/Managed 선택
  - [ ] 플랜별 제한
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-704: Managed API 프록시
- **Scope**: c4/api/llm_proxy.py
- **DoD**:
  - [ ] LLM API 프록시
  - [ ] 사용량 미터링
  - [ ] Rate limiting
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-705: 결과물 전달
- **Scope**: c4/api/delivery.py
- **DoD**:
  - [ ] ZIP/PR 생성
  - [ ] 다운로드 링크
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-706: 클라우드 모니터링
- **Scope**: infra/monitoring/
- **DoD**:
  - [ ] Sentry/Prometheus/Grafana
  - [ ] 알림 설정
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)

### T-707: 클라우드 테스트
- **Scope**: tests/e2e/
- **DoD**:
  - [ ] 워커/샌드박스/과금 테스트
- **Refs**: [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md)
