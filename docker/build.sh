#!/bin/bash
# Build C4 Docker images
#
# Usage:
#   ./docker/build.sh           # Build all images
#   ./docker/build.sh workspace # Build workspace image only
#   ./docker/build.sh api       # Build API image only

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[BUILD]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

build_workspace() {
    log "Building c4-workspace image..."
    docker build \
        -t c4-workspace:latest \
        -f "$SCRIPT_DIR/Dockerfile" \
        "$PROJECT_ROOT"
    success "c4-workspace:latest built successfully"
}

build_api() {
    log "Building c4-api image..."
    docker build \
        -t c4-api:latest \
        -f "$SCRIPT_DIR/Dockerfile.api" \
        "$PROJECT_ROOT"
    success "c4-api:latest built successfully"
}

build_all() {
    build_workspace
    build_api
}

# Main
case "${1:-all}" in
    workspace)
        build_workspace
        ;;
    api)
        build_api
        ;;
    all)
        build_all
        ;;
    *)
        echo "Usage: $0 {workspace|api|all}"
        exit 1
        ;;
esac

log "Build complete!"
