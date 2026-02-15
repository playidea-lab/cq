# C5 Hub — Distributed Job Queue Server
# Build context: c5/ directory (not repo root)
#
# Build:  docker build -t c5-hub .
# Run:    docker run -p 8585:8585 -v c5-data:/data c5-hub

# ─── Build Stage ──────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

# go.mod requires go 1.25.7; let toolchain auto-download if needed
ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /c5 ./cmd/c5/

# ─── Runtime Stage ────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 c5user

COPY --from=builder /c5 /usr/local/bin/c5

RUN mkdir -p /data && chown c5user:c5user /data

ENV C5_PORT=8585 \
    C5_DATA_DIR=/data \
    C5_API_KEY=""

EXPOSE 8585
VOLUME ["/data"]

USER c5user

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -qO- http://localhost:${C5_PORT}/v1/health || exit 1

ENTRYPOINT ["c5"]
CMD ["serve", "--port", "8585", "--db", "/data/c5.db"]
