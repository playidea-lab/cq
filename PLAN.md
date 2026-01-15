# C4 Cloud 개발 계획

## 개요

C4를 다양한 사용자 그룹(개인 IDE, 개인 채팅 UI, 팀)에 맞게 확장하기 위한 개발 계획.

## 현재 Phase: Phase 3 - Git 통합 강화

### 목표
- Git을 C4 내부 인프라로 활용
- 사용자 몰라도 자동 형상 관리

---

## Tasks

### T-301: Git 필수 설치
- **Scope**: `install.sh`
- **Priority**: P0
- **Description**: 
  - Git 설치 여부 체크
  - 없으면 OS별 자동 설치 (macOS: xcode-select, Linux: apt/yum)
  - 설치 실패 시 명확한 에러 메시지
- **Acceptance Criteria**:
  - [ ] `install.sh`에서 Git 체크 로직 추가
  - [ ] macOS: `xcode-select --install` 실행
  - [ ] Linux Debian: `apt-get install git`
  - [ ] Linux RHEL: `yum install git`
  - [ ] 실패 시 에러 메시지와 함께 종료

### T-302: c4 init Git 자동화
- **Scope**: `c4/cli.py`
- **Priority**: P0
- **Dependencies**: T-301
- **Description**:
  - `c4 init` 시 `.git/` 없으면 `git init` 자동 실행
  - `.gitignore` 생성 (`.c4/locks/`, `.c4/workers/`, `*.log`)
  - 초기 커밋 생성: `[C4] Project initialized`
- **Acceptance Criteria**:
  - [ ] `c4 init`에서 `git init` 자동 실행
  - [ ] `.gitignore` 파일 생성/업데이트
  - [ ] 초기 커밋 생성
  - [ ] 이미 `.git/` 있으면 스킵

### T-303: 자동 커밋 시스템
- **Scope**: `c4/daemon/workers.py`, `c4/hooks.py`
- **Priority**: P0
- **Dependencies**: T-302
- **Description**:
  - 태스크 완료 시 자동 커밋: `[C4] task_XXX: {task_name}`
  - 체크포인트 통과 시 태그 생성: `c4/checkpoint/CP-XXX`
  - 수정 완료 시 커밋: `[C4] repair: {description}`
- **Acceptance Criteria**:
  - [ ] `c4/git.py` 모듈 생성 (Git 작업 헬퍼)
  - [ ] 태스크 완료 후 자동 커밋 훅
  - [ ] 체크포인트 태그 생성 로직
  - [ ] 수정 완료 시 커밋 메시지 포맷

### T-304: 롤백 기능
- **Scope**: `c4/cli.py`
- **Priority**: P1
- **Dependencies**: T-303
- **Description**:
  - `c4 rollback <checkpoint>` 명령 추가
  - `git reset --hard c4/checkpoint/CP-XXX` 실행
  - 롤백 전 확인 프롬프트
- **Acceptance Criteria**:
  - [ ] `c4 rollback` 명령 구현
  - [ ] 체크포인트 목록 표시
  - [ ] 확인 프롬프트 후 롤백 실행
  - [ ] 롤백 후 상태 메시지

### T-305: Git 통합 테스트
- **Scope**: `tests/integration/test_git_integration.py`
- **Priority**: P1
- **Dependencies**: T-303, T-304
- **Description**:
  - 자동 커밋 테스트
  - 체크포인트 태그 테스트
  - 롤백 테스트
- **Acceptance Criteria**:
  - [ ] `tests/integration/test_git_integration.py` 파일 생성
  - [ ] 자동 커밋 검증 테스트
  - [ ] 태그 생성 검증 테스트
  - [ ] 롤백 기능 테스트

---

## Checkpoints

### CP-001: Git 기본 통합
- **Tasks**: T-301, T-302
- **Validation**:
  - `c4 init` 실행 시 Git 초기화 확인
  - `.gitignore` 생성 확인
  - 초기 커밋 존재 확인

### CP-002: 자동 형상 관리
- **Tasks**: T-303, T-304, T-305
- **Validation**:
  - 태스크 완료 후 커밋 존재 확인
  - 체크포인트 태그 존재 확인
  - 롤백 후 상태 복원 확인
  - 모든 테스트 통과

---

## 향후 Phase (참고)

### Phase 4: 인증 시스템
- Keycloak SSO
- `c4 login` 명령
- JWT 토큰 관리

### Phase 5: 채팅 UI
- FastAPI Chat API
- `c4 ui` 로컬 서버
- Next.js 웹 프론트엔드

### Phase 6: 팀 협업
- Supabase StateStore
- 중앙 Supervisor
- GitHub 권한 관리

### Phase 7: 클라우드 실행
- 클라우드 워커 (Fly.io)
- 샌드박스 환경
- Stripe 과금

---

## References

- [docs/cloud/DEVELOPMENT_PLAN.md](docs/cloud/DEVELOPMENT_PLAN.md) - 전체 개발 계획
- [docs/cloud/ARCHITECTURE.md](docs/cloud/ARCHITECTURE.md) - 클라우드 아키텍처
- [c4/store/protocol.py](c4/store/protocol.py) - StateStore 프로토콜
