#!/usr/bin/env bash
# C5 GPU Worker 원커맨드 설정
# 사용법: curl -sSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker/setup.sh | bash
#   또는: bash setup.sh [HUB_URL] [API_KEY]
set -euo pipefail

HUB_URL="${1:-${C5_HUB_URL:-}}"
API_KEY="${2:-${C5_API_KEY:-}}"

# ── 색상 ──
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
ok()   { printf "${GREEN}[OK]${NC}   %s\n" "$1"; }
warn() { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
err()  { printf "${RED}[ERR]${NC}  %s\n" "$1"; }

echo "================================================"
echo "  C5 GPU Worker Setup"
echo "================================================"
echo ""

# ── 1. cq + c5 설치 확인 ──
if ! command -v c5 &>/dev/null; then
    echo "c5 바이너리가 없습니다. 설치합니다..."
    curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
    export PATH="$HOME/.local/bin:$PATH"
fi
ok "c5: $(c5 version 2>/dev/null || echo 'installed')"

# ── 2. HUB_URL / API_KEY 입력 ──
if [ -z "$HUB_URL" ]; then
    printf "Hub URL (예: https://your-hub:8585): "
    read -r HUB_URL
fi
if [ -z "$API_KEY" ]; then
    printf "API Key: "
    read -r API_KEY
fi

if [ -z "$HUB_URL" ] || [ -z "$API_KEY" ]; then
    err "HUB_URL과 API_KEY 둘 다 필요합니다"
    echo "  사용법: bash setup.sh <HUB_URL> <API_KEY>"
    echo "  또는:   C5_HUB_URL=... C5_API_KEY=... bash setup.sh"
    exit 1
fi

# ── 3. Hub 연결 확인 ──
echo ""
echo "Hub 연결 확인: $HUB_URL"
if curl -sf --max-time 5 "$HUB_URL/v1/health" >/dev/null 2>&1; then
    ok "Hub 연결 성공"
else
    warn "Hub 연결 실패 — URL을 확인하세요 (계속 진행)"
fi

# ── 4. GPU 감지 ──
GPU_COUNT=0
if command -v nvidia-smi &>/dev/null; then
    GPU_COUNT=$(nvidia-smi --query-gpu=count --format=csv,noheader,nounits 2>/dev/null | head -1 || echo 0)
    GPU_NAME=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1 || echo "unknown")
    ok "GPU: ${GPU_COUNT}x ${GPU_NAME}"
else
    warn "nvidia-smi 없음 — CPU-only 모드"
fi

# ── 5. 작업 디렉토리 생성 ──
WORK_DIR="$HOME/c5-worker"
mkdir -p "$WORK_DIR/scripts"
cd "$WORK_DIR"
ok "작업 디렉토리: $WORK_DIR"

# ── 6. caps.yaml + 스크립트 다운로드 ──
BASE_URL="https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker"

curl -sfL "$BASE_URL/gpu-caps.yaml" -o gpu-caps.yaml 2>/dev/null || cat > gpu-caps.yaml <<'CAPS'
capabilities:
  - name: gpu_status
    description: "GPU 상태 조회"
    command: scripts/gpu-status.sh
    input_schema:
      type: object
      properties: {}
    tags: [gpu, status]

  - name: gpu_train
    description: "GPU 학습 실행"
    command: scripts/gpu-train.sh
    input_schema:
      type: object
      properties:
        script:
          type: string
          description: "Python 스크립트 경로"
        args:
          type: string
          description: "추가 인수"
    tags: [gpu, train]

  - name: run_command
    description: "범용 셸 명령 실행"
    command: scripts/run-command.sh
    input_schema:
      type: object
      properties:
        command:
          type: string
          description: "실행할 명령"
    tags: [shell]
CAPS

for script in gpu-status.sh gpu-train.sh gpu-infer.sh; do
    curl -sfL "$BASE_URL/scripts/$script" -o "scripts/$script" 2>/dev/null || true
done

# run-command.sh (범용)
cat > scripts/run-command.sh <<'RUN'
#!/usr/bin/env bash
set -uo pipefail
PARAMS="${C5_PARAMS:-{}}"
CMD=$(python3 -c "import sys,json; print(json.loads(sys.argv[1]).get('command','echo no command'))" "$PARAMS" 2>/dev/null || echo "echo no command")
RESULT_FILE="${C5_RESULT_FILE:-}"
LOGFILE=$(mktemp); trap 'rm -f "$LOGFILE"' EXIT
set +e; eval "$CMD" 2>&1 | tee "$LOGFILE"; EC=${PIPESTATUS[0]}; set -e
if [ -n "$RESULT_FILE" ]; then
    python3 -c "import json,sys; print(json.dumps({'exit_code':int(sys.argv[1]),'output':sys.argv[2]}))" "$EC" "$(tail -c 65536 "$LOGFILE")" > "$RESULT_FILE"
fi
exit "$EC"
RUN

chmod +x scripts/*.sh
ok "caps.yaml + 스크립트 설치"

# ── 7. .env 파일 생성 ──
cat > .env <<ENV
C5_HUB_URL=$HUB_URL
C5_API_KEY=$API_KEY
ENV
chmod 600 .env
ok ".env 생성 (chmod 600)"

# ── 8. systemd 서비스 등록 ──
if command -v systemctl &>/dev/null; then
    mkdir -p "$HOME/.config/systemd/user"
    cat > "$HOME/.config/systemd/user/c5-worker.service" <<SVC
[Unit]
Description=C5 GPU Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$WORK_DIR
EnvironmentFile=$WORK_DIR/.env
ExecStart=$HOME/.local/bin/c5 worker --capabilities $WORK_DIR/gpu-caps.yaml --server %E_C5_HUB_URL
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
SVC

    # EnvironmentFile에서 %E 안 쓰고 직접 치환
    cat > "$HOME/.config/systemd/user/c5-worker.service" <<SVC
[Unit]
Description=C5 GPU Worker
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$WORK_DIR
EnvironmentFile=$WORK_DIR/.env
ExecStart=$HOME/.local/bin/c5 worker --capabilities $WORK_DIR/gpu-caps.yaml --server $HUB_URL
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
SVC

    systemctl --user daemon-reload
    systemctl --user enable c5-worker.service 2>/dev/null || true
    ok "systemd 서비스 등록: c5-worker.service"
    echo ""
    echo "  시작: systemctl --user start c5-worker"
    echo "  로그: journalctl --user -u c5-worker -f"
    echo "  중지: systemctl --user stop c5-worker"
else
    warn "systemd 없음 — 수동 실행 필요"
fi

# ── 9. 요약 ──
echo ""
echo "================================================"
echo "  설정 완료!"
echo "================================================"
echo ""
echo "  디렉토리: $WORK_DIR"
echo "  Hub:      $HUB_URL"
echo "  GPU:      ${GPU_COUNT}x"
echo ""
echo "  수동 실행:"
echo "    cd $WORK_DIR"
echo "    source .env"
echo "    c5 worker --capabilities gpu-caps.yaml --server \$C5_HUB_URL"
echo ""
echo "  systemd:"
echo "    systemctl --user start c5-worker"
echo ""
