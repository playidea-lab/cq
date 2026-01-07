# C4 Project Plan

## 현재 상태: v0.1.0 릴리즈 준비

### 완료된 작업

#### Phase 5: 리팩토링 ✅
- T-R01: 패키지 리네임 c4d → c4
- T-R02: daemon/ 서브패키지 추출 (LockManager, WorkerManager, EventBus)
- T-R03: models/ 분리 (8개 파일로 분할)
- T-R04: 테스트 재편성 (unit/, integration/, e2e/)

#### 버그 수정 ✅
- 중복 체크포인트 트리거 방지 (`passed_checkpoints` 추가)
- `c4_submit`에서 체크포인트 자동 진입

### 현재 작업

#### 문서 현행화 🔄
- README.md 업데이트 ✅
- ROADMAP.md 작성 ✅
- 기존 문서 현행화 🔄

---

## 프로젝트 구조

```text
c4/
├── c4/                    # 메인 패키지
│   ├── mcp_server.py      # MCP 서버 (C4Daemon)
│   ├── state_machine.py   # 상태 머신
│   ├── models/            # Pydantic 스키마
│   │   ├── enums.py
│   │   ├── task.py
│   │   ├── worker.py
│   │   ├── checkpoint.py
│   │   ├── event.py
│   │   ├── state.py
│   │   ├── config.py
│   │   └── responses.py
│   ├── daemon/            # 매니저 클래스
│   │   ├── locks.py       # LockManager
│   │   ├── workers.py     # WorkerManager
│   │   └── events.py      # EventBus
│   ├── bundle.py          # 체크포인트 번들
│   └── supervisor.py      # 슈퍼바이저
├── tests/
│   ├── unit/              # 단위 테스트
│   ├── integration/       # 통합 테스트
│   └── e2e/               # E2E 테스트
└── .claude/commands/      # 슬래시 명령어
```

---

## 테스트 현황

| 카테고리 | 테스트 수 | 상태 |
|----------|----------|------|
| Unit | 80+ | ✅ |
| Integration | 30+ | ✅ |
| E2E | 20+ | ✅ |
| **Total** | **128** | ✅ |

---

## 다음 단계

[ROADMAP.md](./ROADMAP.md) 참조

1. v0.1.0 릴리즈
2. State Store 추상화 (v0.2.0)
3. 팀 협업 지원 (Supabase/Redis)
4. C4 Cloud (v1.0.0)
