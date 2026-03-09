# GPU Worker — C5 Hub 워커 컨테이너
# PyTorch + CUDA + cq CLI 포함
#
# Build:  docker build -t cq-gpu-worker .
# Run:    docker run --gpus all -e C5_HUB_URL=https://... -e C5_API_KEY=... cq-gpu-worker
#
# JWT 인증 (권장):
#   docker run --gpus all -e C5_HUB_URL=https://... \
#     -v ~/.c4/session.json:/home/worker/.c4/session.json:ro \
#     cq-gpu-worker

FROM nvidia/cuda:12.8.1-runtime-ubuntu24.04

# 비대화형 설치
ENV DEBIAN_FRONTEND=noninteractive

# 기본 패키지
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv \
    curl wget git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# PyTorch (CUDA 12.8)
RUN pip3 install --no-cache-dir --break-system-packages \
    torch torchvision torchaudio \
    numpy scipy scikit-learn pandas matplotlib

# cq 바이너리 설치
RUN curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash \
    || echo "WARN: install.sh failed, cq binary must be mounted"

# 워커 유저 + 작업 디렉토리
RUN useradd -m -u 1000 worker
WORKDIR /home/worker

# GPU 워커 스크립트 + caps 복사
COPY gpu-caps.yaml /home/worker/gpu-caps.yaml
COPY scripts/ /home/worker/scripts/
RUN chmod +x /home/worker/scripts/*.sh && chown -R worker:worker /home/worker

USER worker

# 기본 환경변수
ENV C5_HUB_URL="" \
    C5_API_KEY="" \
    PATH="/home/worker/.local/bin:${PATH}"

ENTRYPOINT ["cq", "hub", "worker", "start"]
