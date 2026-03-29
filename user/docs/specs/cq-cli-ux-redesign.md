feature: CQ CLI UX Redesign
domain: cli
source: /pi cq-cli-ux-redesign

requirements:
  - id: R1
    pattern: event-driven
    text: "When `cq` is executed without arguments and user is not logged in, the system shall initiate OAuth login flow inline"
  - id: R2
    pattern: event-driven
    text: "When `cq` is executed without arguments and serve is not running, the system shall install and start the OS service"
  - id: R3
    pattern: event-driven
    text: "When `cq` is executed without arguments and serve is already running, the system shall display status summary and exit"
  - id: R4
    pattern: ubiquitous
    text: "AI tool launchers (claude/cursor/codex/gemini) shall be separate subcommands with their own flags (-t, --bot)"
  - id: R5
    pattern: ubiquitous
    text: "`cq stop` shall stop the CQ OS service; C4 task stop shall move to `cq run stop`"
  - id: R6
    pattern: ubiquitous
    text: "`--help` shall display only ~10 main commands; remaining 30+ commands shall be hidden"
  - id: R7
    pattern: ubiquitous
    text: "`cq serve` shall remain as foreground mode for debugging (hidden command)"
  - id: R8
    pattern: ubiquitous
    text: "`cq auth` shall be hidden; login is handled inline by `cq` root command"

non_functional:
  - Backward compatibility: all hidden commands still work, just not shown in --help
  - Zero breaking changes to cq serve, cq auth, cq claude subcommands

out_of_scope:
  - New features (relay, journal, etc.) — only CLI surface changes
  - install.sh changes (already handled in v1.26.3)
