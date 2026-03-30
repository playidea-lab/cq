feature: session-dashboard
domain: web-backend
version: 1.0

description: |
  cq sessions를 bubbletea 기반 인터랙티브 TUI 대시보드로 교체.
  프로젝트 진행 상태 그룹핑, 실시간 검색, 키보드 선택+시작.

requirements:
  - id: R1
    pattern: ubiquitous
    text: "cq sessions는 bubbletea 기반 인터랙티브 TUI를 기본 모드로 실행해야 한다"
  - id: R2
    pattern: ubiquitous
    text: "세션 목록을 status별로 그룹핑 (in-progress > planned > idea > active > done)"
  - id: R3
    pattern: event-driven
    text: "문자 입력 시 태그명, summary, idea slug, idea/spec 내용에서 실시간 필터링"
  - id: R4
    pattern: event-driven
    text: "Enter 시 선택한 세션을 cq -t <tag>로 시작"
  - id: R5
    pattern: event-driven
    text: "Tab 시 status 필터 순환 (전체→active→done→idea→planned)"
  - id: R6
    pattern: state-driven
    text: "done 세션에서 Enter 시 status를 active로 변경 후 시작"
  - id: R7
    pattern: optional
    text: "--plain 플래그로 기존 텍스트 출력 모드"
  - id: R8
    pattern: ubiquitous
    text: "stdout이 TTY가 아닐 때 자동으로 plain 모드 fallback"

non_functional:
  - TUI 시작 시간 500ms 이내 (파일 인덱싱 포함)
  - 100개 세션까지 부드러운 스크롤
  - 기존 --plain 출력은 정확히 현재 동작 유지

out_of_scope:
  - 세션 내용 미리보기 패널 (cmd+click으로 대체)
  - 세션 삭제/편집 (cq session rm 사용)
  - 원격 세션 동기화

existing_components:
  - sessionsCmd (init_session.go) — 현재 텍스트 출력
  - named-sessions.json — 세션 데이터
  - bubbletea/lipgloss/bubbles — go.mod에 이미 존재
  - chat.go — bubbletea 사용 패턴 참조

verification:
  - type: cli
    command: "cq sessions"
    expect: "TUI 인터랙티브 모드 실행"
  - type: cli
    command: "cq sessions --plain"
    expect: "기존 텍스트 출력"
  - type: cli
    command: "echo test | cq sessions"
    expect: "non-TTY에서 plain 모드 fallback"
