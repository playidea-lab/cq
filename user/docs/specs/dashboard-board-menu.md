feature: dashboard-board-menu
domain: cli-tui
summary: 대시보드 명령어 카탈로그를 게시판 메뉴 리스트로 교체

requirements:
  - id: R1
    pattern: ubiquitous
    text: 대시보드는 게시판 리스트(이름+단축키+설명)를 표시한다
  - id: R2
    pattern: event-driven
    text: ↑↓ 키로 게시판 간 커서 이동
  - id: R3
    pattern: event-driven
    text: Enter 키로 선택된 게시판의 TUI 화면 진입
  - id: R4
    pattern: event-driven
    text: 단축키(t/i/a/d/g)로 직접 해당 TUI 진입
  - id: R5
    pattern: state-driven
    text: coming soon 게시판은 선택 시 진입하지 않고 안내 표시
  - id: R6
    pattern: ubiquitous
    text: ? 키는 Help 화면(명령어 카탈로그)을 표시한다
  - id: R7
    pattern: ubiquitous
    text: 상단 서비스 상태는 3-5초 tick으로 갱신한다

non_functional:
  - 대시보드 표시까지 <500ms
  - 기존 단축키 동작 100% 호환
  - 최소 터미널 80x24 지원

out_of_scope:
  - 게시판별 요약 데이터 (v2)
  - Workers/Metrics 게시판 실제 구현 (coming soon만)
  - 게시판 커스터마이징

verification:
  type: cli
  commands:
    - go build ./cmd/c4/...
    - go test ./cmd/c4/... -run TestDashboard
    - go vet ./cmd/c4/...