# syntax=docker/dockerfile:1

# ── Build stage (Debian-based — reliable on WSL2 Docker) ─────────────────────
FROM golang:1.25-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /zettabridge ./cmd/api && \
    go build -ldflags="-s -w" -o /paper-adapter ./cmd/paper-adapter && \
    go build -ldflags="-s -w" -o /zerodha-adapter ./cmd/zerodha-adapter

# ── Runtime stage (Ubuntu — matches WSL host) ────────────────────────────────
FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates wget tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /zettabridge /zettabridge
COPY --from=builder /paper-adapter /app/paper-adapter
COPY --from=builder /zerodha-adapter /app/zerodha-adapter

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/zettabridge"]
