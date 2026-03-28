
feature: CQ Unified TUI Hub
domain: cli-tui
version: "1.0"

requirements:
  ubiquitous:
    - id: U1
      text: "글로벌 키바인딩(t/a/g/d/i/Esc)은 모든 TUI 모드에서 동작한다"
    - id: U2
      text: "cq 실행 시 sessions TUI가 기본 화면이다 (dashboard 대체)"
    - id: U3
      text: "로그인 안 됨 → 로그인 TUI → 완료 후 sessions"

  event_driven:
    - id: E1
      text: "WHEN 사용자가 글로벌 키를 누르면 THEN 현재 TUI에서 해당 TUI로 전환"
    - id: E2
      text: "WHEN Esc를 누르면 THEN sessions(홈)로 복귀"
    - id: E3
      text: "WHEN 토큰 만료 감지 THEN 로그인 TUI로 자동 전환"
    - id: E4
      text: "WHEN 첫 실행(onboarded=false) THEN 온보딩 화면 표시 후 sessions"

  optional:
    - id: O1
      text: "IF ? 키 THEN 대시보드(커맨드 레퍼런스) 표시"

  unwanted:
    - id: W1
      text: "TUI 전환 시 화면 깜빡임 최소화 (alt screen 유지)"
    - id: W2
      text: "한 TUI의 상태가 다른 TUI에 영향 주지 않음"

non_functional:
  - "각 TUI 전환 시간 < 50ms (프로세스 재생성이 아닌 루프 기반)"
  - "검색/입력 모드에서는 글로벌 키 비활성화 (충돌 방지)"
  - "기존 서브커맨드(cq ideas, cq doctor 등) 동작 유지"

out_of_scope:
  - "TUI 간 상태 공유 (각 TUI는 독립적으로 데이터 로딩)"
  - "멀티 패널/split view"
  - "dashboard.go 삭제 (deprecated 처리만, 서브커맨드 호환 유지)"

risks:
  - id: R1
    text: "글로벌 키가 검색 입력과 충돌"
    severity: medium
    mitigation: "검색/입력 모드일 때 글로벌 키 비활성화"
  - id: R2
    text: "화면 깜빡임"
    severity: low
    mitigation: "tea.WithAltScreen() 사용으로 alt screen 간 전환"
  - id: R3
    text: "기존 cq 서브커맨드 호환성"
    severity: medium
    mitigation: "cq doctor, cq ideas 등은 기존대로 독립 TUI 실행"
