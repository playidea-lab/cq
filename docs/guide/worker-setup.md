# Remote Worker Setup

Connect a GPU server (or any machine) to CQ Hub as a stateless job worker.

::: info full tier required
Worker mode requires the `full` tier binary. [Install with `--tier full`](/guide/install#install-a-specific-tier).
:::

## How it works

```
Your laptop          C5 Hub (cloud)        GPU server
────────────         ─────────────         ──────────
cq hub submit   ──►  job queue        ◄──  c5 worker
(uploads code +      (distributes)         (pulls job,
 posts job)                                runs it,
                                           uploads results)
```

1. You run `cq hub submit` on your laptop — CQ snapshots the current folder to Drive CAS and posts a job.
2. Any connected worker pulls the job, downloads the exact snapshot, runs it, and pushes output artifacts back.
3. Workers are **stateless** — no project config needed on the server. The job payload carries everything.

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

## Step 3 — Configure the Hub URL

Set the Hub endpoint once:

```sh
cq config set hub.url https://your-hub.fly.dev
cq config set hub.api_key YOUR_API_KEY   # if your hub requires auth
```

Or export environment variables (useful in systemd / Docker):

```sh
export C5_HUB_URL=https://your-hub.fly.dev
export C5_API_KEY=YOUR_API_KEY
```

## Step 4 — Start the worker

```sh
c5 worker
```

The worker registers with Hub and waits for jobs:

```
c5: registered worker  id=worker-abc123  host=gpu-server-1
c5: waiting for jobs...
```

That's it. The server is now a stateless worker — no project setup, no `cq project use`, no local data needed.

---

## Run as a persistent service (systemd)

For production use, keep the worker alive after SSH disconnect:

```sh
cat > ~/.config/systemd/user/c5-worker.service << 'EOF'
[Unit]
Description=CQ C5 Worker
After=network-online.target

[Service]
ExecStart=%h/.local/bin/c5 worker
Environment=C5_HUB_URL=https://your-hub.fly.dev
Environment=C5_API_KEY=YOUR_API_KEY
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now c5-worker
systemctl --user status c5-worker
```

Check logs anytime:

```sh
journalctl --user -u c5-worker -f
```

---

## Version gate (automatic upgrades)

If the Hub operator sets `C5_MIN_VERSION`, workers below that version receive an `upgrade` control message instead of a job. The worker automatically runs `cq upgrade` and restarts — no manual intervention needed.

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

## Submitting jobs from your laptop

See [Distributed Experiments](/examples/distributed-experiments) for the full submit workflow using `cq hub submit` and `cq.yaml`.
