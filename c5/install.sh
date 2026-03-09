#!/usr/bin/env bash
# C5 Worker/Edge 설치 스크립트
# Usage:
#   curl -fsSL https://github.com/PlayIdea-Lab/cq/releases/latest/download/install.sh | bash

set -e

# ── 색상 ──────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[c5]${NC} $*"; }
warn()  { echo -e "${YELLOW}[c5]${NC} $*"; }
error() { echo -e "${RED}[c5]${NC} $*" >&2; exit 1; }

REPO="PlayIdea-Lab/cq"
HUB_URL="${C5_HUB_URL:-https://piqsol-c5.fly.dev}"
API_KEY="${C5_API_KEY:-}"
INSTALL_DIR="${HOME}/.local/bin"
BINARY="${INSTALL_DIR}/c5"

if [ -z "$API_KEY" ]; then
  error "C5_API_KEY 환경변수가 설정되지 않았습니다. export C5_API_KEY=<your-key> 후 재실행하세요."
fi

# ── OS/ARCH 감지 ──────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) error "지원하지 않는 아키텍처: $ARCH" ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) error "지원하지 않는 OS: $OS" ;;
esac

ASSET="c5-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

# ── 바이너리 설치 ─────────────────────────────────────────────
mkdir -p "${INSTALL_DIR}"

info "다운로드: ${DOWNLOAD_URL}"
curl -fsSL "${DOWNLOAD_URL}" -o "${BINARY}"
chmod +x "${BINARY}"
info "설치 완료: ${BINARY}"

# ── PATH 안내 ─────────────────────────────────────────────────
if ! echo "$PATH" | tr ':' '\n' | grep -qx "${INSTALL_DIR}"; then
    warn "${INSTALL_DIR}이 PATH에 없습니다. 아래를 ~/.bashrc에 추가하세요:"
    warn "  export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

# ── 버전 확인 ─────────────────────────────────────────────────
"${BINARY}" --version 2>/dev/null || true

# ── 실행 안내 ─────────────────────────────────────────────────
echo ""
info "=== 실행 명령어 ==="
echo ""
echo "  # Worker 시작"
echo "  c5 worker --server ${HUB_URL} --api-key ${API_KEY}"
echo ""
echo "  # Edge Agent 시작"
echo "  c5 edge-agent --hub-url ${HUB_URL} --api-key ${API_KEY}"
