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

## Channel Adapter (Dooray Messenger)

`c1/` 에는 Dooray Messenger를 Claude Code에 연결하는 MCP Channel 서버가 포함되어 있습니다.
Dooray 웹훅 → Claude 알림 → reply 도구로 응답하는 양방향 채팅 브릿지입니다.

### 설치

```bash
# Bun 필요 (https://bun.sh)
bun install   # c1/ 디렉토리에서 실행
```

### 환경 변수 설정

`.env` 파일 또는 셸 환경 변수로 설정합니다.

| 변수 | 설명 | 기본값 |
|------|------|--------|
| `DOORAY_BOT_TOKEN` | Dooray REST API Bearer 토큰 | (필수) |
| `DOORAY_TENANT_ID` | Dooray 테넌트 ID | (필수) |
| `DOORAY_API_URL` | Dooray API Base URL | `https://api.dooray.com` |
| `DOORAY_LISTEN_PORT` | 로컬 웹훅 수신 포트 | `9981` |

```bash
export DOORAY_BOT_TOKEN="dooray-api xxxxxxxx"
export DOORAY_TENANT_ID="my-tenant"
# 선택 사항
export DOORAY_API_URL="https://api.dooray.com"
export DOORAY_LISTEN_PORT="9981"
```

### Claude Code에 연결 (--channels 옵션)

`claude --channels` 플래그로 MCP Channel 서버를 등록한 뒤 실행합니다.

```bash
# 1. 웹훅 서버 + MCP Channel 서버를 stdio로 기동
bun run c1/index.ts &

# 2. Claude Code에 MCP 서버 등록 후 실행
claude --mcp-server "bun run c1/index.ts" --channels

# 또는 .mcp.json 으로 영구 등록
cat > .mcp.json <<'EOF'
{
  "mcpServers": {
    "dooray": {
      "command": "bun",
      "args": ["run", "c1/index.ts"],
      "env": {
        "DOORAY_BOT_TOKEN": "${DOORAY_BOT_TOKEN}",
        "DOORAY_TENANT_ID": "${DOORAY_TENANT_ID}"
      }
    }
  }
}
EOF
claude
```

### Dooray 웹훅 설정

Dooray 관리자 콘솔에서 **Outgoing Webhook**을 생성하고 URL을 지정합니다.

```
Webhook URL: http://<your-server-ip>:9981/
```

이후 채널에서 메시지를 보내면 Claude Code가 알림을 수신하고, `reply` 도구로 응답합니다.

### 테스트

```bash
# 유닛 테스트
bun test c1/core/adapter.test.ts
bun test c1/core/channel.test.ts
bun test c1/adapters/dooray/dooray.test.ts

# 통합 테스트 (네트워크 불필요, 전체 플로우 검증)
bun test c1/integration.test.ts

# 수동 테스트 — 로컬 웹훅 POST 시뮬레이션
curl -X POST http://localhost:9981/ \
  -H "Content-Type: application/json" \
  -d '{"channelId":"ch-123","senderId":"alice","text":"hello from curl"}'
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
