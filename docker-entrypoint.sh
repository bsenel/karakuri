#!/bin/sh
set -e

# Initialize git repo if not present — worktree operations require at least one commit.
if [ ! -d /data/repo/.git ]; then
  git init /data/repo
  git -C /data/repo config user.email "karakuri@local"
  git -C /data/repo config user.name "Karakuri"
  git -C /data/repo commit --allow-empty -m "karakuri: init"
fi

# Copy config to a writable path and optionally patch auth token.
cp /etc/karakuri/config.yaml /tmp/runtime.yaml
if [ -n "$KARAKURI_AUTH_TOKEN" ]; then
  sed -i "s/token: \"\"/token: \"$KARAKURI_AUTH_TOKEN\"/" /tmp/runtime.yaml
fi

export KARAKURI_CONFIG=/tmp/runtime.yaml
exec /usr/local/bin/karakuri "$@"
