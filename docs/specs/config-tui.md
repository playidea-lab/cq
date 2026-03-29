feature: config-tui
domain: cli-tui
source: .c4/ideas/config-tui.md

requirements:
  - id: R1
    type: ubiquitous
    text: "The system shall display config keys grouped by top-level section with count headers, matching sessions/doctor group pattern."
  - id: R2
    type: ubiquitous
    text: "The system shall display each key's current value and source (default/project/global/env) in a 3-column layout: key | value | source."
  - id: R3a
    type: event-driven
    text: "When the user presses Space on a bool key, the system shall toggle the value and write to config.yaml immediately."
  - id: R3b
    type: event-driven
    text: "When the user presses Enter on a string/int key, the system shall enter inline edit mode with the current value pre-filled."
  - id: R4a
    type: event-driven
    text: "When the user presses a on an array key, the system shall enter inline input for a new array item."
  - id: R4b
    type: event-driven
    text: "When the user presses d on an array item, the system shall show delete confirmation (y/N)."
  - id: R4c
    type: event-driven
    text: "When the user presses e on an array key, the system shall open config.yaml in the user's editor."
  - id: R5
    type: state-driven
    text: "While the user is typing, the system shall filter keys by name/value substring. Tab shall cycle section filter."
  - id: R6
    type: ubiquitous
    text: "The system shall reuse sessions/doctor Lipgloss palette, CJK width, badge styles."
  - id: R7
    type: event-driven
    text: "When cq config is run with no subcommand and stdout is a terminal, the system shall launch TUI. get/set subcommands remain unchanged."

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
