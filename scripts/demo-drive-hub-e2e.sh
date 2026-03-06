#!/usr/bin/env bash
# demo-drive-hub-e2e.sh
# Drive Dataset Versioning + C5 Hub 원격 실행 E2E 데모 재현 스크립트
#
# 전제조건 (릴리즈 바이너리 기준):
#   1. cq 설치됨 (GitHub Releases — Supabase URL/Key 내장)
#   2. cq auth login 완료 (또는 cq auth token으로 세션 주입)
#   3. cq project use <name> 완료 (active_project_id가 .c4/config.yaml에 기록됨)
#   4. hub.url 이 .c4/config.yaml에 설정되거나 C5_HUB_URL env var 설정
#
# 사용법:
#   bash scripts/demo-drive-hub-e2e.sh            # 업로드 + 잡 제출
#   bash scripts/demo-drive-hub-e2e.sh --worker   # 원격 서버: c5 worker도 실행
#   bash scripts/demo-drive-hub-e2e.sh --clean    # 디렉토리 초기화 후 재실행
#
# 선택적 env var:
#   C5_HUB_URL   — .c4/config.yaml hub.url 미설정 시 필요
#   DEMO_BASE    — 데모 디렉토리 생성 위치 (기본: $HOME/git)
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[demo]${NC} $*"; }
success() { echo -e "${GREEN}[demo]${NC} $*"; }
warn()    { echo -e "${YELLOW}[demo]${NC} $*"; }
die()     { echo -e "${RED}[demo] ERROR:${NC} $*" >&2; exit 1; }

WORKER_MODE=false
CLEAN_MODE=false
for arg in "$@"; do
    case "$arg" in
        --worker) WORKER_MODE=true ;;
        --clean)  CLEAN_MODE=true ;;
        *) die "Unknown argument: $arg (use --worker or --clean)" ;;
    esac
done

# ── 의존성 체크 ──────────────────────────────────────────────────────
command -v cq      >/dev/null 2>&1 || die "cq not found. Install: https://github.com/PlayIdea-Lab/cq/releases"
command -v python3 >/dev/null 2>&1 || die "python3 not found"

# ── cq 인증 확인 ──────────────────────────────────────────────────────
info "Checking cq auth status..."
if ! cq auth status >/dev/null 2>&1; then
    die "Not authenticated. Run: cq auth login"
fi
success "Auth OK"

