# Security Policy

## Reporting a vulnerability

Please **do not** open public GitHub issues for security-sensitive bugs.

Report privately via GitHub's [private vulnerability reporting](https://github.com/bsenel/karakuri/security/advisories/new) form, or email the maintainer at the address listed in the repository profile. Include:

- A clear description of the issue and its impact.
- Steps to reproduce, ideally a minimal proof-of-concept.
- Affected versions (commit hash or tag).
- Suggested fix, if you have one.

You'll receive an initial response within 5 business days. A fix and coordinated disclosure timeline will be agreed in the reply.

## Supported versions

Karakuri is pre-1.0 and ships from `main`. Only the latest tagged release receives security fixes; older releases are not patched. Pin your deployment to a specific tag if you need stability across security updates.

## Scope

In scope:

- The main Karakuri server (`cmd/server`) and CLI (`cmd/krk`).
- Reusable Go modules published under `github.com/bsenel/karakuri/{auth,quota}` and their submodules (once shipped — Phases 14+).
- Helm chart under `deploy/`.
- Storage adapters, agent factory, tool adapters.

Out of scope:

- Third-party tools and adapters Karakuri integrates with (report upstream).
- LLM provider security (Anthropic, Google, etc. — report upstream).
- Issues that require physical access or an already-compromised operator workstation.
