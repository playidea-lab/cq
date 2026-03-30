#!/usr/bin/env bash
# gpu-status.sh — GPU 상태 조회
# 출력: JSON {gpu_count, gpus: [{id, name, memory_total, memory_free, utilization}]}
# fallback: nvidia-smi 없거나 실패 시 {gpu_count: 0, gpus: [], note: "..."}

set -uo pipefail

NVIDIA_SMI=$(command -v nvidia-smi 2>/dev/null || true)

if [ -z "$NVIDIA_SMI" ]; then
    printf '{"gpu_count": 0, "gpus": [], "note": "nvidia-smi not found"}\n'
    exit 0
fi

CSV=$("$NVIDIA_SMI" --query-gpu=index,name,memory.total,memory.free,utilization.gpu \
    --format=csv,noheader,nounits 2>/dev/null) || {
    printf '{"gpu_count": 0, "gpus": [], "note": "nvidia-smi query failed"}\n'
    exit 0
}

python3 - "$CSV" <<'PYEOF'
import sys, json

csv_data = sys.argv[1] if len(sys.argv) > 1 else ""
gpus = []
for line in csv_data.strip().splitlines():
    parts = [p.strip() for p in line.split(',')]
    if len(parts) < 5:
        continue
    try:
        gpus.append({
            "id": int(parts[0]),
            "name": parts[1],
            "memory_total": int(parts[2]),
            "memory_free": int(parts[3]),
            "utilization": int(parts[4])
        })
    except (ValueError, IndexError):
        continue

print(json.dumps({"gpu_count": len(gpus), "gpus": gpus}))
PYEOF