# ── Hub URL 결정 ──────────────────────────────────────────────────────
# 우선순위: env var > .c4/config.yaml hub.url
HUB_URL="${C5_HUB_URL:-}"
if [[ -z "$HUB_URL" ]]; then
    # config.yaml에서 읽기
    CONFIG_PATH="${PWD}/.c4/config.yaml"
    if [[ -f "$CONFIG_PATH" ]]; then
        HUB_URL=$(python3 -c "
import re, sys
content = open('${CONFIG_PATH}').read()
m = re.search(r'hub:\s*\n(?:.*\n)*?.*url:\s*(.+)', content)
if m:
    print(m.group(1).strip())
" 2>/dev/null || echo "")
    fi
fi
if [[ -z "$HUB_URL" ]]; then
    die "Hub URL not set. Either:\n  - export C5_HUB_URL=https://...\n  - set hub.url in .c4/config.yaml"
fi
info "Hub URL: $HUB_URL"

# ── c5 바이너리 탐색 (--worker 모드 시) ──────────────────────────────
C5_BIN=""
if $WORKER_MODE; then
    for candidate in c5 "$HOME/c5" "$HOME/.local/bin/c5"; do
        if command -v "$candidate" >/dev/null 2>&1 || [[ -x "$candidate" ]]; then
            C5_BIN="$candidate"
            break
        fi
    done
    [[ -n "$C5_BIN" ]] || die "c5 binary not found. Download from: https://github.com/PlayIdea-Lab/cq/releases"
fi

# ── 디렉토리 설정 ────────────────────────────────────────────────────
DEMO_BASE="${DEMO_BASE:-$HOME/git}"
DRIVE_DIR="$DEMO_BASE/example-drive"
REMOTE_DIR="$DEMO_BASE/example-remote"

if $CLEAN_MODE; then
    warn "--clean: removing $DRIVE_DIR and $REMOTE_DIR..."
    rm -rf "$DRIVE_DIR" "$REMOTE_DIR"
fi

# ══════════════════════════════════════════════════════════════════════
# Part 1: example-drive 설정 (데이터 + 코드)
# ══════════════════════════════════════════════════════════════════════

info "=== Part 1: Setting up example-drive ==="

mkdir -p "$DRIVE_DIR/.c4"
mkdir -p "$DRIVE_DIR/data"
mkdir -p "$DRIVE_DIR/code"

# .c4/config.yaml — active_project_id는 현재 프로젝트에서 복사
# (Supabase URL/Key는 바이너리에 내장 → config에 불필요)
ACTIVE_PROJECT_ID=$(python3 -c "
import re
try:
    content = open('${PWD}/.c4/config.yaml').read()
    m = re.search(r'active_project_id:\s*(.+)', content)
    if m: print(m.group(1).strip())
except: pass
" 2>/dev/null || echo "")

if [[ -z "$ACTIVE_PROJECT_ID" ]]; then
    die "active_project_id not set in .c4/config.yaml. Run: cq project use <name>"
fi
info "Using project: $ACTIVE_PROJECT_ID"

cat > "$DRIVE_DIR/.c4/config.yaml" <<EOF
cloud:
  active_project_id: ${ACTIVE_PROJECT_ID}

hub:
  enabled: true
  url: ${HUB_URL}
EOF
success "Created $DRIVE_DIR/.c4/config.yaml (credentials from binary)"

# 샘플 데이터
if [[ ! -f "$DRIVE_DIR/data/sales_jan.csv" ]]; then
cat > "$DRIVE_DIR/data/sales_jan.csv" <<'EOF'
date,product,quantity,unit_price
2024-01-05,Widget A,10,15.99
2024-01-10,Widget B,5,29.99
2024-01-15,Gadget X,3,49.99
2024-01-20,Widget A,8,15.99
2024-01-25,Gadget Y,2,89.99
EOF
success "Created data/sales_jan.csv"
else
    info "data/sales_jan.csv already exists — skipping"
fi

if [[ ! -f "$DRIVE_DIR/data/sales_feb.csv" ]]; then
cat > "$DRIVE_DIR/data/sales_feb.csv" <<'EOF'
date,product,quantity,unit_price
2024-02-02,Widget A,12,15.99
2024-02-08,Widget B,7,29.99
2024-02-14,Gadget X,4,49.99
2024-02-20,Widget A,6,15.99
2024-02-28,Gadget Y,3,89.99
EOF
success "Created data/sales_feb.csv"
fi

if [[ ! -f "$DRIVE_DIR/code/analyze.py" ]]; then
cat > "$DRIVE_DIR/code/analyze.py" <<'EOF'
"""
Sales analysis script — v2 (bug fixed: unit_price)
Usage: python analyze.py <data_dir> [output_file]
"""
import sys, os, csv
from pathlib import Path
from collections import defaultdict

def load_sales(data_dir):
    rows = []
    for csv_file in sorted(Path(data_dir).glob("*.csv")):
        with open(csv_file) as f:
            for row in csv.DictReader(f):
                rows.append(row)
    return rows

def summarize(rows):
    by_product = defaultdict(lambda: {"quantity": 0, "revenue": 0.0})
    for row in rows:
        qty = int(row["quantity"])
        price = float(row["unit_price"])
        by_product[row["product"]]["quantity"] += qty
        by_product[row["product"]]["revenue"] += qty * price
    return by_product

def main():
    data_dir = sys.argv[1] if len(sys.argv) > 1 else "."
    output_file = sys.argv[2] if len(sys.argv) > 2 else "results/summary.txt"
    rows = load_sales(data_dir)
    print(f"Loaded {len(rows)} records from {data_dir}")
    summary = summarize(rows)
    os.makedirs(os.path.dirname(output_file) or ".", exist_ok=True)
    total = 0.0
    with open(output_file, "w") as f:
        f.write("=== Sales Summary ===\n\n")
        for product, stats in sorted(summary.items()):
            line = f"{product}: qty={stats['quantity']}, revenue=${stats['revenue']:.2f}\n"
            f.write(line); print(line, end="")
            total += stats["revenue"]
        f.write(f"\nTotal Revenue: ${total:.2f}\n")
    print(f"\nTotal Revenue: ${total:.2f}")
    print(f"Output: {output_file}")

if __name__ == "__main__":
    main()
EOF
cat > "$DRIVE_DIR/code/requirements.txt" <<'EOF'
# No external dependencies — stdlib only
EOF
success "Created code/analyze.py + requirements.txt"
fi

# ── Drive 업로드 ──────────────────────────────────────────────────────
info "Uploading datasets to Drive (incremental)..."
(cd "$DRIVE_DIR" && cq drive dataset upload example-data ./data --no-serve 2>&1 | sed 's/^/  /')
success "Uploaded example-data"

(cd "$DRIVE_DIR" && cq drive dataset upload example-code ./code --no-serve 2>&1 | sed 's/^/  /')
success "Uploaded example-code"

# ══════════════════════════════════════════════════════════════════════
# Part 2: example-remote 설정 (워커 사이드)
# ══════════════════════════════════════════════════════════════════════

info "=== Part 2: Setting up example-remote (worker side) ==="

mkdir -p "$REMOTE_DIR"

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

# run_job.sh — cq 바이너리에 Supabase 자격증명 내장 → env var 불필요
cat > "$REMOTE_DIR/run_job.sh" <<SCRIPT
#!/usr/bin/env bash
# C5 worker job script — cq 릴리즈 바이너리 기준 (Supabase 자격증명 내장)
set -euo pipefail

WORK_DIR="\$(dirname "\$0")"
cd "\$WORK_DIR"

echo "[job] starting at \$(date)"

DATA_DATASET=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('data_dataset','example-data'))")
CODE_DATASET=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('code_dataset','example-code'))")
SCRIPT=\$(echo "\$C5_PARAMS" | python3 -c "import sys,json; p=json.load(sys.stdin); print(p.get('script','analyze.py'))")

