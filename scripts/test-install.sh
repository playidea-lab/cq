#!/bin/bash
# CQ Installer Smoke Test — Docker-based Linux e2e verification
#
# Usage:
#   ./scripts/test-install.sh              # Build locally + test in Docker
#   ./scripts/test-install.sh --release    # Test from latest GitHub release
#   ./scripts/test-install.sh --release v1.1.0  # Test specific version
#
# Requires: Docker

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODE="local"
VERSION=""
IMAGE="ubuntu:24.04"
CONTAINER_NAME="cq-install-test-$$"
SMOKE_SCRIPT="/tmp/cq-smoke-test-$$.sh"

while [ $# -gt 0 ]; do
  case "$1" in
    --release)
      MODE="release"
      if [ "${2:-}" != "" ] && [[ ! "${2:-}" =~ ^-- ]]; then
        VERSION="$2"; shift
      fi
      ;;
    --image)  shift; IMAGE="$1" ;;
    -h|--help)
      echo "Usage: $0 [--release [VERSION]] [--image IMAGE]"
      exit 0 ;;
  esac
  shift
done

cleanup() {
  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
  rm -f "$SMOKE_SCRIPT" /tmp/cq-test-linux /tmp/c5-test-linux 2>/dev/null || true
}
trap cleanup EXIT

echo "=== CQ Installer Smoke Test ==="
echo "Mode: $MODE  Image: $IMAGE"
echo ""

# ── Step 1: Prepare binaries ────────────────────────────────────
if [ "$MODE" = "local" ]; then
  echo "▶ Building Linux binaries..."
  (cd "$PROJECT_ROOT/c4-core" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/cq-test-linux ./cmd/c4/)
  (cd "$PROJECT_ROOT/c5" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/c5-test-linux ./cmd/c5/)
  echo "  cq: $(file /tmp/cq-test-linux | grep -o 'ELF.*')"
  echo "  c5: $(file /tmp/c5-test-linux | grep -o 'ELF.*')"
fi

# ── Step 2: Generate smoke test script (in /tmp to avoid hook interference) ─
cat > "$SMOKE_SCRIPT" << 'SMOKE_EOF'
#!/bin/bash
set -euo pipefail
PASS=0; FAIL=0; WARN=0
check() { local n="$1"; shift; if "$@" >/tmp/co 2>&1; then echo "  ✅ $n"; PASS=$((PASS+1)); else echo "  ❌ $n"; cat /tmp/co|sed 's/^/     /'; FAIL=$((FAIL+1)); fi; }
check_warn() { local n="$1"; shift; if "$@" >/tmp/co 2>&1; then echo "  ✅ $n"; PASS=$((PASS+1)); else echo "  ⚠️  $n"; WARN=$((WARN+1)); fi; }

echo ""; echo "━━━ CQ Smoke Test ━━━"; echo ""

echo "▶ Binaries"
check "cq --version" cq --version
check "c5 --version" c5 --version

echo ""; echo "▶ Sidecar"
if command -v uv >/dev/null 2>&1; then
  check "uv" uv --version
  if [ -d /workspace/cq-src ]; then
    check_warn "c4-bridge install" uv tool install --quiet /workspace/cq-src
  fi
  export PATH="$HOME/.local/bin:$PATH"
  check_warn "c4-bridge" which c4-bridge
fi

echo ""; echo "▶ Init"
mkdir -p /workspace/tp && cd /workspace/tp
cq claude -y --no-serve >/tmp/init.txt 2>&1 || true
check ".c4 created" test -d .c4
check ".mcp.json" test -f .mcp.json

echo ""; echo "▶ Doctor (pre-serve)"
cq doctor 2>&1|tee /tmp/d1.txt
FC=$(grep -c '^\[FAIL' /tmp/d1.txt||true)
[ "$FC" -gt 0 ] && { echo "  ❌ $FC FAIL"; FAIL=$((FAIL+1)); } || { echo "  ✅ no FAIL"; PASS=$((PASS+1)); }

echo ""; echo "▶ Serve"
cq serve &
SP=$!
for i in $(seq 1 20); do [ -S .c4/tool.sock ] 2>/dev/null && break; sleep 0.5; done
check "socket" test -S .c4/tool.sock

echo ""; echo "▶ Doctor (post-serve)"
cq doctor 2>&1|tee /tmp/d2.txt
FC=$(grep -c '^\[FAIL' /tmp/d2.txt||true)
[ "$FC" -gt 0 ] && { echo "  ❌ $FC FAIL"; FAIL=$((FAIL+1)); } || { echo "  ✅ no FAIL"; PASS=$((PASS+1)); }
check "dr:binary" sh -c "grep 'cq binary' /tmp/d2.txt|grep -q '\[OK'"
check "dr:.c4" sh -c "grep '.c4 dir' /tmp/d2.txt|grep -q '\[OK'"
check "dr:socket" sh -c "grep 'tool-socket' /tmp/d2.txt|grep -q '\[OK'"

echo ""; echo "▶ MCP"
check "c4_status" cq tool c4_status --json

echo ""; echo "▶ Cleanup"
kill "$SP" 2>/dev/null||true; wait "$SP" 2>/dev/null||true

echo ""; echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  PASS=$PASS  FAIL=$FAIL  WARN=$WARN"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
[ "$FAIL" -gt 0 ] && exit 1 || exit 0
SMOKE_EOF
chmod +x "$SMOKE_SCRIPT"

# ── Step 3: Build Docker image ──────────────────────────────────
DOCKERFILE=$(cat <<'DKEOF'
ARG BASE_IMAGE=ubuntu:24.04
FROM ${BASE_IMAGE}
ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/root
ENV PATH="/root/.local/bin:${PATH}"
RUN apt-get update -qq && apt-get install -y -qq \
    curl git ca-certificates sqlite3 \
    > /dev/null 2>&1 && rm -rf /var/lib/apt/lists/*
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
WORKDIR /workspace
DKEOF
)

echo ""
echo "▶ Building Docker image..."
echo "$DOCKERFILE" | docker build \
  --build-arg BASE_IMAGE="$IMAGE" \
  -f - \
  -t cq-install-test \
  "$PROJECT_ROOT" 2>&1 | tail -3

# ── Step 4: Run container ───────────────────────────────────────
echo ""
echo "▶ Running smoke test in Docker ($IMAGE)..."
echo ""

if [ "$MODE" = "local" ]; then
  docker run --name "$CONTAINER_NAME" \
    -v /tmp/cq-test-linux:/root/.local/bin/cq:ro \
    -v /tmp/c5-test-linux:/root/.local/bin/c5:ro \
    -v "$PROJECT_ROOT":/workspace/cq-src:ro \
    -v "$SMOKE_SCRIPT":/workspace/smoke-test.sh:ro \
    cq-install-test \
    bash /workspace/smoke-test.sh
else
  # Release mode: run install.sh inside container, then smoke test
  VERSION_FLAG=""
  [ -n "$VERSION" ] && VERSION_FLAG="--version $VERSION"
  docker run --name "$CONTAINER_NAME" \
    -v "$SMOKE_SCRIPT":/workspace/smoke-test.sh:ro \
    cq-install-test \
    bash -c "sh /workspace/install.sh --tier full $VERSION_FLAG && bash /workspace/smoke-test.sh"
fi

echo ""
echo "=== Smoke test complete ==="
