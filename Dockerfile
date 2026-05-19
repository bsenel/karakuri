# Stage 1: Build
FROM golang:1.23-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /karakuri ./cmd/server/ && \
    CGO_ENABLED=0 GOOS=linux go build -o /krk ./cmd/krk/

# Stage 2: Runtime
FROM alpine:3

RUN apk add --no-cache git ca-certificates

COPY --from=builder /karakuri /usr/local/bin/karakuri
COPY --from=builder /krk      /usr/local/bin/krk
COPY deploy/karakuri.yaml      /etc/karakuri/config.yaml
COPY docker-entrypoint.sh      /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
