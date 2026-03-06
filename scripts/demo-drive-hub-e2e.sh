#!/usr/bin/env bash
# demo-drive-hub-e2e.sh
# Drive Dataset Versioning + C5 Hub 원격 실행 E2E 데모 재현 스크립트
#
# 사용법:
#   export C4_CLOUD_URL=https://xxx.supabase.co
#   export C4_CLOUD_ANON_KEY=eyJhbGci...
#   export C4_PROJECT_ID=<UUID>          # cq project list 로 확인
#   export C5_HUB_URL=https://your-c5.fly.dev
#   bash scripts/demo-drive-hub-e2e.sh [--worker] [--clean]
#
# 플래그:
#   --worker  워커 모드: example-remote 설정 + c5 worker 백그라운드 실행
#   --clean   기존 example-drive/example-remote 디렉토리 삭제 후 재생성
#
# 의존성: cq (최신 빌드), c5 바이너리 (PATH에 있거나 ~/c5), python3
set -euo pipefail

# ── 색상 ────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[demo]${NC} $*"; }
success() { echo -e "${GREEN}[demo]${NC} $*"; }
warn()    { echo -e "${YELLOW}[demo]${NC} $*"; }
die()     { echo -e "${RED}[demo] ERROR:${NC} $*" >&2; exit 1; }

# ── 인자 파싱 ────────────────────────────────────────────────────────
WORKER_MODE=false
CLEAN_MODE=false
for arg in "$@"; do
    case "$arg" in
        --worker) WORKER_MODE=true ;;
        --clean)  CLEAN_MODE=true ;;
        *) die "Unknown argument: $arg" ;;
    esac
done

# ── 환경 변수 검증 ───────────────────────────────────────────────────
: "${C4_CLOUD_URL:?Set C4_CLOUD_URL to your Supabase URL}"
: "${C4_CLOUD_ANON_KEY:?Set C4_CLOUD_ANON_KEY to your Supabase anon key}"
: "${C4_PROJECT_ID:?Set C4_PROJECT_ID to your project UUID (cq project list)}"
: "${C5_HUB_URL:?Set C5_HUB_URL to your C5 Hub URL (e.g. https://piqsol-c5.fly.dev)}"

# ── 의존성 체크 ──────────────────────────────────────────────────────
command -v cq     >/dev/null 2>&1 || die "cq not found in PATH (build: go build -o ~/.local/bin/cq ./cmd/c4/)"
command -v python3 >/dev/null 2>&1 || die "python3 not found"
C5_BIN=""
if command -v c5 >/dev/null 2>&1; then
    C5_BIN=$(command -v c5)
elif [[ -x "$HOME/c5" ]]; then
    C5_BIN="$HOME/c5"
fi

# ── 디렉토리 설정 ────────────────────────────────────────────────────
DEMO_BASE="${DEMO_BASE:-$HOME/git}"
DRIVE_DIR="$DEMO_BASE/example-drive"
REMOTE_DIR="$DEMO_BASE/example-remote"

if $CLEAN_MODE; then
    warn "--clean: removing existing directories..."
    rm -rf "$DRIVE_DIR" "$REMOTE_DIR"
fi

# ══════════════════════════════════════════════════════════════════════
# Part 1: example-drive 설정 (노트북에서 실행)
# ══════════════════════════════════════════════════════════════════════

info "=== Part 1: Setting up example-drive ==="

mkdir -p "$DRIVE_DIR/.c4"
mkdir -p "$DRIVE_DIR/data"
mkdir -p "$DRIVE_DIR/code"

# .c4/config.yaml
cat > "$DRIVE_DIR/.c4/config.yaml" <<EOF
cloud:
  enabled: true
  url: ${C4_CLOUD_URL}
  anon_key: ${C4_CLOUD_ANON_KEY}
  active_project_id: ${C4_PROJECT_ID}

hub:
  enabled: true
  url: ${C5_HUB_URL}
EOF
success "Created $DRIVE_DIR/.c4/config.yaml"

# 샘플 데이터: sales_jan.csv
cat > "$DRIVE_DIR/data/sales_jan.csv" <<'EOF'
date,product,quantity,unit_price
2024-01-05,Widget A,10,15.99
2024-01-10,Widget B,5,29.99
2024-01-15,Gadget X,3,49.99
2024-01-20,Widget A,8,15.99
2024-01-25,Gadget Y,2,89.99
EOF
success "Created data/sales_jan.csv"

# 샘플 데이터: sales_feb.csv
cat > "$DRIVE_DIR/data/sales_feb.csv" <<'EOF'
date,product,quantity,unit_price
2024-02-02,Widget A,12,15.99
2024-02-08,Widget B,7,29.99
2024-02-14,Gadget X,4,49.99
2024-02-20,Widget A,6,15.99
2024-02-28,Gadget Y,3,89.99
EOF
success "Created data/sales_feb.csv"

