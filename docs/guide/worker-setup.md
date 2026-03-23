# Remote Worker Setup

> See also: [Internal worker guide](https://github.com/PlayIdea-Lab/cq/blob/main/docs/guide/worker.md) for comprehensive CLI/API/troubleshooting reference.

Connect a GPU server (or any machine) to CQ as a remote worker.

## Quick Start (v1.27+)

Connect a GPU server in 2 commands:

```sh
# On the GPU server:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq    # login + service start + relay connect (all automatic)
```

That's it. The server is now:
- **Relay-connected** — accessible from any machine via `https://cq-relay.fly.dev/w/{hostname}/mcp`
- **OS service** — auto-starts on boot, auto-restarts on crash
- **Hub-ready** — accepts jobs from `cq hub submit` (full tier)

Verify with `cq doctor` and `cq relay status`.

---

## How it works

```
Your laptop          Supabase (cloud)       GPU server
────────────         ────────────────       ──────────
cq hub submit   ──►  jobs table        ◄──  cq hub worker start
(uploads code +      LISTEN/NOTIFY          (pulls job via pgx,
 posts job)          (NAT-safe)             runs it,
                                            uploads results)
```

1. You run `cq hub submit` on your laptop — CQ snapshots the current folder to Drive CAS and inserts a job row in Supabase.
2. Workers listen via `pgx LISTEN/NOTIFY` (outbound TCP to Supabase port 5432 — NAT-safe, no inbound port needed).
3. The worker that picks up the job downloads the exact snapshot, runs it, and pushes output artifacts back.
4. Workers are **stateless** — no project config needed on the server. The job payload carries everything.

---

## Step 1 — Install CQ on the server

SSH into your GPU server and run:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

Open a new shell or source your RC file to activate PATH:

```sh
source ~/.bashrc   # or ~/.zshrc
cq --version
```

## Step 2 — Log in

```sh
cq auth login
```

A device code appears. Open the URL on any browser (your laptop is fine), enter the code, and approve. The server will confirm:

```
✓ Logged in as user@example.com
```

## Step 3 — Start the worker

```sh
cq hub worker start
```

The worker connects to Supabase and waits for jobs:

```
cq: registered worker  id=worker-abc123  host=gpu-server-1
cq: listening for jobs via NOTIFY...
```

That's it. The server is now a stateless worker — no project setup, no `cq project use`, no local data needed.

No Hub URL to configure. No `C5_HUB_URL` or `C5_API_KEY` environment variables. The worker uses your existing `cloud.url` from `~/.c4/config.yaml`.

---

## Run as a persistent service

`cq` automatically installs an OS service (LaunchAgent on macOS, systemd on Linux):

```sh
cq              # auto-installs and starts the service
cq serve status # check service status
cq stop         # stop the service
```

Logs:
- **macOS**: `~/Library/Logs/cq-serve.{out,err}.log`
- **Linux**: `~/.local/state/cq/cq-serve.{out,err}.log`

::: details Manual systemd setup (legacy)
If you need a custom unit file for Hub worker specifically:

```sh
cat > ~/.config/systemd/user/cq-worker.service << 'EOF'
[Unit]
Description=CQ Hub Worker
After=network-online.target

[Service]
ExecStart=%h/.local/bin/cq hub worker start
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now cq-worker
journalctl --user -u cq-worker -f
```
:::

---

## Version gate (automatic upgrades)

If the Hub operator sets a minimum version requirement, workers below that version receive an `upgrade` control message instead of a job. The worker automatically runs `cq upgrade` and restarts — no manual intervention needed.

Workers built without version info (`version="unknown"`) bypass the gate to prevent upgrade loops.

---

## What happens when a job arrives

1. **Snapshot pull** — Downloads the code snapshot (Drive CAS, exact version hash)
2. **Parse `cq.yaml`** — Reads `run`, `artifacts.input`, `artifacts.output`
3. **Input artifacts** — Pulls declared datasets/files from Drive
4. **Run** — Executes the command with `C4_PROJECT_ID` injected
5. **Output push** — Uploads declared output paths back to Drive

The worker never needs to know the project name or credentials ahead of time — everything arrives in the job payload.

---

## Maintenance

### Zombie Worker GC *(v0.91.0+)*

Workers offline for 24+ hours are automatically cleaned up by the Hub. Manual pruning:

```sh
cq hub workers prune              # Remove zombie workers
cq hub workers prune --dry-run    # Preview only
cq hub workers                    # Active workers (default)
cq hub workers --all              # Include offline/pruned
```

### Capability Fallback Chain *(v0.91.0+)*

When executing a job, the worker resolves the command through a 3-step fallback:
1. `capabilities/<name>` file on disk
2. `caps.yaml` `command` field
3. `C5_PARAMS.command` from the job payload

With `command:` defined in `caps.yaml`, no capability file is needed.

## Worker Affinity *(v1.5.0+)*

Workers automatically learn which projects they're good at. The more a worker succeeds at a project's jobs, the higher its affinity score — and future jobs for that project are routed there first.

### How it works

```
1st run:  HMR job → any idle worker (no history)
2nd run:  HMR job → same worker preferred (affinity: hmr 1✓)
10th run: HMR job → strongly preferred (affinity: hmr 10✓)
```

Scoring formula: `project_match×10 + tag_overlap×3 + recency×2 + success_rate×5`

### Viewing affinity

```sh
cq hub workers
  ID               STATUS   AFFINITY              TAGS
  gpu-server       idle     hmr(10✓) cq(2✓)       [gpu, a100]
  build-server     idle     cq(15✓)               [cpu, linux]
  nas              idle     (none)                [storage]
```

### Tag-based routing

Use tags to ensure jobs go to the right hardware:

```sh
# GPU-required job → only workers with 'gpu' tag
cq hub submit --project hmr --tags gpu "train backbone"

# CPU-only job → any worker
cq hub submit --project cq "go build test"
```

Tags filter candidates first, then affinity ranks them.

### Manual override

Pin a job to a specific worker:

```sh
cq hub submit --worker gpu-server "urgent training"
```

This bypasses affinity scoring entirely.

---

## Submitting jobs from your laptop

See [Distributed Experiments](/examples/distributed-experiments) for the full submit workflow using `cq hub submit` and `cq.yaml`.
