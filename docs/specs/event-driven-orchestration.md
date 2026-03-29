feature: event-driven-orchestration
domain: go-backend
description: Supabase Realtime으로 c4_tasks 변경을 실시간 감지하여 텔레그램 알림 자동 발송. 기존 RealtimeClient에 c4_tasks subscribe 추가.

requirements:
  - type: event-driven
    text: "When c4_tasks 테이블에서 status가 done으로 변경되면, the system shall 텔레그램으로 완료 알림을 보낸다"
  - type: event-driven
    text: "When c4_tasks 테이블에서 status가 blocked으로 변경되면, the system shall 텔레그램으로 차단 알림을 보낸다"
  - type: state-driven
    text: "While 텔레그램이 설정 안 되었으면, the system shall 알림을 skip한다"
  - type: optional
    text: "If notifications.events config가 설정되면, the system shall 해당 이벤트만 알린다"

non_functional:
  - 기존 c1_messages subscribe에 영향 없음
  - 알림 실패 시 시스템 동작에 영향 없음 (best-effort)

out_of_scope:
  - /run 폴링 제거 (Stage 2)
  - 워커 자동 스폰 (Stage 2)
  - Slack/Discord (미래)