# C4 Commands

## Daily (6)

| Command | Purpose | Args | Example |
|---------|---------|------|---------|
| /c4-status | Check status | (none) | /c4-status |
| /c4-quick | Start task now | "desc" [scope=path] | /c4-quick "fix: timeout bug" |
| /c4-run | Parallel workers | [N] [--continuous] | /c4-run 3 |
| /c4-submit | Submit completion | [task-id] | /c4-submit T-001 |
| /c4-validate | Run validation | (none) | /c4-validate |
| c4_claim/report | Direct mode | task_id (MCP) | c4_claim("T-001-0") |

## Weekly (5)

| Command | Purpose | Args | Example |
|---------|---------|------|---------|
| /c4-plan | Large planning | (none) | /c4-plan |
| /c4-add-task | Add task | "desc" [--domain D] | /c4-add-task "JWT auth" |
| /c4-checkpoint | Review checkpoint | (none) | /c4-checkpoint |
| /c4-swarm | Team collaboration | [N] [--review] [--investigate] | /c4-swarm --review |
| /c4-stop | Stop execution | (none) | /c4-stop |

## Occasional (5)

| Command | Purpose | Args | Example |
|---------|---------|------|---------|
| /c4-interview | Deep interview | "topic" | /c4-interview "realtime sync" |
| /c4-release | Generate changelog | (none) | /c4-release |
| /c4-research | Research iteration | [start\|status\|next\|record\|approve] | /c4-research next |
| /c4-review | Paper review | <pdf_path> | /c4-review paper.pdf |
| /c4-init | Project init | (none) | /c4-init |

## Operations (1)

| Command | Purpose | Warning |
|---------|---------|---------|
| /c4-clear | Full state reset | Irreversible |
