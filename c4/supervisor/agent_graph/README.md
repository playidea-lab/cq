# Agent Graph System

Graph-based agent routing with 4-layer architecture for intelligent task-to-agent matching.

## Overview

The Agent Graph system provides a flexible, extensible framework for routing tasks to the most appropriate agents based on:

- **Skills**: Atomic capabilities that agents possess
- **Agents**: Personas with skills and relationships
- **Domains**: Problem areas with preferred workflows
- **Rules**: Routing overrides and chain extensions

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Rules Layer                          │
│  Overrides, Chain Extensions, Conditional Routing           │
├─────────────────────────────────────────────────────────────┤
│                       Domains Layer                         │
│  web-backend, web-frontend, ml-dl, infra, ...               │
├─────────────────────────────────────────────────────────────┤
│                       Agents Layer                          │
│  backend-architect, debugger, test-automator, ...           │
├─────────────────────────────────────────────────────────────┤
│                       Skills Layer                          │
│  python-coding, debugging, api-design, testing, ...         │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Basic Usage

```python
from c4.supervisor.agent_graph import (
    AgentGraph,
    AgentGraphLoader,
    GraphRouter,
    SkillMatcher,
    TaskContext,
)

# Load definitions from YAML files
loader = AgentGraphLoader(base_dir=Path(".c4/agents"))
graph = AgentGraph()

# Add loaded definitions to graph
for skill in loader.load_skills():
    graph.add_skill(skill)
for agent in loader.load_agents():
    graph.add_agent(agent)
for domain in loader.load_domains():
    graph.add_domain(domain)

# Create router with skill matching
skill_matcher = SkillMatcher(graph)
router = GraphRouter(skill_matcher=skill_matcher, graph=graph)

# Route a task
task = TaskContext(title="Fix Python API bug", scope="api/endpoints.py")
config = router.get_recommended_agent("web-backend", task=task)

print(f"Primary agent: {config.primary}")
print(f"Agent chain: {config.chain}")
```

### Skill-Based Routing

```python
from c4.supervisor.agent_graph import SkillMatcher, TaskContext

# Create task context
task = TaskContext(
    title="Debug authentication error",
    description="Users report 401 errors on login",
    scope="auth/login.py",
    task_type="bugfix"
)

# Extract required skills from task
skills = skill_matcher.extract_required_skills(task)
# ['debugging', 'python-coding']

# Find best matching agents
agents = skill_matcher.find_best_agents(skills)
# [AgentMatch(agent_id='debugger', score=1.5, matched_skills=['debugging']), ...]

# Get recommended agent
agent = skill_matcher.get_recommended_agent(task)
# 'debugger'
```

### Routing with Details

```python
from c4.supervisor.agent_graph import GraphRouter, RoutingResult

result: RoutingResult = router.get_recommended_agent_with_details(
    domain="web-backend",
    task=task
)

print(f"Agent: {result.config.primary}")
print(f"Method: {result.routing_method}")  # 'skill', 'domain', or 'task_type'
print(f"Matched skills: {result.matched_skills}")
print(f"Score: {result.skill_score}")
```

## Components

### AgentGraph

NetworkX-based graph storing skills, agents, and domains with their relationships.

```python
from c4.supervisor.agent_graph import AgentGraph, NodeType, EdgeType

graph = AgentGraph()

# Query methods
skills = graph.skills  # All skill IDs
agents = graph.agents  # All agent IDs
domains = graph.domains  # All domain IDs

# Find agents with a skill
python_agents = graph.find_agents_with_skill("python-coding")

# Find handoff targets (sorted by weight)
targets = graph.find_handoff_targets("backend-architect")
# [('python-pro', 0.9), ('test-automator', 0.8)]

# Find shortest path between agents
path = graph.get_path("backend-architect", "code-reviewer")
# ['backend-architect', 'python-pro', 'code-reviewer']
```

### AgentGraphLoader

Loads and validates YAML definitions against JSON schemas.

```python
from c4.supervisor.agent_graph import AgentGraphLoader

loader = AgentGraphLoader(base_dir=Path("agents/"))

# Load all definitions
all_defs = loader.load_all()
# {'skills': [...], 'agents': [...], 'domains': [...], 'rules': [...]}

# Load specific types
skills = loader.load_skills()
agents = loader.load_agents()
domains = loader.load_domains()
rules = loader.load_rules()

# Load by ID
skill = loader.load_skill_by_id("python-coding")
agent = loader.load_agent_by_id("backend-architect")
```

### SkillMatcher

Matches tasks to skills and agents based on triggers.

