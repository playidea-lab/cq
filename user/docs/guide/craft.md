# CQ Craft — Build and Install Your Own Tools

> Design agent behavior directly as a user.

---

## Why Craft?

Claude Code customizes agent behavior with three types: skill, agent, and rule.
CQ Craft is the **unified interface to install, create, and manage all three**.

```
I have a repeating pattern
→ Check presets? → cq add
→ Not there? → Create it conversationally with /craft
→ Available in community? → cq add owner/repo:name
```

---

## Four Tool Types

| Type | Role | Storage Path | Usage |
|------|------|------|--------|
| **Skill** | Workflow (multi-step) | `~/.claude/skills/{name}/SKILL.md` | `/{name}` or trigger keyword |
| **Agent** | Expert/persona | `~/.claude/agents/{name}.md` | Auto-detected (trigger keyword) |
| **Rule** | Always-applied constraint | `~/.claude/rules/{name}.md` | Automatically applied to all conversations |
| **CLAUDE.md** | Project guidelines | `./CLAUDE.md` | Auto-loaded when project opens |

---

## Quick Start

### 1. Browse Presets with TUI

```bash
cq add
```

Browse 53 built-in presets by category. Navigate with arrow keys, install with Enter.

### 2. Install by Name

```bash
cq add code-review        # Code review checklist skill
cq add go-pro             # Go expert agent
cq add no-console-log     # console.log ban rule
cq add python-project     # Python project CLAUDE.md
```

### 3. Install from GitHub

```bash
# Official Anthropic skills
cq add anthropics/skills:pdf
cq add anthropics/skills:pptx
cq add anthropics/skills:xlsx

# Superpowers community skills
cq add obra/superpowers:brainstorming
cq add obra/superpowers:test-driven-development

# Full URL also works
cq add https://github.com/anthropics/skills/tree/main/skills/webapp-testing
```

### 4. Create Interactively

In Claude Code, type `/craft`:

```
> Create a skill — always run go vet + tests before commit

[Decision] This is a skill — it's a multi-step workflow.

Name: pre-commit-check
Triggers: "commit check", "pre-commit"
Actions: go vet → go test → report results

Shall we create this?
```

---

## Management

```bash
# List installed tools
cq list --mine

# Update remote-installed skill
cq update pdf

# Delete
cq remove brainstorming
cq remove --force pdf     # No confirmation
```

---

## Craft's Place in the CQ Workflow

The complete CQ development loop:

```
/pi (idea) → /plan (design) → /run (execute) → /finish (wrap-up)
```

Craft is the **meta-tool to customize this loop**:

| Stage | What Craft Lets You Do |
|------|---------------------|
| Before `/pi` | Add custom brainstorming skill with `/craft` |
| During `/plan` | Install API design checklist with `cq add api-design` |
| During `/run` | Add Go expert agent with `cq add go-pro` |
| After `/finish` | Automate release notes with `cq add release-notes` |
| Code review | Install 6-axis review checklist with `cq add code-review` |
| New project | Auto-generate CLAUDE.md with `cq add python-project` |

### Example: New Project Setup

```bash
# 1. Create CLAUDE.md
cq add go-project                    # Go project base guidelines

# 2. Install team rules
cq add strict-types                  # Ban any type
cq add test-naming                   # Test function naming rules
cq add error-handling                # No error ignoring

# 3. Install review workflow
cq add code-review                   # 6-axis code review

# 4. Add community skills
cq add anthropics/skills:pdf         # PDF generation

# 5. Create your own skill
/craft                               # Create custom skill interactively
```

---

## Built-in Preset Catalog

### Skills (16)

| Name | Description |
|------|------|
| code-review | 6-axis code review checklist |
| pr-template | PR creation + auto-check |
| daily-standup | Daily standup summary (git log based) |
| deploy-checklist | Pre-deployment verification checklist |
| test-first | TDD cycle guide |
| hotfix-flow | Emergency fix workflow |
| git-cleanup | Branch cleanup workflow |
| migration-guide | Safe DB migration execution |
| incident-runbook | Incident response runbook |
| api-design | REST API design checklist |
| security-audit | Security audit workflow |
| onboarding | New team member onboarding guide |
| release-notes | Auto-generate release notes |
| refactor-plan | Refactoring plan strategy |
| perf-check | Performance check checklist |
| doc-review | Documentation review checklist |

### Agents (17)

| Name | Description |
|------|------|
| rust-pro | Rust expert |
| python-pro | Python expert |
| go-pro | Go expert |
| ts-pro | TypeScript expert |
| kotlin-pro | Kotlin expert |
| swift-pro | Swift expert |
| reviewer | Senior code reviewer |
| architect | Systems architect |
| mentor | Junior mentor |
| sql-expert | SQL/DB expert |
| devops-pro | DevOps expert |
| security-pro | Security expert |
| api-designer | API design expert |
| tech-writer | Technical writer |
| data-engineer | Data engineer |
| ux-reviewer | UX reviewer |
| perf-expert | Performance expert |

### Rules (12)

| Name | Description |
|------|------|
| no-console-log | Ban console.log/print debug output |
| korean-comments | Comments must be in Korean |
| strict-types | Ban any type |
| no-magic-numbers | Ban magic numbers |
| import-order | Import ordering rules |
| error-handling | No error ignoring |
| no-todo-comments | Ban TODO comments → use issue tracker |
| no-hardcoded-urls | Ban hardcoded URLs |
| max-function-length | Functions must be 50 lines or less |
| test-naming | Test function naming rules |
| no-god-objects | Ban God Objects |
| log-level-policy | Log level policy |

### CLAUDE.md (8)

| Name | Description |
|------|------|
| general | Generic template |
| go-project | Go project |
| python-project | Python project |
| web-frontend | React/TypeScript frontend |
| rust-project | Rust project |
| kotlin-android | Kotlin Android |
| ml-experiment | ML experiment project |
| monorepo | Monorepo |

---

## Recommended GitHub Sources

| Source | Description | Install |
|------|------|------|
| [Anthropic Official](https://github.com/anthropics/skills) | Document skills: PDF, DOCX, PPTX, XLSX, etc. | `cq add anthropics/skills:<name>` |
| [Superpowers](https://github.com/obra/superpowers) | TDD, brainstorming, debugging methodology | `cq add obra/superpowers:<name>` |
| [awesome-claude-skills](https://github.com/travisvn/awesome-claude-skills) | Community-curated list | Install individual items by URL |
