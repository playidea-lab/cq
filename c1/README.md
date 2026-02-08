# C4 Canvas

> Auto-Map Project Visualizer - 프로젝트 맥락을 무한 캔버스에 자동 시각화

## Features

- **자동 스캔**: `.claude/`, `.c4/`, `docs/` 등 프로젝트 파일 자동 스캔
- **노드 시각화**: 파일 타입별 노드로 표시 (Document, Config, Session, Task, Connection)
- **시간순 레이아웃**: 파일 수정 시간 기준 자동 배치 (왼쪽=과거, 오른쪽=최근)
- **상세 패널**: 노드 클릭 시 메타데이터 표시
- **영속 저장**: 캔버스 상태 `.c4/canvas.json`에 저장

## Tech Stack

- **Desktop**: [Tauri](https://tauri.app/) v2
- **Canvas**: [tldraw](https://tldraw.dev/) v4
- **Frontend**: React + TypeScript + Vite
- **Backend**: Rust

## Development

### Prerequisites

- Node.js 18+
- pnpm
- Rust (rustup)

### Setup

```bash
# Install dependencies
pnpm install

# Run development server
pnpm tauri dev
```

### Build

```bash
pnpm tauri build
```

## Usage

1. "Open Project" 클릭하여 C4 프로젝트 선택
2. 자동으로 노드 생성 및 배치
3. 노드 클릭하여 상세 정보 확인
4. 노드 드래그로 재배치 (위치는 자동 저장)
5. "Refresh" 클릭하여 재스캔

## Node Types

| 타입 | 아이콘 | 설명 |
|------|--------|------|
| Document | 📄 | Markdown 문서 |
| Config | ⚙️ | 설정 파일 (.claude, .c4, yaml, json) |
| Session | 💬 | Claude 세션 기록 |
| Task | ✅ | C4 태스크 |
| Connection | 🔗 | MCP/환경 연결 |

## License

MIT
