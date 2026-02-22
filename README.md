# CQ — AI Project Orchestration Engine

**CQ** is a project management engine for Claude Code.
It automates the full development lifecycle — planning, implementation, review, and delivery — through a structured workflow powered by C4 Engine.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Opens a new terminal and you're ready:

```sh
cq --help
```

## Tiers

Choose the tier that fits your setup:

| Tier | Description | Use when |
|------|-------------|----------|
| `solo` | Local only, no external deps | Personal / offline |
| `connected` | + Supabase, LLM Gateway, EventBus | Team / cloud sync |
| `full` | + Hub, Drive, CDP, GPU, C1 Messenger | Full production |

```sh
# Install a specific tier (default: solo)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

## Quick Start

```sh
# 1. Check environment
cq doctor

# 2. Initialize C4 in your project (generates .mcp.json + CLAUDE.md)
cd your-project
cq claude   # for Claude Code
cq cursor   # for Cursor

# 3. Open Claude Code — C4 MCP tools are now available
```

## How it looks

### Scenario 1 — Building a new feature

> **You:** "JWT 인증 추가해줘. Google이랑 GitHub 로그인"

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery
    Q: Session store — Redis or DB?          → DB (stateless preferred)
    Q: Token expiry — access/refresh split?  → Yes, 15min / 7day
    Q: Existing user model?                  → users table exists

  ● Design
    Provider abstraction → Google → GitHub → Session middleware
    JWT: RS256, stored in httpOnly cookie

  ● Tasks created
    T-001  OAuth provider interface
    T-002  Google provider
    T-003  GitHub provider
    T-004  JWT middleware + session store
    T-005  Integration tests
```

> **You:** "ㄱㄱ"

```
/c4-run

  ◆ T-001  [worker-a] worktree: c4/w-T-001-0  ████████░░  implementing...
  ◆ T-002  [worker-b] worktree: c4/w-T-002-0  ████░░░░░░  implementing...
  ◆ T-003  [worker-c] worktree: c4/w-T-003-0  ██░░░░░░░░  implementing...
  ◆ T-004  waiting on T-001

  ✓ T-001  submitted (sha: a3f8c21)  →  R-001 review queued
  ✓ T-002  submitted (sha: 7b2e94d)  →  R-002 review queued
  ...
  ✓ All tasks complete → auto-polish → /c4-finish → done
```

---

### Scenario 2 — Quick bug fix

> **You:** "모바일에서 로그인 버튼 클릭이 안 돼"

```
/c4-quick "fix login button not responding on mobile"

  ● Task T-011-0 created
    DoD: touch event handler added, tested on viewport <768px

  ◆ [worker] implementing fix...
  ✓ submitted  →  review passed  →  done

  Changed: src/components/LoginButton.tsx (+3 -1)
```

---

### Scenario 3 — Checking status mid-flight

> **You:** "지금 어디까지 됐어?"

```
/c4-status

  Phase: EXECUTE  ████████████░░░░  75%

  ✓ T-001  OAuth interface      [merged]
  ✓ T-002  Google provider      [merged]
  ▶ T-003  GitHub provider      [in review]
  ◷ T-004  JWT middleware        [pending T-003]
  ◷ T-005  Integration tests    [pending T-004]

  Workers: 1 active  |  Queue: 2 pending  |  Knowledge: 8 records
```

---

### Scenario 4 — Distributed experiments across machines

> **You:** "backbone 3개 비교 실험 돌려야 해. ResNet / EfficientNet / ViT"

```
/c4-plan "backbone ablation: ResNet50 vs EfficientNet-B4 vs ViT-B/16"

  ● Tasks created
    T-020  train ResNet50       (GPU: 1x A100, est. 4h)
    T-021  train EfficientNet   (GPU: 1x A100, est. 3h)
    T-022  train ViT-B/16       (GPU: 1x A100, est. 6h)
    T-023  compare results + report
```

각 머신에서 `/c4-standby`로 워커 등록:

```
# machine-1 (Claude Code session)
/c4-standby

  ● Registered as worker  [id: worker-m1]
  ◷ Waiting for jobs from C5 Hub...
  ✓ Claimed T-020  →  training ResNet50...

# machine-2
/c4-standby
  ✓ Claimed T-021  →  training EfficientNet...

# machine-3
/c4-standby
  ✓ Claimed T-022  →  training ViT-B/16...
```

> **You:** "결과 어때?"

```
/c4-status

  ✓ T-020  ResNet50       MPJPE: 48.3mm  PA-MPJPE: 34.1mm  [worker-m1]
  ✓ T-021  EfficientNet   MPJPE: 44.7mm  PA-MPJPE: 31.8mm  [worker-m2]  ← best
  ▶ T-022  ViT-B/16       running 3h 42m / ~6h               [worker-m3]
  ◷ T-023  waiting on T-020, T-021, T-022

  Knowledge: 2 new experiment records saved
```

T-022 완료 후 비교 리포트 자동 생성:

```
  ✓ T-023  Comparison report

  | Backbone      | MPJPE  | PA-MPJPE | Params | Throughput |
  |---------------|--------|----------|--------|------------|
  | ResNet50      | 48.3mm | 34.1mm   | 25.6M  | 142 img/s  |
  | EfficientNet  | 44.7mm | 31.8mm   | 19.3M  | 98 img/s   |
  | ViT-B/16      | 41.2mm | 29.4mm   | 86.6M  | 47 img/s   |

  Recommendation: EfficientNet — best accuracy/efficiency ratio
  Knowledge recorded → injected into next experiment planning
```

---

## Workflow

```
/c4-plan "feature description"   → discovery + design + tasks
/c4-run                          → spawn workers, implement in parallel
/c4-finish                       → polish · build · test · commit
/c4-status                       → check progress at any time
```

## Config

`solo` tier works out of the box — no config needed.

For `connected` / `full` tiers, place the config provided by your team at `~/.c4/config.yaml`.

## Update

Re-run the install command to update to the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Requirements

- macOS Apple Silicon (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) installed
- `curl` available

## License

[MIT + Commons Clause](LICENSE) — free to use and modify, commercial resale prohibited.
