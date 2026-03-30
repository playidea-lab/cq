# C5 Hub Worker Guide

Connect any machine — GPU server, cloud VM, or local workstation — to the C5 Hub as a stateless GPU job worker. This is how CQ delivers GPU Anywhere: submit from your laptop, execute on any GPU, anywhere.

## Overview

```
Your laptop          C5 Hub (cloud)        Worker (GPU/CPU)
────────────         ─────────────         ────────────────
cq hub submit   ──►  job queue        ◄──  cq hub worker start
(uploads code +      (distributes)         (pulls job,
 posts job)                                 runs it,
                                            uploads results)
```

Workers are **stateless** — no project config needed on the server. The job payload carries everything: code snapshot, project ID, environment variables, and artifact declarations.

**1-Tier Docker Model**: The worker runs on the host. When a capability has `runtime.image` set, the worker executes the job via `docker run` (no Docker-in-Docker required). Without `runtime.image`, jobs run directly on the host.

## Quick Start

### Cloud-connected worker (recommended)

```sh
# 1. Install cq
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash

# 2. Authenticate (Supabase-backed cloud account)
cq auth login

# 3. Start all-in-one serve (Hub worker + MCP + relay + cron + pg_notify)
cq serve
```

`cq serve` is the recommended entry point. It starts everything in one process. See [cq serve: All-in-One Mode](#cq-serve-all-in-one-mode) for details.

### Self-hosted Hub worker

```sh
# 1. Install cq
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash

# 2. Install worker service (Docker + NVIDIA toolkit + systemd — all automatic)
cq hub worker install

# 3. Verify
systemctl status cq-worker
```

That's it. The worker is now polling the Hub for jobs.

## cq serve: All-in-One Mode

`cq serve` is the unified entry point that replaces running individual components separately.

| Component | `cq serve` | `cq hub worker start` (legacy) |
|-----------|-----------|-------------------------------|
| Hub worker (job polling) | Yes | Yes |
| MCP server | Yes | No |
| Relay (NAT traversal) | Yes | No |
| Cron scheduler | Yes | No |
| pg_notify real-time | Yes | No |

**`cq hub worker start` is legacy** — `cq serve` is a strict superset and should be used instead.

### Customer onboarding flow

```sh
cq update           # Update to latest binary
cq auth login       # Authenticate with cloud account
cq serve            # Start everything
```

### pg_notify real-time mode

When `cloud.direct_url` is configured, `cq serve` activates PostgreSQL `LISTEN 'new_job'` for instant job delivery instead of polling.

```yaml
# ~/.c4/config.yaml
cloud:
  direct_url: "postgresql://..."  # Direct Supabase connection string
```

- **With `direct_url`**: LISTEN/NOTIFY — sub-second job delivery.
- **Without `direct_url`**: 30-second polling fallback (original behavior).

---

## Job Routing

Route jobs to specific workers or machines with capabilities.

### Target a specific worker

```sh
# CLI
cq hub submit --target worker-id python train.py

# MCP
c4_job_submit(command="python train.py", target_worker="worker-abc123")
```

### Route by capability

```sh
# CLI — any worker with 'cuda' capability
cq hub submit --capability cuda python train.py

# MCP
c4_job_submit(command="python train.py", capability="cuda")
```

### Route by tags

```sh
# CLI — worker must have both 'gpu' and 'a100' tags
cq hub submit --tags gpu,a100 python train.py

# MCP
c4_job_submit(command="python train.py", required_tags=["gpu", "a100"])
```

Tags are declared in the worker's `caps.yaml`:

```yaml
tags:
  - gpu
  - a100
  - datacenter-us
```

---

## Prerequisites

| Item | Required | Notes |
|------|----------|-------|
| `cq` binary | Yes | `curl -fsSL .../install.sh \| bash` |
| Hub URL | Yes | `C5_HUB_URL` env or `cq config set hub.url` |
| Auth (JWT or API key) | Yes | `cq auth login` or `C5_API_KEY` env |
| Docker | Recommended | `cq hub worker install` auto-installs |
| NVIDIA Container Toolkit | Optional | For GPU jobs; auto-installed if GPU detected |
| `nvidia-smi` | Optional | CPU-only fallback if absent |

## Installation

### Method 1: `cq hub worker install` (recommended)

Installs Docker, NVIDIA toolkit (if GPU present), and registers a systemd service — all in one command.

```sh
cq hub worker install
```

Options:
```sh
cq hub worker install --dry-run   # Preview only (prints service file, no install)
cq hub worker install --user      # User-level systemd unit (Linux only)
```

After install: `systemctl status cq-worker`

### Method 2: Manual (`cq hub worker start`)

For development or when you want manual control:

```sh
# Configure credentials
cq hub worker init                # Interactive: Hub URL + API key → ~/.c5/config.yaml
# Or non-interactive:
cq hub worker init --non-interactive --hub-url "$C5_HUB_URL" --api-key "$C5_API_KEY"

# Start worker (foreground)
cq hub worker start
```

You can also start the `c5` binary directly:

```sh
c5 worker --capabilities caps.yaml
```

### Method 3: Docker Compose

```sh
# Download worker files
curl -sSL https://github.com/PlayIdea-Lab/cq/releases/latest/download/gpu-worker.tar.gz | tar xz

# Set credentials
cat > .env <<EOF
C5_HUB_URL=https://<hub-host>:8585
C5_API_KEY=sk-worker-<your-key>
EOF

# Start
docker compose up -d
docker compose logs -f
```

The `docker-compose.yml` and `Dockerfile` are in `docs/gpu-worker/`.

## Authentication

### JWT Authentication (recommended)

No API key needed — uses browser-based login with automatic token refresh.

```sh
cq auth login --device    # Opens device code flow
cq hub worker start       # JWT auto-detected from ~/.c4/session.json
```

When `cq hub worker start` finds no API key, it automatically injects the cloud JWT from `~/.c4/session.json`. Hub accepts HS256 JWT tokens when `C5_JWT_SECRET` (or `SUPABASE_JWT_SECRET`) is configured.

### Scoped API Keys

Key prefixes restrict access scope:

| Prefix | Scope | Use Case |
|--------|-------|----------|
| `sk-user-*` | Submit + query jobs | Claude Code → Hub connection |
| `sk-worker-*` | Poll + complete jobs | Worker authentication |
| (none) | Full access | Admin, backward compat |

```sh
# Worker key (can only poll/report, cannot submit jobs)
export C5_API_KEY="sk-worker-<your-key>"

# User key (can only submit/query, cannot poll)
export C5_API_KEY="sk-user-<your-key>"
```

### Environment Variables

| Variable | Description | Priority |
|----------|-------------|----------|
| `C5_HUB_URL` | Hub server URL | Highest |
| `C5_API_KEY` | API key | Highest |
| `C5_JWT_SECRET` | Hub-side JWT secret | Hub config |
| Config `hub.url` | `.c4/config.yaml` | Medium |
| Built-in URL | ldflags at build time | Lowest |

## Configuration

### caps.yaml (capability definition)

Declare what your worker can do:

```yaml
capabilities:
  - name: gpu_train
    description: "Run GPU training"
    command: scripts/gpu-train.sh        # Executed when job arrives
    input_schema:
      type: object
      properties:
        script:
          type: string
          description: "Python script path"
        args:
          type: string
          description: "Additional arguments"

  - name: gpu_status
    description: "Query GPU status"
    command: scripts/gpu-status.sh
    input_schema:
      type: object
      properties: {}
```

Start with capabilities:
```sh
cq hub worker init       # Enter caps.yaml path when prompted
cq hub worker start
# Or directly:
c5 worker --capabilities caps.yaml
```

### cq.yaml (job submission side)

Declare artifacts and run command in the experiment directory:

```yaml
run: python train.py
artifacts:
  input:
    - name: mnist_mini        # Drive dataset name
      local_path: data/mnist
  output:
    - name: model-checkpoint
      local_path: checkpoints/
```

## Worker Lifecycle

```
register → heartbeat → poll → execute → complete → poll → ...
```

1. **Register** — Worker connects to Hub, reports capabilities and version
2. **Heartbeat** — Periodic keepalive with uptime and status
3. **Poll** — Pull next available job matching worker capabilities
4. **Execute** — Run job (see execution details below)
5. **Complete** — Report result back to Hub

### Job Execution Pipeline

When a job arrives:

1. **Snapshot pull** — Downloads code snapshot from Drive CAS (exact version hash)
2. **Parse `cq.yaml`** — Reads `run`, `artifacts.input`, `artifacts.output`
3. **Input artifacts** — Pulls declared datasets/files from Drive
4. **Run** — Executes the command with environment variables injected
5. **Output push** — Uploads declared output paths back to Drive

### Environment Variables Injected Per Job

| Variable | Description |
|----------|-------------|
| `C4_PROJECT_ID` | Project identifier (from job payload) |
| `C5_CAPABILITY` | Capability name being executed |
| `C5_PARAMS` | Job parameters (JSON) |
| `C5_RESULT_FILE` | Path to write result JSON |

Workers never need to know the project name or credentials ahead of time.

## Capability System

### Auto-detected Capabilities

If no `caps.yaml` is provided, the worker runs jobs using the command from the job payload.

### Custom Capabilities

Define capabilities in `caps.yaml` (see [Configuration](#capsyaml-capability-definition) above).

### Command Resolution (3-step fallback) *(v0.91.0+)*

When executing a job, the worker resolves the command through:

1. **`capabilities/<name>` file** — Executable file in the worker's capabilities directory
2. **`caps.yaml` `command` field** — Command defined in the capability YAML
3. **`C5_PARAMS.command`** — `run_command` from the job payload

With `command:` defined in `caps.yaml`, no capability file is needed on disk.

## Running as a Service

### systemd (Linux) — via `cq hub worker install`

Automatically created by `cq hub worker install`. Manual setup:

```ini
# /etc/systemd/system/cq-worker.service
[Unit]
Description=CQ Hub Worker
After=network.target docker.service
Wants=docker.service

[Service]
User=ubuntu
SupplementaryGroups=docker
WorkingDirectory=/opt/gpu-worker
ExecStart=/usr/local/bin/cq hub worker start
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl enable --now cq-worker
sudo journalctl -fu cq-worker
```

> **Note**: `cq hub worker start` uses JWT or `~/.c5/config.yaml` API key automatically. For systemd, run `cq auth login --device` as the service user first.

### systemd (user-level)

```sh
cat > ~/.config/systemd/user/cq-worker.service << 'EOF'
[Unit]
Description=CQ Hub Worker
After=network-online.target

[Service]
ExecStart=%h/.local/bin/cq hub worker start
Environment=C5_API_KEY=YOUR_API_KEY
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now cq-worker
journalctl --user -u cq-worker -f
```

### launchd (macOS)

Use `cq hub worker install` which generates the appropriate launchd plist.

### Docker

See [Method 3: Docker Compose](#method-3-docker-compose) above.

## Submitting Jobs

### CLI

```sh
# Submit current folder as a job
cq hub submit [--run "python train.py"] [--project myproj]
# --run omitted → uses cq.yaml `run` field
```

CQ snapshots the current folder to Drive CAS (content-addressable storage, automatic dedup) and posts a job to the Hub. No Git required.

### MCP (from Claude Code)

```sh
# Use c4_hub_submit MCP tool
c4_hub_submit(command="python train.py")
```

### Claude Code `.mcp.json` Connection

```json
{
  "mcpServers": {
    "gpu-hub": {
      "type": "http",
      "url": "http://<hub-host>:8585/v1/mcp",
      "headers": { "X-API-Key": "sk-user-<your-key>" }
    }
  }
}
```

### Project Auto-detection

Project is resolved in order: `C4_PROJECT_ID` env → config `active_project_id` → directory name matching → single-project auto-select.

## Monitoring

```sh
cq hub workers              # List active workers (default: active-only)
cq hub workers --all        # Include offline/pruned workers
cq hub status <job_id>      # Job status
cq hub list                 # List jobs
cq hub watch <job_id>       # Watch job progress
cq hub log <job_id>         # Job logs
cq hub summary              # Hub summary stats
```

## Maintenance

### Zombie Worker GC *(v0.91.0+)*

Workers offline for 24+ hours are automatically moved to `worker_history` and deleted. This runs every hour during the Hub's lease expiry loop.

Manual pruning:
```sh
cq hub workers prune              # Remove zombie workers
cq hub workers prune --dry-run    # Preview what would be pruned
```

### Version Gate

When `C5_MIN_VERSION` is set on the Hub, workers below that version receive `control: {action:"upgrade"}` instead of a job. The worker automatically runs `cq upgrade` and restarts.

Workers with `version=""` or `"unknown"` bypass the gate to prevent upgrade loops.

```sh
# If you see "upgrade" control messages:
cq upgrade              # Update cq binary
cq hub worker start     # Restart worker
```

## CLI Reference

### `cq hub worker` subcommands

| Command | Description |
|---------|-------------|
| `cq hub worker install` | Install service (Docker + systemd) |
| `cq hub worker install --dry-run` | Preview service file |
| `cq hub worker install --user` | User-level systemd unit |
| `cq hub worker init` | Configure credentials interactively |
| `cq hub worker init --non-interactive` | Non-interactive setup |
| `cq hub worker start` | Start worker (foreground) |

### `cq hub` subcommands

| Command | Description |
|---------|-------------|
| `cq hub submit` | Submit job from current directory |
| `cq hub status <id>` | Job status |
| `cq hub list` | List jobs |
| `cq hub watch <id>` | Watch job progress |
| `cq hub log <id>` | Job logs |
| `cq hub workers` | List workers |
| `cq hub workers prune` | Remove zombie workers |
| `cq hub summary` | Hub summary |
| `cq hub metrics` | Hub metrics |

### `c5` direct commands

| Command | Description |
|---------|-------------|
| `c5 worker` | Start worker directly |
| `c5 worker --capabilities <file>` | Start with capability YAML |
| `c5 serve --port <port>` | Start Hub server |

## API Reference

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/health` | GET | Health check |
| `/v1/mcp` | POST | MCP Streamable HTTP (JSON-RPC 2.0) |
| `/v1/capabilities` | GET | List registered capabilities |
| `/v1/capabilities/invoke` | POST | Create capability job |
| `/v1/workers/register` | POST | Register worker |
| `/v1/workers/heartbeat` | POST | Worker heartbeat |
| `/v1/jobs/poll` | POST | Poll for next job |
| `/v1/jobs/complete` | POST | Report job completion |
| `/v1/jobs/submit` | POST | Submit new job |
| `/v1/jobs/{id}` | GET | Get job status |
| `/v1/jobs` | GET | List jobs |

## Troubleshooting

### nvidia-smi not found

The worker runs in CPU-only mode automatically. No action required.
```
[worker] nvidia-smi not found — starting in CPU-only mode
```

### API key / authentication error

```sh
# Re-initialize with correct credentials
cq hub worker init --non-interactive --hub-url "$C5_HUB_URL" --api-key "<correct-key>"

# Or use JWT instead
cq auth login --device
cq hub worker start
```

### Worker rejected by Hub (version gate)

```sh
cq upgrade            # Update cq binary
cq hub worker start   # Restart
```

The Hub's `C5_MIN_VERSION` env controls the minimum allowed version.

### Worker shows offline

1. Ensure `cq hub worker start` is running: `ps aux | grep cq`
2. Check Hub reachability: `curl -s "$C5_HUB_URL/v1/health"`
3. Check firewall — worker initiates outbound connections to the Hub
4. Check logs: `journalctl -fu cq-worker`

### Job stuck / timeout

- Default smoke test timeout is 120s. Increase for slow GPUs.
- Inspect job logs: `cq hub log <job_id>`
- Check worker logs for errors during execution

### API key stored in plaintext

`cq hub worker init` stores the API key in `~/.c5/config.yaml` (permissions `0600`). For higher security, use JWT authentication (`cq auth login`) or integrate with `cq secret set`.

### `--non-interactive` required in CI

When passing `--hub-url` and `--api-key` flags, always include `--non-interactive` to prevent stdin hang in CI pipelines:

```sh
cq hub worker init --non-interactive --hub-url "$C5_HUB_URL" --api-key "$C5_API_KEY"
```
