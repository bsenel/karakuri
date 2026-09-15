# Stage 1: Build
# Pinned by digest to golang:1.26-bookworm (go1.26.8), at or above the
# `toolchain go1.26.8` line in go.mod so this stage builds with the image's own
# Go and never downloads one.
#
# The pin still matters, but it is no longer the only thing standing between us
# and F-06: a bare `go 1.26.0` line makes GOTOOLCHAIN=auto fetch exactly
# go1.26.0, whose stdlib advisories are fixed in 1.26.1 through 1.26.3. The
# toolchain line in go.mod holds that floor for every build; this digest holds
# it for the shipped image. Bump both together.
FROM golang:1.26-bookworm@sha256:9fdc884aacc3bec89b20ffc69f4bb369c78210e3e4f600387b5128b12c199f81 AS builder

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