```python
from c4.supervisor.agent_graph import SkillMatcher, TaskContext

matcher = SkillMatcher(graph)

# Task matching uses:
# - keywords in title/description
# - task_type matching
# - file pattern matching (scope)

task = TaskContext(
    title="Optimize database queries",
    scope="db/queries.py",
    task_type="optimization"
)

skills = matcher.extract_required_skills(task)
agents = matcher.find_best_agents(skills)
```

### GraphRouter

Routes tasks to agents with priority: task_type override → skill matching → domain fallback.

```python
from c4.supervisor.agent_graph import GraphRouter

# With skill matching
router = GraphRouter(skill_matcher=matcher, graph=graph)

# Legacy mode (domain-only)
router = GraphRouter()

# Get recommendation
config = router.get_recommended_agent("web-backend", task=task)

# Delegation to legacy router
chain = router.get_chain_for_domain("web-backend")
instructions = router.get_handoff_instructions("web-backend")
```

## YAML Definitions

### Skill Definition

```yaml
skill:
  id: python-coding
  name: "Python Development"
  description: "Writing Python code and modules"

  capabilities:
    - write-python-code
    - debug-python
    - optimize-python

  triggers:
    keywords:
      - python
      - py
      - django
      - flask
    task_types:
      - feature
      - refactor
    file_patterns:
      - "*.py"
      - "**/python/**"

  tools:
    - python-interpreter
    - pytest

  complementary_skills:
    - testing
    - debugging
```

### Agent Definition

```yaml
agent:
  id: backend-architect
  name: "Backend Architect"

  persona:
    role: "Senior backend architecture specialist"
    expertise: "System design, APIs, databases, scalability"

  skills:
    primary:
      - api-design
      - database-design
      - python-coding
    secondary:
      - debugging
      - code-review

  relationships:
    hands_off_to:
      - agent: python-pro
        when: "Architecture designed, ready for implementation"
        passes: "Design docs, API specs, requirements"
        weight: 0.9
      - agent: test-automator
        when: "Implementation complete"
        passes: "Code, test requirements"
        weight: 0.8
```

### Domain Definition

```yaml
domain:
  id: web-backend
  name: "Web Backend Development"
  description: "Server-side web development"

  workflow:
    - step: 1
      role: architect
      purpose: "Design API and data models"
      select:
        prefer_agent: backend-architect
        require_skills:
          - api-design
    - step: 2
      role: implementer
      purpose: "Implement features"
      select:
        prefer_agent: python-pro
```

## Directory Structure

```
.c4/agents/                    # or custom base_dir
├── skills/
│   ├── python-coding.yaml
│   ├── debugging.yaml
│   └── ...
├── personas/
│   ├── backend-architect.yaml
│   ├── debugger.yaml
│   └── ...
├── domains/
│   ├── web-backend.yaml
│   ├── web-frontend.yaml
│   └── ...
└── rules/
    └── task-overrides.yaml
```

## API Reference

### Core Classes

| Class | Description |
|-------|-------------|
| `AgentGraph` | NetworkX-based graph for agent routing |
| `AgentGraphLoader` | Loads YAML definitions with schema validation |
| `GraphRouter` | Routes tasks to agents |
| `SkillMatcher` | Matches tasks to skills and agents |
| `TaskContext` | Task information for routing |
| `RoutingResult` | Routing decision with metadata |
| `AgentMatch` | Agent match result with score |

### Node Types

| Type | Description |
|------|-------------|
| `NodeType.SKILL` | Skill nodes |
| `NodeType.AGENT` | Agent nodes |
| `NodeType.DOMAIN` | Domain nodes |

### Edge Types

| Type | Description |
|------|-------------|
| `EdgeType.HAS_SKILL` | Agent → Skill |
| `EdgeType.HANDS_OFF_TO` | Agent → Agent |
| `EdgeType.PREFERS` | Domain → Agent |
| `EdgeType.TRIGGERS` | Skill → Agent |
| `EdgeType.REQUIRES` | Skill → Skill |
| `EdgeType.COMPLEMENTS` | Skill → Skill |

### Exceptions

| Exception | Description |
|-----------|-------------|
| `LoaderError` | Base exception for loader errors |
| `FileNotFoundError` | File or directory not found |
| `SchemaValidationError` | YAML data fails schema validation |
| `YAMLParseError` | Invalid YAML syntax |
| `ModelValidationError` | Pydantic model validation failed |

## Routing Priority

1. **Task Type Override**: If task has `task_type` matching an override (e.g., "debug" → "debugger")
2. **Skill-Based Routing**: If SkillMatcher finds matching skills → best agent by score
3. **Domain-Based Routing**: Fall back to domain's preferred agent chain

## Examples

See the `examples/` directory for complete YAML examples:

- `examples/skills/` - Skill definitions
- `examples/personas/` - Agent definitions
- `examples/domains/` - Domain definitions
- `examples/rules/` - Rule definitions (overrides, chain extensions)