echo "[job] data=\$DATA_DATASET code=\$CODE_DATASET script=\$SCRIPT"

echo "[job] pulling datasets..."
cq drive dataset pull "\$DATA_DATASET" --dest ./data --no-serve 2>&1
cq drive dataset pull "\$CODE_DATASET" --dest ./code --no-serve 2>&1

echo "[job] running \$SCRIPT..."
mkdir -p results
python3 "code/\$SCRIPT" ./data results/summary.txt 2>&1

RESULT_JSON=\$(python3 -c "
import json, os
output = open('results/summary.txt').read() if os.path.exists('results/summary.txt') else ''
print(json.dumps({'status': 'ok', 'output': output}))
")

echo "[job] done"
echo "\$RESULT_JSON" > "\$C5_RESULT_FILE"
SCRIPT
chmod +x "$REMOTE_DIR/run_job.sh"
success "Created $REMOTE_DIR/run_job.sh"

# run_job.sh도 .c4/config.yaml 필요 (active_project_id for drive pull)
mkdir -p "$REMOTE_DIR/.c4"
cat > "$REMOTE_DIR/.c4/config.yaml" <<EOF
cloud:
  active_project_id: ${ACTIVE_PROJECT_ID}

hub:
  enabled: true
  url: ${HUB_URL}
EOF
success "Created $REMOTE_DIR/.c4/config.yaml"

# ── 워커 실행 (--worker 플래그) ───────────────────────────────────────
if $WORKER_MODE; then
    WORKER_LOG="$REMOTE_DIR/worker.log"
    info "Starting c5 worker (caps: $REMOTE_DIR/caps.yaml)..."
    nohup "$C5_BIN" worker \
        --capabilities "$REMOTE_DIR/caps.yaml" \
        --hub "$HUB_URL" \
        > "$WORKER_LOG" 2>&1 &
    WORKER_PID=$!
    success "c5 worker started (PID=$WORKER_PID) — logs: $WORKER_LOG"
    sleep 1
    head -5 "$WORKER_LOG" | sed 's/^/  /'
fi

# ══════════════════════════════════════════════════════════════════════
# Part 3: 잡 제출 및 결과 확인
# ══════════════════════════════════════════════════════════════════════

info "=== Part 3: Submit job to C5 Hub ==="

if ! $WORKER_MODE; then
    warn "Worker is not running (no --worker flag)."
    warn "Ensure 'c5 worker --capabilities caps.yaml --hub $HUB_URL' runs on the target machine."
fi

info "Submitting data-analysis job..."
JOB_RESPONSE=$(curl -s -X POST "${HUB_URL}/v1/capabilities/invoke" \
    -H "Content-Type: application/json" \
    -d "{
        \"capability\": \"data-analysis\",
        \"params\": {
            \"data_dataset\": \"example-data\",
            \"code_dataset\": \"example-code\",
            \"script\": \"analyze.py\"
        }
    }")

echo "  $JOB_RESPONSE"
JOB_ID=$(echo "$JOB_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('job_id',''))" 2>/dev/null || echo "")

if [[ -z "$JOB_ID" ]]; then
    warn "Could not extract job_id. Check Hub connectivity."
else
    success "Job submitted: $JOB_ID"
    info "Polling for result (max 60s)..."
    for i in $(seq 1 12); do
        sleep 5
        STATUS_RESP=$(curl -s "${HUB_URL}/v1/jobs/${JOB_ID}" 2>/dev/null || echo "{}")
        STATUS=$(echo "$STATUS_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null || echo "unknown")
        echo "  [${i}] status=$STATUS"
        if [[ "$STATUS" == "succeeded" || "$STATUS" == "failed" ]]; then
            echo ""
            echo "$STATUS_RESP" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'Status: {d.get(\"status\")}')
result = d.get('result') or {}
if isinstance(result, str):
    try: result = json.loads(result)
    except: pass
if isinstance(result, dict):
    print(f'Output:\n{result.get(\"output\",\"\")}')
" 2>/dev/null || echo "$STATUS_RESP"
            break
        fi
    done
fi

echo ""
success "=== E2E Demo Complete ==="
echo ""
echo "재현 체크리스트:"
echo "  1. cq 바이너리: Supabase 자격증명 내장 (env var 불필요)"
echo "  2. active_project_id: $ACTIVE_PROJECT_ID"
echo "  3. hub.url: $HUB_URL"
echo "  4. 업로드: example-data, example-code datasets"
echo "  5. 잡: data-analysis → 워커 pull → 스크립트 실행 → 결과 반환"
echo ""
echo "코드 수정 후 증분 업로드 테스트:"
echo "  edit $DRIVE_DIR/code/analyze.py"
echo "  cd $DRIVE_DIR && cq drive dataset upload example-code ./code --no-serve"
echo "  # 변경된 파일만 업로드됨"
