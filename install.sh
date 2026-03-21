#!/bin/sh
# CQ installer — POSIX sh compatible
# Usage: curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
#        sh install.sh --dry-run
#        sh install.sh --version v1.6.2

set -e

DRY_RUN=0
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run)   DRY_RUN=1 ;;
    --install-dir) shift; INSTALL_DIR="$1" ;;
    --install-dir=*) INSTALL_DIR="${1#--install-dir=}" ;;
    --version) shift; VERSION="$1" ;;
    --version=*) VERSION="${1#--version=}" ;;
    -h|--help)
      echo "Usage: install.sh [--dry-run] [--version vX.Y.Z] [--install-dir PATH]"
      exit 0 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
  shift
done

OS="$(uname -s)"
case "$OS" in
  MINGW*|MSYS*|CYGWIN*)
    echo ""
    echo "  Windows detected ($OS)."
    echo ""
    echo "  CQ requires a POSIX environment. Two options:"
    echo ""
    echo "  Option 1 — WSL2 (recommended)"
    echo "    1. Install WSL2:  wsl --install  (PowerShell as Admin)"
    echo "    2. Open Ubuntu terminal and re-run this installer."
    echo ""
    echo "  Option 2 — Pre-built binary from GitHub Releases"
    echo "    https://github.com/PlayIdea-Lab/cq/releases/latest"
    echo ""
    exit 0
    ;;
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

if [ "$GOOS" = "darwin" ] && [ "$GOARCH" = "amd64" ]; then
  echo "Error: Intel Mac (darwin/amd64) is not supported."
  echo "CQ requires Apple Silicon (arm64). See: https://github.com/PlayIdea-Lab/cq/releases"
  if [ "$DRY_RUN" = "1" ]; then exit 0; else exit 1; fi
fi

ARTIFACT="cq-${GOOS}-${GOARCH}"
if [ -z "$VERSION" ]; then
  URL="https://github.com/PlayIdea-Lab/cq/releases/latest/download/${ARTIFACT}"
else
  API_URL="https://api.github.com/repos/PlayIdea-Lab/cq/releases/tags/${VERSION}"
  if ! curl -sf "$API_URL" > /dev/null 2>&1; then
    echo "Error: Version '${VERSION}' not found. See: https://github.com/PlayIdea-Lab/cq/releases" >&2
    exit 1
  fi
  URL="https://github.com/PlayIdea-Lab/cq/releases/download/${VERSION}/${ARTIFACT}"
fi

if [ "$DRY_RUN" = "1" ]; then
  echo "Would download: ${URL}"
  echo "Would install to: ${INSTALL_DIR}/cq"
  exit 0
fi

echo "Installing cq (${GOOS}/${GOARCH})..."

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

# Auto-add INSTALL_DIR to PATH in shell rc files
add_to_path() {
  LINE="export PATH=\"\$PATH:${INSTALL_DIR}\""
  ADDED=0
  for RC in "${HOME}/.zshrc" "${HOME}/.bashrc" "${HOME}/.profile"; do
    if [ -f "$RC" ]; then
      if ! grep -qF "${INSTALL_DIR}" "$RC" 2>/dev/null; then
        printf '\n# Added by cq installer\n%s\n' "$LINE" >> "$RC"
        echo "PATH updated: $RC"
        ADDED=1
      fi
    fi
  done
  if [ "$ADDED" = "0" ] && [ ! -f "${HOME}/.zshrc" ] && [ ! -f "${HOME}/.bashrc" ]; then
    printf '%s\n' "$LINE" > "${HOME}/.profile"
    echo "PATH updated: ${HOME}/.profile"
  fi
}

case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    add_to_path
    export PATH="$PATH:${INSTALL_DIR}"
    ;;
esac

# Auto-add shell completion to rc files
add_completion() {
  for RC_SHELL in "zsh:.zshrc" "bash:.bashrc"; do
    SHELL_NAME="${RC_SHELL%%:*}"
    RC_FILE="${HOME}/${RC_SHELL##*:}"
    if [ ! -f "$RC_FILE" ]; then
      continue
    fi
    MARKER="cq completion ${SHELL_NAME}"
    if grep -qF "$MARKER" "$RC_FILE" 2>/dev/null; then
      continue
    fi
    printf '\n# cq shell completion\neval "$(cq completion %s)"\n' "$SHELL_NAME" >> "$RC_FILE"
    echo "Shell completion added: $RC_FILE"
  done
}

add_completion

# Install Python sidecar (provides LSP and doc parsing features)
install_python_sidecar() {
  if ! command -v uv > /dev/null 2>&1; then
    echo ""
    echo "┌─ Python sidecar (LSP/doc features) requires uv ──────────────────┐"
    echo "│  Install uv: https://docs.astral.sh/uv/getting-started/installation/ │"
    echo "│  Then run:   cq doctor --fix                                     │"
    echo "└───────────────────────────────────────────────────────────────────┘"
    return
  fi
  if command -v c4-bridge > /dev/null 2>&1; then
    return
  fi
  echo "Installing c4-bridge (Python sidecar for LSP/doc features)..."
  uv tool install "git+https://github.com/PlayIdea-Lab/cq" --quiet 2>/dev/null || \
    echo "Warning: c4-bridge install skipped. Run manually: uv tool install git+https://github.com/PlayIdea-Lab/cq"
}

install_python_sidecar

if [ -x "${INSTALL_DIR}/cq" ]; then
  echo ""
  "${INSTALL_DIR}/cq" doctor --fix || true
fi

echo ""
echo "Done! cq is ready."
echo ""
echo "Next steps:"
echo "  cq claude     # Start with Claude Code"
echo "  cq setup      # Pair a Telegram bot (optional)"
echo "  cq doctor     # Check installation"
echo ""
echo "(Open a new terminal if commands are not found yet)"
