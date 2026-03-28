feature: ChatGPT Active Session Tracking
domain: infra/mcp
source: /pi chatgpt-active-session-tracking

requirements:
  - id: R1
    type: event-driven
    text: "When ChatGPT calls any CF Worker MCP tool, the system shall upsert an active session in ai_sessions (INSERT on first call, UPDATE last_seen on subsequent calls)."
  - id: R2
    type: event-driven
    text: "When c4_session_summary is called, the system shall set session status to done and store the summary."
  - id: R3
    type: event-driven
    text: "When an active session's last_seen exceeds 30 minutes, the system shall transition status to done."
  - id: R4
    type: event-driven
    text: "When cq sessions TUI opens, the system shall fetch active ChatGPT sessions from Supabase and display them with a green dot indicator."
  - id: R5
    type: event-driven
    text: "When a ChatGPT session has c4_knowledge_record calls, the system shall display the latest record title as the session summary in TUI."

non_functional:
  - Heartbeat upsert must not add >50ms latency to tool calls
  - Timeout check via Supabase cron (not polling)

out_of_scope:
  - ChatGPT conversation content capture (only metadata)
  - Sessions from tools without MCP (browser ChatGPT without CQ)
