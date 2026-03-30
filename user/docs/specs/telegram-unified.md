feature: telegram-unified
domain: go-backend
summary: CQ 알림/세션을 텔레그램으로 통합, Dooray 및 멀티채널 webhook 전면 폐기

requirements:
  ubiquitous:
    - "notify 시스템은 Telegram Bot API sendMessage로 알림을 전송해야 한다"
    - "notifyhandler는 botstore에서 bot_token과 chat_id를 읽어야 한다"
    - "notifications.json은 bot_username과 events 필터를 저장해야 한다"
    - "c4_notification_set 도구는 channel 파라미터 대신 bot_username을 받아야 한다"

  event_driven:
    - "WHEN c4_notify 호출 시 THEN botstore에서 token/chat_id를 조회하여 Telegram sendMessage API로 전송한다"
    - "WHEN c4_notification_set 호출 시 THEN bot_username을 notifications.json에 저장한다"
    - "WHEN 존재하지 않는 bot_username이 지정되면 THEN 에러를 반환한다"

  state_driven:
    - "WHILE notifications.json에 bot_username이 설정되어 있을 때 THEN c4_notify는 해당 봇으로 알림을 전송한다"
    - "WHILE notifications.json이 없을 때 THEN c4_notify는 'not configured' 에러를 반환한다"

  unwanted:
    - "Dooray webhook 코드가 남아있으면 안 된다 (doorayhandler, dooraySender, Hub dooray 라우트)"
    - "Slack, Discord, Teams Sender가 notify 패키지에 남아있으면 안 된다"
    - "notifications.json에 webhook_url 필드가 남아있으면 안 된다"

out_of_scope:
  - "C1 텔레그램 전용 재정의 (별도 계획)"
  - "Hub fallback LLM 텔레그램 전환 (별도 계획)"
  - "텔레그램 양방향 대화 세션 (TBS에서 구현)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/notify/..."
    - "cd c4-core && go test ./internal/mcp/handlers/notifyhandler/..."
    - "cd c5 && go build ./..."
