# C4 Workspace Base Image
#
# This image provides an isolated development environment for C4 workers.
# Each user gets a separate container with their git repository cloned.
#
# Build: docker build -t c4-workspace:latest -f docker/Dockerfile .
# Run: docker run -e GIT_URL=https://github.com/user/repo c4-workspace:latest

FROM python:3.12-slim

LABEL maintainer="C4 Team"
LABEL description="C4 Workspace - Isolated development environment"
LABEL version="1.0.0"

# Environment variables
ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    # UV settings
    UV_SYSTEM_PYTHON=1 \
    UV_NO_CACHE=1 \
    # Workspace settings
    WORKSPACE_DIR=/workspace \
    GIT_URL="" \
    GIT_BRANCH="main"

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    build-essential \
    ca-certificates \
    openssh-client \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Install uv (fast Python package manager)
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
    && mv /root/.local/bin/uv /usr/local/bin/uv \
    && mv /root/.local/bin/uvx /usr/local/bin/uvx \
    && chmod +x /usr/local/bin/uv /usr/local/bin/uvx

# Create workspace directory
RUN mkdir -p ${WORKSPACE_DIR} \
    && chmod 755 ${WORKSPACE_DIR}

# Copy entrypoint script
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set working directory
WORKDIR ${WORKSPACE_DIR}

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD [ -d "${WORKSPACE_DIR}/.git" ] || exit 1

# Default entrypoint
ENTRYPOINT ["/entrypoint.sh"]

# Default command (keep container running)
CMD ["sleep", "infinity"]
