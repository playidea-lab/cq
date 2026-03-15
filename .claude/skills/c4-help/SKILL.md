---
name: c4-help
description: |
  Quick reference for C4 skills, agents, and tools. Provides summaries,
  decision trees, and keyword search across 22 skills, 37 agents (9 categories),
  and 133 MCP tools (107 base + 26 hub). Use when the user needs help, reference,
  skill list, or wants to search C4 capabilities. Triggers: "도움말",
  "명령어 목록", "도구 검색", "help", "list commands", "show agents",
  "what tools", "how to".
---

# C4 Help

Quick reference for commands, agents, and tools. Parse `$ARGUMENTS` and branch accordingly.

## Usage

```
/c4-help              → Full summary
/c4-help commands     → All skills
/c4-help agents       → Agents by category
/c4-help tools        → MCP tools (3 layers)
/c4-help <keyword>    → Keyword search
```

## Instructions

Parse `$ARGUMENTS` for branch. No MCP calls needed (static output).

### No args → Decision Tree + Core Commands

```
What's the task?
├─ 1-line fix → Just fix it (no C4)
├─ Small (1-5 files) → /c4-quick "desc" → /c4-submit
├─ Medium (5-15 files) → /c4-add-task → /c4-run  OR  c4_claim → c4_report
├─ Large (15+ files) → /c4-plan → /c4-run N  OR  /c4-swarm N
├─ Research/experiment → /c4-research
└─ Document work → /c4-review (paper) OR c4_parse_document

Core: /c4-status, /c4-quick, /c4-run, /c4-submit, /c4-validate
More: /c4-help commands | agents | tools | <keyword>
```

### "commands" → See `references/commands.md`

### "agents" → See `references/agents.md`

### "tools" → See `references/tools.md`

### Other → Keyword search across `references/search-data.md`

Output matching commands, agents, and tools. If no matches: "No results. Use /c4-help for full list."
