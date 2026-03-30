feature: session-exit-done-split
domain: web-backend
version: 1.0

description: |
  세션 exit/done 분리 + TUI 텍스트 입력 개선.
  /exit는 상태 변경 없이 light capture만, /done은 full capture + done 전환.
  세션 이름 입력에 bubbles textinput 적용하여 커서 이동 지원.

requirements:
  - id: R1
    pattern: ubiquitous
    text: "세션 TUI의 이름 입력 필드는 bubbles textinput 컴포넌트를 사용하여 좌/우 화살표, Home/End, 단어 점프를 지원해야 한다"
  - id: R2
    pattern: event-driven
    text: "WHEN 사용자가 /exit 하면, THEN 시스템은 세션 상태를 변경하지 않고 가벼운 방문 메모만 기록해야 한다"
  - id: R3
    pattern: event-driven
    text: "WHEN 사용자가 /done 하면, THEN 시스템은 풀 캡처를 수행해야 한다 (전체 요약 + 지식 기록 + 페르소나 학습 + 상태 → done)"
  - id: R4
    pattern: state-driven
    text: "WHILE 세션에 연결된 태스크가 전부 완료 상태이면, THEN 시스템은 자동으로 done 전환 + 풀 캡처를 실행해야 한다"
  - id: R5
    pattern: unwanted
    text: "/exit 시 세션 상태가 done으로 변경되어서는 안 된다"

non_functional:
  - "light capture는 best-effort — 실패해도 exit 차단하지 않음"
  - "기존 captureSession 호출처 하위호환 유지"
  - "named-sessions.json 스키마 변경 없음"

out_of_scope:
  - "세션 자동 정리/GC"
  - "Supabase 클라우드 동기화"
  - "세션 시작 시 맥락 주입"

verification:
  - type: cli
    command: "go build ./cmd/c4/..."
    expect: "빌드 성공"
  - type: unit
    command: "go vet ./cmd/c4/..."
    expect: "vet 통과"
