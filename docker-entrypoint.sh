#!/bin/sh
set -e

# Initialize git repo if not present — worktree operations require at least one commit.
if [ ! -d /data/repo/.git ]; then
  git init /data/repo
  git -C /data/repo config user.email "karakuri@local"
  git -C /data/repo config user.name "Karakuri"
  git -C /data/repo commit --allow-empty -m "karakuri: init"
fi

# Copy config to a writable path. Auth no longer needs patching in: the server
# reads KARAKURI_AUTH_JWT_SECRET and friends straight from the environment, so
# no secret is ever written to a file on disk.
cp /etc/karakuri/config.yaml /tmp/runtime.yaml

if [ -z "$KARAKURI_AUTH_JWT_SECRET" ]; then
  echo "karakuri: KARAKURI_AUTH_JWT_SECRET is not set — the server will refuse to start." >&2
  echo "karakuri: generate one with: openssl rand -base64 32" >&2
fi
if [ -z "$KARAKURI_AUTH_BOOTSTRAP_PASSWORD" ]; then
  echo "karakuri: KARAKURI_AUTH_BOOTSTRAP_PASSWORD is not set. This is only" >&2
  echo "karakuri: required on a database with no principals, where it becomes" >&2
  echo "karakuri: the first administrator's password." >&2
fi

export KARAKURI_CONFIG=/tmp/runtime.yaml
exec /usr/local/bin/karakuri "$@"