# 분석 코드: analyze.py (v2 — 버그 수정됨)
cat > "$DRIVE_DIR/code/analyze.py" <<'EOF'
"""
Sales analysis script — v2 (bug fixed: price -> unit_price)
Usage: python analyze.py <data_dir> [output_file]
"""
import sys
import os
import csv
from pathlib import Path
from collections import defaultdict

def load_sales(data_dir):
    rows = []
    for csv_file in sorted(Path(data_dir).glob("*.csv")):
        with open(csv_file) as f:
            reader = csv.DictReader(f)
            for row in reader:
                rows.append(row)
    return rows

def summarize(rows):
    by_product = defaultdict(lambda: {"quantity": 0, "revenue": 0.0})
    for row in rows:
        product = row["product"]
        qty = int(row["quantity"])
        price = float(row["unit_price"])  # fixed: was row["price"]
        by_product[product]["quantity"] += qty
        by_product[product]["revenue"] += qty * price
    return by_product

def main():
    data_dir = sys.argv[1] if len(sys.argv) > 1 else "."
    output_file = sys.argv[2] if len(sys.argv) > 2 else "results/summary.txt"

    print(f"Loading sales data from: {data_dir}")
    rows = load_sales(data_dir)
    print(f"Loaded {len(rows)} records")

    summary = summarize(rows)

    os.makedirs(os.path.dirname(output_file) or ".", exist_ok=True)
    with open(output_file, "w") as f:
        f.write("=== Sales Summary ===\n\n")
        total_revenue = 0.0
        for product, stats in sorted(summary.items()):
            line = f"{product}: qty={stats['quantity']}, revenue=${stats['revenue']:.2f}\n"
            f.write(line)
            print(line, end="")
            total_revenue += stats["revenue"]
        f.write(f"\nTotal Revenue: ${total_revenue:.2f}\n")

    print(f"\nOutput written to: {output_file}")

if __name__ == "__main__":
    main()
EOF
success "Created code/analyze.py (v2, bug fixed)"

# requirements.txt
cat > "$DRIVE_DIR/code/requirements.txt" <<'EOF'
# No external dependencies — stdlib only
EOF

# ── Drive 업로드 ──────────────────────────────────────────────────────
info "Uploading datasets to Drive..."
(
    cd "$DRIVE_DIR"
    C4_CLOUD_URL="$C4_CLOUD_URL" \
    C4_CLOUD_ANON_KEY="$C4_CLOUD_ANON_KEY" \
    cq drive dataset upload example-data ./data --no-serve 2>&1 | sed 's/^/  [upload-data] /'
)
success "Uploaded example-data dataset"

(
    cd "$DRIVE_DIR"
    C4_CLOUD_URL="$C4_CLOUD_URL" \
    C4_CLOUD_ANON_KEY="$C4_CLOUD_ANON_KEY" \
    cq drive dataset upload example-code ./code --no-serve 2>&1 | sed 's/^/  [upload-code] /'
)
success "Uploaded example-code dataset"

# ══════════════════════════════════════════════════════════════════════
# Part 2: example-remote 설정 (원격 서버에서 실행 — 이 스크립트가 대신)
# ══════════════════════════════════════════════════════════════════════

info "=== Part 2: Setting up example-remote (worker side) ==="

mkdir -p "$REMOTE_DIR"

# caps.yaml
cat > "$REMOTE_DIR/caps.yaml" <<EOF
capabilities:
  - name: data-analysis
    description: Pull datasets from CQ Drive and run Python analysis script
    command: bash ${REMOTE_DIR}/run_job.sh
    params:
      - name: data_dataset
        type: string
        description: Dataset name for input data
        required: true
      - name: code_dataset
        type: string
        description: Dataset name for analysis code
        required: true
      - name: script
        type: string
        description: Script filename to run (relative to code/)
        default: analyze.py
EOF
success "Created $REMOTE_DIR/caps.yaml"

# run_job.sh
cat > "$REMOTE_DIR/run_job.sh" <<SCRIPT
#!/usr/bin/env bash
# C5 worker job script — pull Drive datasets, run analysis, write results
set -euo pipefail

WORK_DIR="\$(dirname "\$0")"
cd "\$WORK_DIR"

echo "[job] starting at \$(date)"

# Parse params from C5_PARAMS env (JSON)
DATA_DATASET=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('data_dataset','example-data'))")
CODE_DATASET=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('code_dataset','example-code'))")
SCRIPT=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('script','analyze.py'))")

echo "[job] data_dataset=\$DATA_DATASET code_dataset=\$CODE_DATASET script=\$SCRIPT"

