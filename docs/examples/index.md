# Examples

Real-world scenarios showing CQ in action. Each example shows the conversation between you and Claude Code, and what CQ does behind the scenes.

## Examples

| Example | Scenario | Tier | Skills used |
|---------|----------|:----:|-------------|
| [Your First Task](/examples/first-task) | Generate a CSV summary script from scratch | `solo` | `/pi` `/c4-run` |
| [Quick Bug Fix](/examples/quick-fix) | Fix a bug without the full plan flow | `solo` | `/c4-quick` |
| [Idea to Ship](/examples/idea-to-ship) | From vague idea to committed code, fully automatic | `solo` | `/pi` → auto pipeline |
| [Feature Planning](/examples/feature-planning) | Build a new feature end-to-end | `solo` | `/c4-plan` `/c4-run` `/c4-finish` |
| [Quality Gates](/examples/team-review) | See refine, polish, and review gates in action | `solo` | `/c4-plan` `/c4-run` |
| [Distributed Experiments](/examples/distributed-experiments) | Run ML experiments across multiple machines | `full` | `/c4-plan` `/c4-standby` `/c4-status` |

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
