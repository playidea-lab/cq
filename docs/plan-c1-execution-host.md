# C1 Execution Host: "The Hands of AI" Blueprint

`c1`을 단순 모니터링 앱에서 AI 에이전트의 실제 실행 주체(Execution Host)로 전환하기 위한 설계도입니다.

## 1. 계층 아키텍처
- **AI Layer (c4/Gemini)**: "무엇을 할 것인가" 결정 (Decision Making)
- **Tool Layer (c5/Go)**: "어떻게 할 것인가" 도구 제공 (MCP/RPC)
- **Execution Layer (c1/Tauri)**: "직접 실행" (Native OS Operation)

## 2. 핵심 기능 (The Hands)
### 2.1 Native Terminal Bridge
- Tauri의 `Command` API를 사용하여 하위 프로세스 실행.
- `xterm.js`를 통한 실시간 stdout/stderr 스트리밍.
- 인터랙티브 입력을 위한 stdin 채널 확보.

### 2.2 Atomic File Operations
- AI가 특정 섹션만 수정할 수 있도록 `tauri::fs` 기반의 정밀 수정(Edit) API.
- 수정 전 백업 및 롤백 기능 내장.

### 2.3 Approval-First Workflow (HITL Gate)
- 위험 명령어(rm, push, deploy) 실행 전 UI에서 명시적 승인 대기.
- 에이전트의 "의도(Intent)"를 사용자 언어로 번역하여 승인창에 표시.

### 2.4 Visual Feedback Loop
- 현재 UI의 특정 영역 또는 전체 화면 스크린샷 캡처.
- Gemini 3.0에게 전달하여 시각적 정합성 검토.

## 3. 통신 프로토콜
- `c5`의 RPC 서버가 `c1`의 로컬 웹소켓 서버와 통신.
- AI -> c5 -> c1 순으로 명령 전달.

## 4. 보안 가이드라인
- **Sandbox**: 모든 실행은 프로젝트 루트(`/Users/changmin/git/cq`) 내로 제한.
- **Whitelist**: 허용된 명령어 외 실행 차단.
- **Log Audit**: 모든 네이티브 호출은 `.c4/logs/c1_execution.log`에 기록.
