#!/usr/bin/env bash
# E2E smoke test: cq hub worker init → start → job → verify
# Usage: C5_HUB_URL=... C5_API_KEY=... bash scripts/smoke_test_gpu_worker.sh
set -euo pipefail

C5_HUB_URL=${C5_HUB_URL:?"C5_HUB_URL must be set"}
C5_API_KEY=${C5_API_KEY:?"C5_API_KEY must be set"}

echo "==> Initializing GPU worker..."
cq hub worker init --non-interactive --hub-url "$C5_HUB_URL" --api-key "$C5_API_KEY"

echo "==> Starting worker in background..."
cq hub worker start &
WORKER_PID=$!

echo "==> Waiting 10s for worker to come online..."
sleep 10

echo "==> Checking worker is online..."
cq hub workers | grep -i online || { echo "FAIL: worker not online"; kill "$WORKER_PID" 2>/dev/null; exit 1; }

echo "==> Submitting test job (nvidia-smi)..."
JOB_ID=$(cq hub submit --run "nvidia-smi" | grep -oP "(?<=job_id: )\S+")

if [ -z "$JOB_ID" ]; then
  echo "FAIL: could not parse job_id from submit output"
  kill "$WORKER_PID" 2>/dev/null
  exit 1
fi

echo "==> Watching job $JOB_ID (timeout 120s)..."
cq hub watch "$JOB_ID" --timeout 120s || { echo "FAIL: job did not complete"; kill "$WORKER_PID" 2>/dev/null; exit 1; }

echo "==> Verifying LAST JOB updated on worker..."
cq hub workers | grep -E "LAST JOB" || { echo "FAIL: last job not updated on worker"; kill "$WORKER_PID" 2>/dev/null; exit 1; }

kill "$WORKER_PID" 2>/dev/null || true
echo "PASS: E2E smoke test complete"
