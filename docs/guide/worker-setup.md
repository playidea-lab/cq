# Worker Setup

Connect a GPU server, cloud VM, or local workstation to the CQ Hub as a job worker.

## Architecture

```
Your laptop              CQ Hub (cloud)          Worker (GPU/CPU)
───────────              ──────────────          ────────────────
cq hub submit  ────────► job queue        ◄────  cq serve
(code snapshot +         (distributes)           (polls queue,
 job spec)                                        runs job,
                                                  uploads results)
```

Workers are **stateless** — no project config needed on the worker machine. The job carries everything: code snapshot, environment variables, and artifact declarations.

## 3-Step Quick Start

### Step 1: Install CQ on the Worker Machine

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

This works on Linux (x86_64, ARM64), macOS, and Windows/WSL2. Docker and NVIDIA Container Toolkit are detected and configured automatically if present.

### Step 2: Authenticate

```sh
cq auth login    # GitHub OAuth — use the same account as your laptop
```

Or, for headless machines (no browser):

```sh
cq auth login --device    # Device code flow — enter code on another device
```

### Step 3: Start

```sh
cq serve    # Starts Hub worker + MCP + relay + cron in one process
```

The worker is now connected. Jobs submitted from your laptop arrive automatically.

## What `cq serve` Starts

`cq serve` is the all-in-one entry point. It replaces running individual components separately.

| Component | Included |
|-----------|----------|
| Hub worker (job polling) | Yes |
| MCP server | Yes |
| Relay (NAT traversal) | Yes |
| Cron scheduler | Yes |
| pg_notify real-time | Yes (when `cloud.direct_url` is set) |

## Run as a Service

### Linux (systemd) — recommended

```sh
cq serve install    # Installs systemd service, Docker, NVIDIA toolkit if GPU found
systemctl status cq-worker
```

Check logs:

```sh
journalctl -fu cq-worker
```

Manual systemd unit (if you prefer):

```ini
[Unit]
Description=CQ Hub Worker
After=network.target docker.service

[Service]
User=ubuntu
SupplementaryGroups=docker
WorkingDirectory=/opt/gpu-worker
ExecStart=/usr/local/bin/cq serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### macOS (launchd)

```sh
cq serve install    # Generates and loads launchd plist
```

### Docker Compose

```sh
curl -sSL https://github.com/PlayIdea-Lab/cq/releases/latest/download/gpu-worker.tar.gz | tar xz

cat > .env <<EOF
C5_HUB_URL=https://<hub-host>:8585
C5_API_KEY=sk-worker-<your-key>
EOF

docker compose up -d
docker compose logs -f
```

## Real-Time Job Delivery

By default, workers poll for jobs every 30 seconds. For sub-second delivery, configure a direct database connection:

```yaml
# ~/.c4/config.yaml
cloud:
  direct_url: "postgresql://..."    # Direct Supabase connection string
```

With `direct_url`, the worker uses PostgreSQL `LISTEN 'new_job'` — jobs arrive instantly.

## Submitting Jobs

From your laptop, in Claude Code:

```sh
# MCP tool
c4_hub_submit(command="python train.py")
```

Or from the terminal:

```sh
cq hub submit --run "python train.py"
```

CQ snapshots the current directory to Drive (content-addressable, automatic dedup) and posts the job to the Hub. No Git required.

## GPU Detection

Workers automatically detect GPU capabilities:

- If `nvidia-smi` is found, the worker registers as GPU-capable
- Jobs with `requires_gpu: true` are only routed to GPU workers
- If `nvidia-smi` is not found, the worker starts in CPU-only mode (no action needed)

## Routing Jobs to Specific Workers

### By worker ID

```sh
cq hub submit --target worker-abc123 python train.py
```

### By capability

```sh
cq hub submit --capability cuda python train.py
```

### By tags

```sh
cq hub submit --tags gpu,a100 python train.py
```

Declare tags in `caps.yaml` on the worker:

```yaml
tags:
  - gpu
  - a100
  - datacenter-us
```

## Monitoring

```sh
cq hub workers              # Active workers
cq hub workers --all        # Include offline workers
cq hub list                 # Recent jobs
cq hub status <job_id>      # Job status
cq hub watch <job_id>       # Live job output
cq hub log <job_id>         # Job logs
cq hub summary              # Hub stats
```

## Maintenance

### Remove zombie workers

Workers offline for 24+ hours are pruned automatically. Manual cleanup:

```sh
cq hub workers prune              # Remove offline workers
cq hub workers prune --dry-run    # Preview
```

### Version gate

If the Hub requires a minimum worker version:

```sh
cq update               # Update binary
cq hub worker start     # Restart worker
```

## Authentication Reference

| Method | How |
|--------|-----|
| JWT (recommended) | `cq auth login` — auto-injected from `~/.c4/session.json` |
| API key | `export C5_API_KEY=sk-worker-<key>` |
| Device code | `cq auth login --device` — for headless machines |

Key prefixes:

| Prefix | Scope |
|--------|-------|
| `sk-worker-*` | Poll and complete jobs only |
| `sk-user-*` | Submit and query jobs only |
| (none) | Full access |

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `nvidia-smi not found` | Worker runs in CPU-only mode automatically — no action needed |
| Auth error | Re-run `cq auth login` or `cq auth login --device` |
| Worker shows offline | Run `ps aux | grep cq` and `curl -s "$C5_HUB_URL/v1/health"` |
| Job stuck | Check `cq hub log <job_id>` and worker logs |
| `--non-interactive` needed in CI | Pass `--non-interactive` flag to `cq hub worker init` |

## Next Steps

- [Remote Brain](remote-brain.md) — access CQ knowledge from other AI tools
- [Tiers](tiers.md) — understand the full tier feature set
