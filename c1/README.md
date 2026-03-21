# C1 — Channel Adapter

채널 기반 메시징 브릿지. PlatformAdapter 인터페이스로 다양한 메신저를 Claude Code에 연결합니다.

> Tauri 데스크톱 앱에서 채널 어댑터 아키텍처로 전환 (2026-03).
> 기본 진입점은 Telegram 봇으로 이전 예정 (`telegram-bot-session` spec 참조).

## 아키텍처

```
메신저 (Telegram/Dooray/...)
  ↕ PlatformAdapter
MCP Channel Server (stdio)
  ↕
Claude Code
```

## 디렉토리 구조

```
c1/
├── core/
│   ├── adapter.ts       # PlatformAdapter 인터페이스 + AdapterRegistry
│   ├── channel.ts       # MCP Channel 서버
│   └── auth.ts          # allowlist, pairing code, sender gating
├── adapters/
│   └── dooray/          # Dooray Messenger 어댑터
├── index.ts             # 진입점
├── integration.test.ts  # 통합 테스트
└── package.json
```

## Quick Start

```bash
bun install
bun test                           # 유닛 테스트
bun test integration.test.ts       # 통합 테스트
```

## Dooray 어댑터

| 환경 변수 | 설명 | 기본값 |
|----------|------|--------|
| `DOORAY_BOT_TOKEN` | Dooray REST API Bearer 토큰 | (필수) |
| `DOORAY_TENANT_ID` | Dooray 테넌트 ID | (필수) |
| `DOORAY_API_URL` | Dooray API Base URL | `https://api.dooray.com` |
| `DOORAY_LISTEN_PORT` | 로컬 웹훅 수신 포트 | `9981` |

```bash
# Claude Code에 연결
claude --mcp-server "bun run c1/index.ts" --channels
```