# Pull latest datasets (incremental — skips unchanged files)
echo "[job] pulling data..."
C4_CLOUD_URL="${C4_CLOUD_URL}" \\
C4_CLOUD_ANON_KEY="${C4_CLOUD_ANON_KEY}" \\
cq drive dataset pull "\$DATA_DATASET" --dest ./data --no-serve 2>&1

echo "[job] pulling code..."
C4_CLOUD_URL="${C4_CLOUD_URL}" \\
C4_CLOUD_ANON_KEY="${C4_CLOUD_ANON_KEY}" \\
cq drive dataset pull "\$CODE_DATASET" --dest ./code --no-serve 2>&1

# Run analysis
echo "[job] running \$SCRIPT..."
mkdir -p results
python3 "code/\$SCRIPT" ./data results/summary.txt 2>&1

RESULT_JSON=\$(python3 -c "
import json, os
result_file = 'results/summary.txt'
output = open(result_file).read() if os.path.exists(result_file) else ''
print(json.dumps({'status': 'ok', 'output': output}))
")

echo "[job] done"

# Write result for C5 to collect
echo "\$RESULT_JSON" > "\$C5_RESULT_FILE"
SCRIPT
chmod +x "$REMOTE_DIR/run_job.sh"
success "Created $REMOTE_DIR/run_job.sh"

# ── 워커 실행 (--worker 모드) ─────────────────────────────────────────
if $WORKER_MODE; then
    if [[ -z "$C5_BIN" ]]; then
        die "c5 binary not found. Install it or set PATH. Download: https://github.com/PlayIdea-Lab/cq/releases"
    fi
    info "Starting c5 worker in background (caps: $REMOTE_DIR/caps.yaml)..."
    nohup "$C5_BIN" worker \
        --capabilities "$REMOTE_DIR/caps.yaml" \
        --hub "$C5_HUB_URL" \
        > "$REMOTE_DIR/worker.log" 2>&1 &
    WORKER_PID=$!
    success "c5 worker started (PID=$WORKER_PID) — logs: $REMOTE_DIR/worker.log"
    info "To stop: kill $WORKER_PID"
fi

# ══════════════════════════════════════════════════════════════════════
# Part 3: 잡 제출 및 결과 확인
# ══════════════════════════════════════════════════════════════════════

info "=== Part 3: Submit job to C5 Hub ==="

if ! $WORKER_MODE; then
    warn "Worker not started by this script (no --worker flag)."
    warn "Make sure 'c5 worker --capabilities caps.yaml --hub $C5_HUB_URL' is running on the target machine."
    echo ""
fi

info "Submitting data-analysis job..."
JOB_RESPONSE=$(curl -s -X POST "${C5_HUB_URL}/v1/capabilities/invoke" \
    -H "Content-Type: application/json" \
    -d '{
        "capability": "data-analysis",
        "params": {
            "data_dataset": "example-data",
            "code_dataset": "example-code",
            "script": "analyze.py"
        }
    }')

echo "  Job response: $JOB_RESPONSE"
JOB_ID=$(echo "$JOB_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('job_id',''))" 2>/dev/null || echo "")

if [[ -z "$JOB_ID" ]]; then
    warn "Could not extract job_id from response. Check Hub connectivity."
else
    success "Job submitted: $JOB_ID"
    info "Polling for result (max 60s)..."

    for i in $(seq 1 12); do
        sleep 5
        STATUS_RESP=$(curl -s "${C5_HUB_URL}/v1/jobs/${JOB_ID}" 2>/dev/null || echo "{}")
        STATUS=$(echo "$STATUS_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null || echo "unknown")
        echo "  [${i}] status=$STATUS"

        if [[ "$STATUS" == "succeeded" || "$STATUS" == "failed" ]]; then
            echo ""
            echo "=== Final Result ==="
            echo "$STATUS_RESP" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f\"Status: {d.get('status')}\")
result = d.get('result') or {}
if isinstance(result, str):
    try: result = json.loads(result)
    except: pass
if isinstance(result, dict):
    print(f\"Output:\\n{result.get('output','')}\")
else:
    print(f\"Result: {result}\")
" 2>/dev/null || echo "$STATUS_RESP"
            break
        fi
    done
fi

echo ""
success "=== E2E Demo Complete ==="
echo ""
echo "What was demonstrated:"
echo "  1. Created example-drive/ with data + code"
echo "  2. Uploaded datasets to CQ Drive (CAS, incremental)"
echo "  3. Created example-remote/ with caps.yaml + run_job.sh"
echo "  4. Submitted job to C5 Hub → worker pulled datasets → ran analysis"
echo ""
echo "To test incremental re-upload after code change:"
echo "  1. Edit $DRIVE_DIR/code/analyze.py"
echo "  2. cd $DRIVE_DIR && cq drive dataset upload example-code ./code --no-serve"
echo "  3. Re-submit the job — worker will pull only changed files"
