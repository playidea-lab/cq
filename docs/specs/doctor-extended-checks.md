feature: doctor-extended-checks
domain: cli
source: .c4/ideas/doctor-extended-checks.md

requirements:
  - id: R1
    type: event-driven
    text: "When cq serve is running on port 4140, the system shall call fetchServeHealth() and display each component's status as individual check rows. When serve is not running, display a single INFO row."
  - id: R2
    type: optional
    text: "If telegram section exists in config.yaml, the system shall check TELEGRAM_BOT_TOKEN env var."
  - id: R3
    type: event-driven
    text: "When supabase/migrations/*.sql files exist, the system shall count local migrations and report."
  - id: R4
    type: ubiquitous
    text: "The system shall verify ~/.c4/named-sessions.json is readable and valid JSON."
  - id: R5
    type: ubiquitous
    text: "The system shall validate config.yaml has project_id set and report missing required sections."

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cq doctor --plain | grep -E 'serve|telegram|migration|session|config'"
