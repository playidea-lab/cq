# CQ Claude Code Agents

This directory contains specialized Claude Code agents for the CQ project.

## Available Agents

### c4-scout

**Purpose**: Lightweight codebase explorer for context compression

**Features**:
- Explores codebases efficiently using Glob, Grep, and Read
- Compresses findings to ≤500 tokens
- Optimized for cost (uses Haiku model)
- Designed for efficient handoff between agents

**Use Cases**:
- Quick codebase overview for new tasks
- Finding specific files or patterns
- Architecture discovery
- Context compression before expensive operations

## Installation

### For User-Level Access (Recommended)

Copy agent definitions to your user directory:

```bash
# Create agents directory if it doesn't exist
mkdir -p ~/.claude/agents

# Copy c4-scout agent
cp .claude/agents/c4-scout.md ~/.claude/agents/

# Verify installation
ls ~/.claude/agents/
```

### For Project-Level Access

Agents in `.claude/agents/` are already available within this project.

## Usage

### Using c4-scout

In Claude Code, invoke the agent by mentioning it:

```
@c4-scout Show me all API routes in this project
```

Or:

```
@c4-scout Give me an overview of the authentication system
```

The agent will:
1. Use Glob/Grep to find relevant files
2. Read only essential content
3. Return a compressed summary (≤500 tokens)
4. Provide structured, actionable information

### Example Interactions

**Example 1: Finding files**
```
User: @c4-scout Find all database models
Scout:
Models (4 files):
- config.py: ProjectConfig, ValidationConfig
- state.py: ProjectState, TaskState
- task.py: Task, TaskType
- checkpoint.py: Checkpoint

Total: 7 models
```

**Example 2: Architecture overview**
```
User: @c4-scout Understand the authentication flow
Scout:
Auth System Overview:
3 modules, 450 LOC

Core:
- oauth.py: Google/GitHub OAuth
- session.py: JWT sessions (15min expiry)
- token_manager.py: Refresh logic

Security: HTTPOnly cookies, CSRF tokens
DB: supabase.users table
```

## Benefits

### Cost Efficiency
- Uses Haiku model (cheapest tier)
- Minimizes token usage (≤500 tokens)
- Reduces need for expensive full-context reads

### Speed
- Optimized search strategy (Glob → Grep → Read)
- Completes exploration in <30s
- No unnecessary file reads

### Context Management
- Compresses information effectively
- Provides high signal-to-noise ratio
- Enables efficient handoff to other agents

## Agent Design Principles

All C4 agents follow these principles:

1. **Purpose-built**: Each agent has a specific, focused task
2. **Model-appropriate**: Uses the right model tier for the job
3. **Tool-restricted**: Limited to necessary tools only
4. **Token-conscious**: Optimizes for minimal token usage
5. **Handoff-ready**: Output designed for other agents to consume

## Adding New Agents

To add a new agent:

1. Create `{agent-name}.md` in `.claude/agents/`
2. Use the frontmatter format:
   ```yaml
   ---
   name: agent-name
   description: Brief description
   model: haiku|sonnet|opus
   color: blue|green|red|yellow
   tools:
     - Tool1
     - Tool2
   ---
   ```
3. Write clear instructions following the c4-scout template
4. Document usage in this README
5. Test the agent before committing

## Related Documentation

- [C4 MCP Tools](/docs/api/MCP-도구-레퍼런스.md)
- [Agent Routing](/docs/developer-guide/에이전트-라우팅.md)
- [Smart Auto Mode](/docs/user-guide/Smart-Auto-Mode.md)

---

*Last updated: 2026-02-05*
