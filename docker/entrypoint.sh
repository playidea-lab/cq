#!/bin/bash
# C4 Workspace Entrypoint
#
# This script initializes the workspace by cloning the git repository
# and setting up the development environment.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Validate required environment variables
validate_env() {
    if [ -z "$GIT_URL" ]; then
        log_error "GIT_URL environment variable is required"
        exit 1
    fi
}

# Clone or update the repository
setup_repository() {
    local git_url="$GIT_URL"
    local git_branch="${GIT_BRANCH:-main}"

    log_info "Setting up repository: $git_url (branch: $git_branch)"

    if [ -d "$WORKSPACE_DIR/.git" ]; then
        log_info "Repository already exists, updating..."
        cd "$WORKSPACE_DIR"
        git fetch origin
        git checkout "$git_branch"
        git reset --hard "origin/$git_branch"
    else
        log_info "Cloning repository..."
        git clone --branch "$git_branch" --single-branch "$git_url" "$WORKSPACE_DIR"
    fi

    log_info "Repository setup complete"
}

# Setup Python environment if pyproject.toml exists
setup_python_env() {
    if [ -f "$WORKSPACE_DIR/pyproject.toml" ]; then
        log_info "Found pyproject.toml, setting up Python environment..."
        cd "$WORKSPACE_DIR"

        # Use uv to sync dependencies
        if command -v uv &> /dev/null; then
            log_info "Installing dependencies with uv..."
            uv sync --frozen 2>/dev/null || uv sync
        else
            log_warn "uv not found, skipping dependency installation"
        fi
    else
        log_info "No pyproject.toml found, skipping Python environment setup"
    fi
}

# Create workspace info file
create_info_file() {
    local info_file="$WORKSPACE_DIR/.c4-workspace"

    cat > "$info_file" << EOF
# C4 Workspace Info
# Generated at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

WORKSPACE_ID=${WORKSPACE_ID:-unknown}
GIT_URL=${GIT_URL}
GIT_BRANCH=${GIT_BRANCH:-main}
CREATED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

    log_info "Workspace info written to $info_file"
}

# Main entrypoint logic
main() {
    log_info "Starting C4 Workspace initialization..."
    log_info "Workspace ID: ${WORKSPACE_ID:-not set}"

    # Validate environment
    validate_env

    # Setup repository
    setup_repository

    # Setup Python environment
    setup_python_env

    # Create info file
    create_info_file

    log_info "C4 Workspace ready!"

    # Execute the command passed to the container
    exec "$@"
}

# Run main function
main "$@"
