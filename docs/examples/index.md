# Examples

Real-world scenarios showing CQ in action. Each example shows the conversation between you and Claude Code, and what CQ does behind the scenes.

## Examples

| Example | Scenario | Skills used |
|---------|----------|-------------|
| [Feature Planning](/examples/feature-planning) | Build a new feature end-to-end | `/c4-plan` `/c4-run` `/c4-finish` |
| [Quick Bug Fix](/examples/quick-fix) | Fix a bug without the full plan flow | `/c4-quick` |
| [Distributed Experiments](/examples/distributed-experiments) | Run ML experiments across multiple machines | `/c4-plan` `/c4-standby` `/c4-status` |

## Pattern

Every example follows the same shape:

```
You say something natural
  ↓
CQ skill activates (via trigger keyword)
  ↓
Workers handle the work in isolated worktrees
  ↓
Results land in your repo with DoD verified
```

No commands to memorize. Just describe what you want.
