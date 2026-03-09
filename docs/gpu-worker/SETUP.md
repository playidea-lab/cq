# GPU Worker Setup Guide

Connect a remote GPU server to the C5 Hub using `cq` in 5 steps.

## Prerequisites

| Item | Required | Notes |
|------|----------|-------|
| `cq` binary | Required | See Step 1 |
| `C5_HUB_URL` | Required | Hub server URL |
| `C5_API_KEY` | Required | Hub API key |
| `nvidia-smi` | Optional | CPU-only fallback if absent |

---

## Step 1 — Install cq

```bash
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
# verify
cq version
```

---

## Step 2 — Set environment variables

```bash
export C5_HUB_URL="https://piqsol-c5.fly.dev"   # or your self-hosted Hub URL
export C5_API_KEY="<your-hub-api-key>"
```

Persist in `~/.bashrc` or `~/.zshrc` for permanent setup.

---

## Step 3 — Initialize worker

```bash
cq hub worker init --non-interactive \
  --hub-url "$C5_HUB_URL" \
  --api-key "$C5_API_KEY"
```

This registers the worker with the Hub and writes local config to `~/.c5/config.yaml`.

---

## Step 4 — Start worker

```bash
# Foreground (development)
cq hub worker start

# Background / systemd (production)
cq hub worker start &
```

The worker polls the Hub for jobs. GPU presence is auto-detected via `nvidia-smi`.

---

## Step 5 — Run E2E smoke test

```bash
bash scripts/smoke_test_gpu_worker.sh
```

Expected output:

```
==> Initializing GPU worker...
==> Starting worker in background...
==> Waiting for worker to come online (up to 30s)...
==> Submitting test job (nvidia-smi)...
==> Watching job <job_id> (timeout 120s)...
==> Verifying LAST JOB updated on worker...
PASS: E2E smoke test complete
```

---

## Troubleshooting

### nvidia-smi not installed

The worker runs in CPU-only mode automatically. This is normal — `cq hub worker start` detects GPU presence and falls back gracefully. No action required.

```
[worker] nvidia-smi not found — starting in CPU-only mode
```

### API key error

Re-run init with the correct key:

```bash
cq hub worker init --non-interactive \
  --hub-url "$C5_HUB_URL" \
  --api-key "<correct-key>"
```

Check that `C5_API_KEY` matches the key issued in the Hub admin panel.

### Version gate: worker rejected by Hub

If you see `control: {action:"upgrade"}` in logs, the worker version is too old:

```bash
cq upgrade        # update cq binary
cq hub worker start   # restart worker
```

The Hub's `C5_MIN_VERSION` env controls the minimum allowed version.

### Worker shows offline after `cq hub workers`

- Ensure `cq hub worker start` is running (check `ps aux | grep cq`)
- Confirm `C5_HUB_URL` is reachable: `curl -s "$C5_HUB_URL/v1/health"`
- Check firewall rules — worker initiates outbound connections to the Hub

### Job stuck / timeout

- Default job timeout in smoke test is 120s. Increase with `--timeout 300s` if on slow GPU.
- Inspect job logs: `cq hub log <job_id>`
