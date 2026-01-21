#!/bin/bash
# Test C4 Docker images
#
# Usage:
#   ./docker/test.sh           # Run all tests
#   ./docker/test.sh workspace # Test workspace image only
#   ./docker/test.sh api       # Test API image only

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    exit 1
}

test_workspace_image() {
    log "Testing c4-workspace image..."

    # Test 1: Image exists
    if ! docker image inspect c4-workspace:latest &>/dev/null; then
        fail "Image c4-workspace:latest not found. Run ./docker/build.sh first"
    fi
    success "Image exists"

    # Test 2: Container can start
    local container_id
    container_id=$(docker run -d \
        -e GIT_URL=https://github.com/anthropics/anthropic-cookbook \
        -e GIT_BRANCH=main \
        -e WORKSPACE_ID=test-workspace \
        c4-workspace:latest)

    # Wait for container to initialize
    sleep 5

    # Test 3: Git repository cloned
    if ! docker exec "$container_id" test -d /workspace/.git; then
        docker rm -f "$container_id" &>/dev/null
        fail "Git repository not cloned"
    fi
    success "Git repository cloned"

    # Test 4: uv available
    if ! docker exec "$container_id" uv --version &>/dev/null; then
        docker rm -f "$container_id" &>/dev/null
        fail "uv not available"
    fi
    success "uv installed"

    # Test 5: Python available
    if ! docker exec "$container_id" python --version &>/dev/null; then
        docker rm -f "$container_id" &>/dev/null
        fail "Python not available"
    fi
    success "Python available"

    # Test 6: Workspace info file created
    if ! docker exec "$container_id" test -f /workspace/.c4-workspace; then
        docker rm -f "$container_id" &>/dev/null
        fail "Workspace info file not created"
    fi
    success "Workspace info file created"

    # Cleanup
    docker rm -f "$container_id" &>/dev/null
    success "c4-workspace image tests passed"
}

test_api_image() {
    log "Testing c4-api image..."

    # Test 1: Image exists
    if ! docker image inspect c4-api:latest &>/dev/null; then
        fail "Image c4-api:latest not found. Run ./docker/build.sh first"
    fi
    success "Image exists"

    # Test 2: Container can start
    local container_id
    container_id=$(docker run -d -p 8001:8000 c4-api:latest)

    # Wait for server to start
    sleep 10

    # Test 3: Health endpoint responds
    if ! curl -s -f http://localhost:8001/health &>/dev/null; then
        docker logs "$container_id"
        docker rm -f "$container_id" &>/dev/null
        fail "Health endpoint not responding"
    fi
    success "Health endpoint responding"

    # Test 4: API documentation accessible
    if ! curl -s -f http://localhost:8001/docs &>/dev/null; then
        docker rm -f "$container_id" &>/dev/null
        fail "API docs not accessible"
    fi
    success "API docs accessible"

    # Cleanup
    docker rm -f "$container_id" &>/dev/null
    success "c4-api image tests passed"
}

test_all() {
    test_workspace_image
    echo ""
    test_api_image
}

# Main
case "${1:-all}" in
    workspace)
        test_workspace_image
        ;;
    api)
        test_api_image
        ;;
    all)
        test_all
        ;;
    *)
        echo "Usage: $0 {workspace|api|all}"
        exit 1
        ;;
esac

echo ""
log "All tests passed!"
