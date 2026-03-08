#!/usr/bin/env bash
# E2E smoke test: cq hub worker init → start → job → verify
# Usage: C5_HUB_URL=... C5_API_KEY=... bash scripts/smoke_test_gpu_worker.sh
set -euo pipefail

C5_HUB_URL=${C5_HUB_URL:?"C5_HUB_URL must be set"}
C5_API_KEY=${C5_API_KEY:?"C5_API_KEY must be set"}

WORKER_PID=""
cleanup() { [ -n "$WORKER_PID" ] && kill "$WORKER_PID" 2>/dev/null || true; }
trap cleanup EXIT

echo "==> Initializing GPU worker..."
cq hub worker init --hub-url "$C5_HUB_URL" --api-key "$C5_API_KEY"

echo "==> Starting worker in background..."
cq hub worker start &
WORKER_PID=$!

echo "==> Waiting for worker to come online (up to 30s)..."
online=0
for i in $(seq 1 30); do
  cq hub workers 2>/dev/null | grep -i online && online=1 && break
  sleep 1
done
if [ "$online" -eq 0 ]; then
  echo "FAIL: worker not online after 30s"
  exit 1
fi

echo "==> Submitting test job (nvidia-smi)..."
SUBMIT_OUT=$(cq hub submit --run "nvidia-smi")
JOB_ID=$(echo "$SUBMIT_OUT" | grep -o 'Job submitted: [^ ]*' | awk '{print $3}')

if [ -z "$JOB_ID" ]; then
  echo "FAIL: could not parse job_id from submit output: $SUBMIT_OUT"
  exit 1
fi

echo "==> Watching job $JOB_ID (timeout 120s)..."
cq hub watch "$JOB_ID" --timeout 120s || { echo "FAIL: job did not complete"; exit 1; }

echo "==> Verifying LAST JOB updated on worker..."
cq hub workers | grep -E "LAST JOB" || { echo "FAIL: last job not updated on worker"; exit 1; }

echo "PASS: E2E smoke test complete"
