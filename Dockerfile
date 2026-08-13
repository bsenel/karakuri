# Stage 1: Build
# Pinned by digest to golang:1.25-bookworm (go1.25.12), which matches the
# `go 1.25.0` toolchain in go.mod. The previous golang:1.23 base made
# GOTOOLCHAIN=auto download and pin exactly go1.25.0 — the patch govulncheck
# flags for 28 stdlib advisories. See SECURITY_AUDIT.md F-06.
FROM golang:1.25-bookworm@sha256:6359592445455f2dbe2412bed411336035bc019a50017720d77454ffdd6d0f82 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# The Go binary embeds web/dist via //go:embed (web/embed.go). The build context
# must already contain a built frontend (`make web-build`, or the CI frontend
# job). CGO is off so the binary is static and runs on the minimal runtime below.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /karakuri ./cmd/server/ && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -o /krk ./cmd/krk/

# Stage 2: Runtime
# Pinned by digest. alpine:3.21.
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

RUN apk add --no-cache git ca-certificates wget

# Run as a non-root user (SECURITY_AUDIT.md F-06). Creating /data with this
# ownership in the image means a fresh Docker named volume inherits it, so the
# entrypoint can `git init /data/repo` without being root; in Kubernetes the
# chart's fsGroup handles the same for the PVC.
RUN addgroup -g 65532 -S karakuri && \
    adduser -u 65532 -S -G karakuri -h /home/karakuri karakuri && \
    mkdir -p /data && chown -R karakuri:karakuri /data

COPY --from=builder /karakuri /usr/local/bin/karakuri
COPY --from=builder /krk      /usr/local/bin/krk
COPY deploy/karakuri.yaml      /etc/karakuri/config.yaml
COPY docker-entrypoint.sh      /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER karakuri:karakuri

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/v1/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
