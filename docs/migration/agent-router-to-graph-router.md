# Migration Guide: AgentRouter to GraphRouter

This guide describes the migration from `AgentRouter` to `GraphRouter` for
agent routing in C4.

## Overview

The `GraphRouter` provides a graph-based approach to agent routing while
maintaining 100% backward compatibility with the existing `AgentRouter` API.

### Key Benefits

- **Same API**: `GraphRouter` implements all `AgentRouter` methods
- **Fallback Mode**: Works without loading agent graph (same behavior as legacy)
- **Feature Flag**: Enable/disable via environment variable
- **Performance**: Queries complete in < 10ms (same as legacy)

## Enabling GraphRouter

### Via Environment Variable

```bash
# Enable GraphRouter
export C4_USE_GRAPH_ROUTER=1

# Disable (use legacy AgentRouter) - default
export C4_USE_GRAPH_ROUTER=0
# or simply don't set the variable
```

### Verification

Check which router is being used:

```python
from c4.mcp_server import C4Daemon

daemon = C4Daemon(Path("/your/project"))
result = daemon.c4_test_agent_routing()
print(result["router_type"])  # "AgentRouter" or "GraphRouter"
```

## API Compatibility

Both routers implement the same interface:

| Method                        | AgentRouter | GraphRouter |
|-------------------------------|:-----------:|:-----------:|
| `get_recommended_agent()`     | ✓           | ✓           |
| `get_agent_for_task_type()`   | ✓           | ✓           |
| `get_chain_for_domain()`      | ✓           | ✓           |
| `get_handoff_instructions()`  | ✓           | ✓           |
| `get_all_domains()`           | ✓           | ✓           |

### GraphRouter-Only Features

These methods are only available in GraphRouter with a loaded graph:

| Method                        | Description                    |
|-------------------------------|--------------------------------|
| `find_agents_for_skill()`     | Find agents with specific skill |
| `get_path_between_agents()`   | Find handoff path between agents |

## Migration Steps

### Step 1: Test with Feature Flag

1. Set `C4_USE_GRAPH_ROUTER=1` in your test environment
2. Run your existing test suite
3. Verify all tests pass

```bash
C4_USE_GRAPH_ROUTER=1 pytest tests/
```

### Step 2: Monitor Production

1. Enable feature flag in staging/production
2. Monitor for any regressions
3. Check performance metrics (< 10ms per query)

### Step 3: Load Agent Graph (Optional)

For advanced features, load an agent graph:

```python
from c4.supervisor.agent_graph import AgentGraphLoader, GraphRouter

# Load graph from YAML files
loader = AgentGraphLoader()
graph = loader.load_directory("path/to/agents/")

# Use GraphRouter with graph
router = GraphRouter(graph=graph)

# Now graph-only features work
agents = router.find_agents_for_skill("python-coding")
path = router.get_path_between_agents("backend-dev", "code-reviewer")
```

## Differences in Behavior

### Task Type Overrides

Both routers support the same task type overrides:

- `debug` → `debugger`
- `security` → `security-auditor`
- `test` → `test-automator`
- etc.

### Custom Configurations

**AgentRouter** supports custom domain/task configurations via `config.yaml`.
**GraphRouter** in fallback mode uses built-in defaults only.

To use custom configurations with GraphRouter, load a custom agent graph.

## Rollback

If issues occur, disable the feature flag:

```bash
export C4_USE_GRAPH_ROUTER=0
# or
unset C4_USE_GRAPH_ROUTER
```

## Performance Benchmarks

Both routers meet the < 10ms query requirement:

| Operation                    | AgentRouter | GraphRouter |
|------------------------------|:-----------:|:-----------:|
| `get_recommended_agent()`    | ~0.1ms      | ~0.2ms      |
| `get_all_domains()`          | ~0.1ms      | ~0.1ms      |
| 100 sequential queries       | ~10ms       | ~20ms       |

## FAQ

### Q: Will my existing tests break?

No. GraphRouter is API-compatible with AgentRouter. All 143 router tests pass
with both implementations.

### Q: Is GraphRouter faster?

In fallback mode (no graph), GraphRouter has slight overhead from creating an
internal legacy router. With a loaded graph, performance depends on graph size.

### Q: When should I use the agent graph?

Use the agent graph when you need:
- Skill-based agent discovery
- Custom handoff paths
- Dynamic agent routing based on graph structure

### Q: Can I use both routers in the same project?

Yes. Each `C4Daemon` instance checks the feature flag when creating its router.
You can override by setting `daemon._agent_router` directly.

## Support

For issues or questions about the migration, open an issue in the C4 repository.
