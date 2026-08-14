#!/usr/bin/env bash
# Enforce the architectural boundary from AGENTS.md and ADR 001: the
# github.com/tmc/langchaingo SDK may only be imported under internal/platform/.
# internal/core/ and internal/feature/ must stay vendor-free so the domain and
# use-case layers never depend on a concrete LLM SDK.
#
# AGENTS.md referenced this script as the enforcer of that rule, but the script
# did not exist (SECURITY_AUDIT.md / ENGINEERING_AUDIT.md F-08). golangci-lint's
# depguard rule (.golangci.yml) enforces the same boundary; this script is the
# standalone, dependency-free check for pre-commit hooks and CI.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

pattern='github.com/tmc/langchaingo'
violations=0

# Search the layers that must not import the SDK. Test files are excluded, in
# line with the depguard config.
while IFS= read -r -d '' file; do
  if grep -qE "\"${pattern}" "$file"; then
    echo "FORBIDDEN import of ${pattern} in ${file}" >&2
    violations=$((violations + 1))
  fi
done < <(find internal/core internal/feature -name '*.go' ! -name '*_test.go' -print0 2>/dev/null)

if [ "$violations" -gt 0 ]; then
  echo "" >&2
  echo "langchaingo must only be imported under internal/platform/ (AGENTS.md, ADR 001)." >&2
  exit 1
fi

echo "OK: no langchaingo imports in internal/core or internal/feature"
