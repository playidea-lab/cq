#!/bin/sh
# CQ installer — POSIX sh compatible
# Usage: sh install.sh --tier solo
#        sh install.sh --dry-run --tier connected

set -e

TIER="solo"
DRY_RUN=0
INSTALL_DIR="${HOME}/.local/bin"

while [ $# -gt 0 ]; do
  case "$1" in
    --tier)      shift; TIER="$1" ;;
    --tier=*)    TIER="${1#--tier=}" ;;
    --dry-run)   DRY_RUN=1 ;;
    --install-dir) shift; INSTALL_DIR="$1" ;;
    --install-dir=*) INSTALL_DIR="${1#--install-dir=}" ;;
    -h|--help)
      echo "Usage: install.sh [--tier solo|connected|full] [--dry-run]"
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

case "$TIER" in
  solo|connected|full) ;;
  *) echo "Error: invalid tier '$TIER'. Must be solo, connected, or full." >&2; exit 1 ;;
esac

OS="$(uname -s)"
case "$OS" in
  Linux)  GOOS="linux" ;;
  Darwin) GOOS="darwin" ;;
  *) echo "Error: unsupported OS '$OS'" >&2; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "Error: unsupported arch '$ARCH'" >&2; exit 1 ;;
esac

ARTIFACT="cq-${TIER}-${GOOS}-${GOARCH}"
URL="https://github.com/PlayIdea-Lab/cq/releases/latest/download/${ARTIFACT}"

if [ "$DRY_RUN" = "1" ]; then
  echo "Would download: ${URL}"
  echo "Would install to: ${INSTALL_DIR}/cq"
  exit 0
fi

echo "Installing cq (tier: ${TIER}, os: ${GOOS}, arch: ${GOARCH})..."

if ! curl -sf --head "${URL}" > /dev/null 2>&1; then
  echo "Error: Release not found: ${URL}" >&2
  exit 1
fi

mkdir -p "${INSTALL_DIR}"
TMP_FILE="$(mktemp)"
if ! curl -fsSL -o "${TMP_FILE}" "${URL}"; then
  echo "Error: failed to download ${URL}" >&2
  rm -f "${TMP_FILE}"
  exit 1
fi

chmod +x "${TMP_FILE}"
mv "${TMP_FILE}" "${INSTALL_DIR}/cq"
echo "Installed: ${INSTALL_DIR}/cq"

if [ -x "${INSTALL_DIR}/cq" ]; then
  echo ""
  "${INSTALL_DIR}/cq" doctor || true
fi

echo ""
echo "Done. Make sure ${INSTALL_DIR} is in your PATH."
