feature: lighthouse-tdd-enforcement
domain: go-backend
description: promote 시 스키마 검증 강제 + require_stub_first 게이트. config로 on/off.

requirements:
  - type: event-driven
    text: "When promote가 호출되고 enforce_schema가 true이면, the system shall 스키마 불일치 시 에러를 반환하여 promote를 차단한다"
  - type: event-driven
    text: "When promote가 호출되고 enforce_schema가 false이면, the system shall 기존 동작(경고만) 유지한다"
  - type: state-driven
    text: "While require_stub_first가 true이면, the system shall /plan에서 새 MCP 도구 태스크 생성 시 lighthouse stub 등록을 필수로 한다"
  - type: ubiquitous
    text: "The system shall config.yaml의 lighthouse 섹션에서 enforce_schema와 require_stub_first를 읽는다"

non_functional:
  - 기존 253개 implemented 도구에 영향 없음
  - config 기본값: enforce_schema=true, require_stub_first=false

out_of_scope:
  - 기존 도구 재검증
  - CI/CD 연동