feature: project-notification-channels
domain: infra
description: 프로젝트 단위 알림 시스템 — 워커 오프라인, 잡 완료/실패 등 이벤트를 프로젝트 채널로 자동 알림

requirements:
  - type: ubiquitous
    text: "The system shall store notification channels per project in project_notification_channels table"
  - type: event-driven
    text: "When a user runs cq notify add telegram, the system shall generate a pairing code, store it in notification_pairings table, and prompt the user to send /start <code> to the bot"
  - type: event-driven
    text: "When the telegram bot receives /start <code>, the system shall verify the code against notification_pairings, save the chat_id to project_notification_channels, and confirm to the user"
  - type: event-driven
    text: "When hub_workers.status transitions from online to offline, the system shall query project_notification_channels for that worker's project and send alerts to all registered channels"
  - type: event-driven
    text: "When a hub_job reaches terminal state (COMPLETE/FAILED/CANCELLED), the system shall send alerts to the project's notification channels"
  - type: state-driven
    text: "While a pairing code is valid (5 minutes TTL), the system shall accept /start <code> from any Telegram user"
  - type: state-driven
    text: "While a project has no notification channels, the system shall skip alerting silently"
  - type: optional
    text: "If a project has multiple channels (telegram + slack), the system shall send to all channels in parallel"
  - type: unwanted
    text: "If a pairing code is expired or invalid, the system shall respond with an error message to the Telegram user"
  - type: unwanted
    text: "If the Telegram Bot API returns an error, the system shall log the error and continue (non-fatal)"

non_functional:
  - alert latency under 10 seconds from event to delivery
  - pairing code 5 minute TTL
  - channel types extensible (telegram now, slack/email later)

out_of_scope:
  - slack/email channel implementation (future)
  - per-event mute/snooze UI
  - alert deduplication across channels