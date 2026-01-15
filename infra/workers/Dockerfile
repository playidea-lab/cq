# C4 Cloud Worker - Docker Image
# Runs isolated task execution in cloud environment

FROM python:3.12-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install uv for fast Python package management
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.local/bin:$PATH"

# Create non-root user for security
RUN useradd -m -s /bin/bash worker
WORKDIR /home/worker

# Copy C4 package
COPY --chown=worker:worker . /home/worker/c4/

# Install C4 and dependencies
WORKDIR /home/worker/c4
RUN uv pip install --system -e . && \
    rm -rf /root/.cache

# Create workspace directory
RUN mkdir -p /home/worker/workspace && \
    chown -R worker:worker /home/worker/workspace

# Switch to non-root user
USER worker
WORKDIR /home/worker/workspace

# Environment variables
ENV C4_CLOUD_WORKER=1
ENV C4_WORKER_MODE=remote
ENV PYTHONUNBUFFERED=1

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import c4; print('healthy')" || exit 1

# Default command - worker process
CMD ["python", "-m", "c4.cloud.worker"]
