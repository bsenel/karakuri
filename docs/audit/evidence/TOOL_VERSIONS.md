# Tool versions & raw output index

Captured 2026-08-13 on the CI/audit container (linux/amd64).

| Tool | Version | Scope | Raw output |
|------|---------|-------|-----------|
| Go toolchain | go1.25.0 (module), go1.25.12 (installed) | build, vet, test | — |
| govulncheck | latest (golang.org/x/vuln) | Go stdlib+deps reachability | `govulncheck.json` |
| gosec | v2 latest (securego) | Go SAST | `gosec.json` |
| semgrep | 1.172.0 | p/golang, p/security-audit, p/secrets, p/typescript | `semgrep.json` |
| golangci-lint | 2.5.0 | Go lint (config broken pre-fix — see F-08) | — |
| gitleaks | v8 latest | secret scan (history + tree) | `gitleaks-history.json` (0 findings) |
| Trivy | 0.58.1 (docker image) | IaC misconfig + secret | `trivy-config.json` |
| Checkov | 3.3.10 | Dockerfile IaC | `checkov-dockerfile.json` |
| Syft | latest (anchore) | SBOM (CycloneDX + SPDX) | `sbom.cdx.json`, `sbom.spdx.json` |
| npm audit | npm 10.9.7 | JS dependency SCA | `npm-audit.json` |

## Method notes

- Trivy ran with `--network host` and `SSL_CERT_FILE=/root/.ccr/ca-bundle.crt` so its
  DB and registry pulls verify through the agent proxy.
- Checkov's Helm scan requires `helm template` rendering first; `helm` is not
  installed on this container, so the Kubernetes misconfigurations were taken from
  Trivy's config scanner (20 findings against `deploy/templates/`), which reads the
  chart templates directly. Both agree on the securityContext gaps.
- The 28 govulncheck hits are all Go **standard-library** advisories against the
  exact toolchain pinned in the shipped Docker image (go1.25.0). CI builds float to
  the latest 1.25.x, so they do not affect the CI-built binary — only the artifact.
  See F-06.
