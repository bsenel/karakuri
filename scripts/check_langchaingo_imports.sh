#!/usr/bin/env bash
set -euo pipefail
violations=$(grep -r "github.com/tmc/langchaingo" --include="*.go" . \
  | grep -v "^\./internal/platform/" || true)
if [ -n "$violations" ]; then
  echo "LangChain Go imports outside internal/platform/:"
  echo "$violations"
  exit 1
fi
echo "OK: langchaingo confined to internal/platform/"
