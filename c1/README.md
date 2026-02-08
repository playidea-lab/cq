# C1 (See) — Multi-LLM Project Explorer

Tauri 2.x 데스크톱 앱. Claude Code, Codex CLI, Cursor, Gemini CLI 등 다양한 LLM 도구의 세션을 통합 탐색합니다.

## Features

### 4개 뷰

| 뷰 | 기능 | 데이터 소스 |
|-----|------|-------------|
| **Sessions** | 세션 목록 + 메시지 뷰어 + Analytics | 프로바이더별 JSONL/SQLite |
| **Dashboard** | 태스크 관리 + Timeline + Validation | `.c4/c4.db` |
| **Config** | 설정 파일 탐색 (CLAUDE.md, personas 등) | `~/.claude/`, `.c4/` |
| **Team** | 팀 프로젝트 현황 (Cloud) | Supabase |

### 4개 프로바이더

| 프로바이더 | 소스 | 형식 |
|-----------|------|------|
| Claude Code | `~/.claude/projects/` | JSONL |
| Codex CLI | `~/.codex/` | JSONL |
| Cursor | `~/.cursor/` | SQLite (READONLY) |
| Gemini CLI | `~/.gemini/` | 스텁 |

### v0.2.0 (Performance + UX)

- **가상 스크롤**: `@tanstack/react-virtual` — SessionList, MessageViewer, TaskList
- **LRU 캐시**: Rust `lru` 크레이트, 32 entries, 30s TTL, watcher 무효화
- **Cloud retry**: Exponential backoff (3회, 1s/2s/4s)
- **FilterBar**: 정렬(date/size/name) + 기간 필터(today/week/month/all)
- **Skeleton**: CSS shimmer 로딩 애니메이션
- **Toast**: 자동 해제 알림 (3s)
- **ErrorState**: Retry 버튼 포함 통일된 에러 표시
- **테마 토글**: 다크/라이트 모드 (localStorage 저장)
- **접근성**: WCAG aria-*, role, prefers-reduced-motion

## Tech Stack

- **Frontend**: React 18 + TypeScript + Vite
- **Backend**: Rust (Tauri 2.x)
- **스타일**: CSS BEM + design-tokens.css
- **테스트**: Vitest 80개 + Cargo 31개

## Quick Start

```bash
# 의존성 설치
pnpm install

# 개발 서버
pnpm tauri dev

# 프로덕션 빌드
pnpm tauri build

# 테스트
pnpm test                    # Vitest (frontend)
cd src-tauri && cargo test   # Cargo (backend)
```

## 환경 변수

Team/Cloud 기능 사용 시 프로젝트 루트 `.env`에 설정:

```
SUPABASE_URL=https://xxxx.supabase.co
SUPABASE_KEY=eyJ...
```

앱 시작 시 `dotenvy`가 `.env`, `../.env`, `~/.c4/.env` 순으로 자동 로드합니다.

## 디렉토리 구조

```
c1/
├── src/
│   ├── components/
│   │   ├── auth/          # LoginView
│   │   ├── sessions/      # SessionsView, SessionList, MessageViewer, FilterBar
│   │   ├── dashboard/     # DashboardView, TaskList, TaskTimeline, ValidationPanel
│   │   ├── config/        # ConfigView
│   │   ├── team/          # TeamView
│   │   └── shared/        # StatusBadge, ProgressBar, Skeleton, Toast, ErrorState
│   ├── hooks/             # useSessions, useDashboard, useConfig, useEditors
│   ├── contexts/          # AuthContext, ToastContext
│   ├── styles/            # CSS (BEM + design-tokens)
│   └── App.tsx
├── src-tauri/
│   └── src/
│       ├── lib.rs         # Tauri 앱 설정
│       ├── commands.rs    # IPC 커맨드 (31개)
│       ├── providers/     # 4개 세션 프로바이더
│       ├── analytics.rs   # 세션 통계
│       ├── cloud.rs       # Supabase REST
│       ├── auth.rs        # GitHub OAuth
│       ├── scanner.rs     # JSONL 파싱
│       └── watcher.rs     # 파일 변경 감지
└── package.json
```
