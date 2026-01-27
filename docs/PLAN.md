# C4 Project Plan

## 현재 상태: v0.6.6 (UX & Platform Refinement)

### 완료된 Phase

| Phase | 제목 | 상태 |
|-------|------|------|
| Phase 1 | Core Foundation | ✅ 완료 |
| Phase 2 | Multi-Worker Support | ✅ 완료 |
| Phase 3 | Auto Supervisor | ✅ 완료 |
| Phase 4 | Agent Routing | ✅ 완료 |
| Phase 5 | Enhanced Discovery & Design | ✅ 완료 |
| Phase 5.5 | Skill System Enhancement | ✅ 완료 |
| Phase 6 | Team Collaboration | ✅ 완료 |
| Phase 6.5 | MCP Advanced Tools | ✅ 완료 |
| Phase 6.6 | UX & Platform Refinement | ✅ 완료 |
| Phase 6.7 | Reliability & Observability | 🚧 진행 중 |

### Phase 6.7: Reliability & Observability 🚧

**가시성 및 복구 자동화**:
- T-671: OpenTelemetry 통합 및 메트릭 대시보드 기초 구축 ✅ 완료
- T-675: Prompt Caching 최적화 (비용 절감을 위한 고정 시퀀스 도입) ✅ 완료
- T-676: Context Slimming 엔진 (지능적 컨텍스트 압축) ✅ 완료
- T-677: 비용 및 사용량 리포팅 (토큰 트래킹 대시보드) ✅ 완료
- T-674: 워커 하트비트 이상 탐지 및 자동 복구 고도화

### Phase 6.6: UX & Platform Refinement ✅

**플랫폼 최적화**:
- T-661: Gemini CLI 전용 가이드 (`GEMINI.md`) 생성
- T-662: Gemini 슬래시 커맨드 경로 수정 (`.gemini/commands/`)
- T-663: 전역 MCP 설정 가이드 업데이트 (`~/.gemini/settings.json`)
- T-664: 프로젝트 구조 클린업 (Debug 스크립트 → `tests/debug/`)
- T-665: README.md 및 공식 문서 현행화

### Phase 6.5: MCP Advanced Tools ✅

**Supabase 기반 팀 협업**:
- T-601: Supabase 스키마 (`infra/supabase/migrations/`)
- T-602: SupabaseStateStore (`c4/store/supabase.py`)
- T-603: SupabaseLockStore (`c4/store/supabase.py`)
- T-604: 팀 관리 서비스 (`c4/services/teams.py`)
- T-605: CloudSupervisor (`c4/supervisor/cloud_supervisor.py`)
- T-606: TaskDispatcher (`c4/daemon/task_dispatcher.py`)
- T-607: GitHub 권한 통합 (`c4/integrations/github.py`)

**Branding Middleware**:
- T-614: BrandingMiddleware (`c4/api/middleware/branding.py`)

### Phase 6.5: MCP Advanced Tools ✅

**코드 분석 엔진**:
- T-7001: Code Analysis Engine (`c4/services/code_analysis/`)
  - PythonParser, TypeScriptParser
  - 심볼 테이블, 의존성 그래프

**Semantic Search Engine** (TF-IDF 기반):
- T-7009: SemanticSearcher (`c4/docs/semantic_search.py`)
  - 자연어 쿼리로 코드 검색
  - 프로그래밍 동의어 확장

**Call Graph Analyzer**:
- T-7010: CallGraphAnalyzer (`c4/docs/call_graph.py`)
  - 호출자/피호출자 분석
  - Mermaid 다이어그램 생성

**MCP 도구** (12개 신규):
- T-7005: Code Analysis MCP (`c4/mcp/code_tools.py`)
- T-7006: Documentation MCP (`c4/mcp/docs_server.py`)
- T-7007: Gap Analyzer MCP (`c4/mcp/gap_analyzer.py`)
- T-7008: Public Docs API (`c4/api/routes/docs.py`)

**명세-구현 매핑**:
- T-7002: Spec-Implementation Mapper
- T-7003: Gap Analyzer
- T-7004: Test Generator

**Long-Running Worker Detection**:
- T-7011: Heartbeat 기반 이상 탐지 (`c4/daemon/workers.py`)

---

## 프로젝트 구조

```text
c4/
├── c4/                    # 메인 패키지
│   ├── mcp_server.py      # MCP 서버 (C4Daemon)
│   ├── state_machine.py   # 상태 머신
│   ├── models/            # Pydantic 스키마
│   ├── daemon/            # 매니저 클래스
│   ├── store/             # State/Lock Store
│   │   ├── sqlite.py      # SQLite 구현 (기본)
│   │   └── supabase.py    # Supabase 구현 (팀)
│   ├── services/          # 비즈니스 로직
│   │   ├── teams.py       # 팀 관리
│   │   ├── branding.py    # 브랜딩
│   │   └── code_analysis/ # 코드 분석
│   ├── supervisor/        # Supervisor 컴포넌트
│   │   ├── supervisor_loop.py
│   │   ├── cloud_supervisor.py
│   │   └── agent_graph/   # 에이전트 라우팅
│   ├── integrations/      # 외부 연동
│   │   ├── github.py
│   │   └── gitlab.py
│   ├── api/               # REST API
│   │   ├── server.py
│   │   ├── routes/
│   │   └── middleware/
│   │       └── branding.py
│   └── mcp/               # MCP 도구
│       ├── docs_server.py
│       └── gap_analyzer.py
├── infra/
│   └── supabase/          # Supabase 설정
│       └── migrations/    # DB 마이그레이션
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
| Unit | 2700+ | ✅ |
| Integration | 300+ | ✅ |
| E2E | 30+ | ✅ |
| **Total** | **3030+** | ✅ |

---

## 다음 단계

[ROADMAP.md](./ROADMAP.md) 참조

1. ~~v0.1.0 릴리즈~~ ✅
2. ~~팀 협업 지원 (Supabase)~~ ✅
3. ~~MCP Advanced Tools~~ ✅
4. C4 Cloud (v0.7.0) - Phase 7
