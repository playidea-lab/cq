# C4D - AI Project Orchestration Daemon

C4 (Codex–Claude–Completion Control) is an AI project orchestration system that enables AI agents to execute projects from planning through completion without interruption.

## Installation

```bash
uv sync
```

## Usage

### Initialize a project

```bash
c4 init
```

### Start execution

```bash
c4 run
```

### Check status

```bash
c4 status
```

## Architecture

- **c4d**: MCP Server daemon for state management
- **Worker**: Claude Code with Ralph Loop for task execution
- **Supervisor**: Headless Claude for checkpoint review

## Documentation

See `docs/` for detailed documentation.

---

## C4 Cloud (Coming Soon)

C4 Cloud는 C4의 호스팅 SaaS 버전입니다.

### Why Cloud?

| Feature | C4 Local (CLI) | C4 Cloud |
|---------|---------------|----------|
| Setup | `pip install c4d` + API key | Sign up only |
| Interface | Terminal | Web dashboard |
| Workers | Manual terminal tabs | Slider control |
| Results | Local files | Auto GitHub push/PR |
| Cost | Your API keys | Subscription |

### Key Features

- **No API Key Setup**: Start immediately, we handle LLM billing
- **Web Dashboard**: Monitor projects from anywhere (even mobile)
- **Worker Scaling**: Scale 1→20 workers with a slider
- **GitHub Integration**: Auto push results, create PRs
- **Team Collaboration**: Share projects, audit logs

### Pricing (Planned)

| Tier | Price | Included | Workers |
|------|-------|----------|---------|
| Free | $0 | $10/mo credits | 1 |
| Pro | $30/mo | $50/mo credits | 5 |
| Team | $100/mo | $200/mo credits | 20 |
| Enterprise | Contact | Custom | Unlimited |

See [docs/cloud/](./docs/cloud/) for detailed Cloud architecture and PRD.

---

## License

This project is licensed under the **Business Source License 1.1** (BSL).

- **Free for**: Personal use, evaluation, non-commercial projects
- **Requires license for**: Commercial use, production deployment in businesses

See [LICENSE](./LICENSE) for full terms.
