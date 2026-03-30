feature: doctor-tui
domain: cli-tui
source: .c4/ideas/doctor-tui.md

requirements:
  - id: R1
    type: ubiquitous
    text: "The system shall display check results grouped by severity (FAIL → WARN → INFO → OK) with count headers."
  - id: R2
    type: event-driven
    text: "When a network check starts, the system shall show a spinner; when it completes, the system shall update the row in-place."
  - id: R3a
    type: event-driven
    text: "When the user presses Enter on a safe-fix item, the system shall execute the fix immediately and refresh the check result."
  - id: R3b
    type: event-driven
    text: "When the user presses Enter on a dangerous-fix item, the system shall show a confirmation prompt (y/N) before executing."
  - id: R4
    type: event-driven
    text: "When the user presses → on a check item, the system shall show detail view with error message, log, and fix command."
  - id: R5
    type: event-driven
    text: "When the user presses r, the system shall re-run all checks (or selected check if in detail view) with spinners."
  - id: R6
    type: state-driven
    text: "While the user is typing, the system shall filter checks by name/message substring. Tab shall cycle status filter (ALL → FAIL → WARN → OK)."
  - id: R7
    type: ubiquitous
    text: "The system shall use the same Lipgloss color palette, CJK width calculation, and badge styles as cq sessions."
  - id: R8
    type: optional
    text: "If --plain flag is set, the system shall output text format (current behavior). If --json flag is set, the system shall output JSON."

non_functional:
  - "All 19 checks must complete within 15s total (timeout per network check: 5s)"
  - "TUI must render correctly in terminals >= 80 columns wide"

out_of_scope:
  - "Adding new check items (reuse existing 19)"
  - "Watch mode / auto-refresh timer"
  - "Remote doctor (diagnosing other machines)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./cmd/c4/ -run TestDoctor -v"
