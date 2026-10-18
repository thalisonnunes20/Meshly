# Multi-stage Dockerfile for Meshly Server

# Stage 1: Build the Go binary
FROM golang:1.23-bookworm AS builder

WORKDIR /app

# Copy dependency definitions
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o meshly-server ./cmd/server

# Stage 2: Minimal runtime image with WireGuard and iptables
FROM debian:bookworm-slim

# Install WireGuard tools, iproute2 and iptables
RUN apt-get update && apt-get install -y --no-install-recommends \
    wireguard-tools \
    iproute2 \
    iptables \
    ca-certificates \
    procps \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy compiled binary from builder
COPY --from=builder /app/meshly-server /usr/local/bin/meshly-server

# Configuration directory
RUN mkdir -p /etc/meshly

# Expose API and WireGuard UDP ports
EXPOSE 8080/tcp
EXPOSE 51820/udp

# Environment defaults
ENV MESHLY_API_PORT=8080
ENV MESHLY_WG_PORT=51820
ENV MESHLY_WG_INTERFACE=wg0
ENV MESHLY_DATA_DIR=/etc/meshly

CMD ["/usr/local/bin/meshly-server"]
