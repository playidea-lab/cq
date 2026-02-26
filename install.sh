#!/bin/sh
# CQ installer — POSIX sh compatible
# Usage: sh install.sh --tier solo
#        sh install.sh --dry-run --tier connected

set -e

TIER="solo"
DRY_RUN=0
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

while [ $# -gt 0 ]; do
  case "$1" in
    --tier)      shift; TIER="$1" ;;
    --tier=*)    TIER="${1#--tier=}" ;;
    --dry-run)   DRY_RUN=1 ;;
    --install-dir) shift; INSTALL_DIR="$1" ;;
    --install-dir=*) INSTALL_DIR="${1#--install-dir=}" ;;
    --version) shift; VERSION="$1" ;;
    --version=*) VERSION="${1#--version=}" ;;
    -h|--help)
      echo "Usage: install.sh [--tier solo|connected|full] [--dry-run] [--version vX.Y.Z]"
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

if [ "$GOOS" = "darwin" ] && [ "$GOARCH" = "amd64" ]; then
  echo "Error: Intel Mac (darwin/amd64) is not supported."
  echo "CQ requires Apple Silicon (arm64). See: https://github.com/PlayIdea-Lab/cq/releases"
  if [ "$DRY_RUN" = "1" ]; then exit 0; else exit 1; fi
fi

ARTIFACT="cq-${TIER}-${GOOS}-${GOARCH}"
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
      continue # already set up
    fi
    printf '\n# cq shell completion\neval "$(cq completion %s)"\n' "$SHELL_NAME" >> "$RC_FILE"
    echo "Shell completion added: $RC_FILE"
  done
}

add_completion
# fish shell requires manual setup (eval-based sourcing differs from bash/zsh):
#   Add to ~/.config/fish/config.fish:  cq completion fish | source

if [ -x "${INSTALL_DIR}/cq" ]; then
  echo ""
  "${INSTALL_DIR}/cq" doctor || true
fi

echo ""
echo "Done! cq is ready to use."
echo "(Open a new terminal if 'cq' is not found yet)"
