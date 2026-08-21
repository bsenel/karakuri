# Karakuri — CI Security Pipeline

**Date:** 2026-08-13 · **Platform:** GitHub Actions · **Baseline:** `ec5a795`
**Refs:** [GitHub Actions](https://docs.github.com/en/actions) · [Security hardening for Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions) · [OWASP DevSecOps Guideline](https://owasp.org/www-project-devsecops-guideline/) · [OWASP Top 10 CI/CD Risks](https://owasp.org/www-project-top-10-ci-cd-security-risks/) · [SARIF](https://sarifweb.azurewebsites.net/) · [Sigstore/cosign](https://docs.sigstore.dev/)

## Approach

Karakuri already had a **good** CI baseline — `ci.yml` (build, vet, test, a 9-module
coverage matrix, a real-Keycloak identity job, a Playwright e2e job), `codeql.yml` (Go +
JS SAST, weekly), three release workflows, and Dependabot across gomod/npm/actions. The
strategy here is to **extend, not replace**: add the missing DevSecOps gates (SCA, secret,
IaC, lint, SBOM, container scan) as a new `security-scan.yml`, wire in the pre-commit stage,
and route findings to the code-scanning dashboard as SARIF.

Two principles govern the rollout:

1. **Baseline existing findings so day one does not block all work.** The baseline is
   report-only; new findings are not. gosec's job never fails, but the SARIF it uploads
   becomes a `gosec` code-scanning check that **does** fail a pull request introducing new
   alerts in the code it changed — so the pre-existing ~35 findings stay off everyone's
   way while freshly written code is still gated. The license check is report-only outright
   (no SARIF, so no second check). Blocking gates (govulncheck, gitleaks, trivy
   HIGH/CRITICAL, golangci-lint) are ones the codebase already passes clean after this
   branch's fixes. Allowlists (`.trivyignore`) carry a reason per entry.
2. **Least privilege + speed.** Default `permissions: contents: read`; only SARIF-upload
   jobs widen to `security-events: write`. Jobs run in parallel with caching, keeping the
   added critical-path time small (the new jobs run alongside the existing `ci.yml` jobs,
   not after them).

## Delivered files

| File | Purpose |
|------|---------|
| `.github/workflows/security-scan.yml` | The DevSecOps gate set (validated with `actionlint`, clean) |
| `.pre-commit-config.yaml` | Local pre-commit stage: hygiene, gitleaks, gofmt/vet, langchaingo boundary |
| `.trivyignore` | IaC allowlist with per-entry reasons (baseline) |
| `.golangci.yml` | Repaired to v2 (F-08) so the lint gate can run at all |
| `scripts/check_langchaingo_imports.sh` | Architectural-boundary gate, for pre-commit + CI |

## Pipeline stages

**pre-commit (local):** trailing-whitespace/EOF/large-file/private-key checks, **gitleaks**,
`gofmt`, `go vet`, and the **langchaingo import-boundary** script. Fast, so committing stays
quick; heavy scans are left to CI.

**PR (`security-scan.yml` on pull_request):** golangci-lint (root + auth + quota),
gosec (baseline report-only; new findings block via code scanning), govulncheck
(blocking), npm audit (prod deps, blocking),
gitleaks (blocking), Trivy fs+config+secret (SARIF, blocks HIGH/CRITICAL), license check
(report-only) — alongside the existing `ci.yml` build/test/coverage and `codeql.yml` SAST.

**merge to main (`security-scan.yml` on push):** all of the above **plus** Syft SBOM
(CycloneDX, uploaded as a 90-day artifact). Container image scan and image signing are the
next increment (see "Not yet wired").

**nightly/weekly:** `security-scan.yml` `schedule` (Mon 07:00 UTC) re-runs the full set
against newly published advisories; `codeql.yml` already runs its weekly deep SAST.

**release:** the existing release workflows tag and publish; SBOM attach + provenance
attestation + `cosign` signing are the recommended next increment.

## Matrix E — CI quality gate matrix

| Gate | Stage | Tool | Trigger | Threshold | On failure | Bypass path | Runtime budget | Owner |
|------|-------|------|---------|-----------|-----------|-------------|---------------|-------|
| Secret (local) | pre-commit | gitleaks | every commit | any secret | commit blocked | `--no-verify` (discouraged) | <5 s | dev |
| Format/vet/boundary | pre-commit | gofmt, go vet, script | every commit | any diff/violation | commit blocked | `--no-verify` | <10 s | dev |
| Lint | PR + push | golangci-lint v2 | PR/push | any of 4 linters | **PR blocked** | fix, or config change w/ review | ~2 min | maintainers |
| Go SAST | PR + push | gosec | PR/push | new alerts in changed code | **PR blocked** (baseline exempt) | fix, or `#nosec Gxxx -- reason` | ~2 min | security |
| Deep SAST | PR + weekly | CodeQL | PR/push/cron | security-and-quality | **PR blocked** (existing) | dismiss w/ justification | ~6 min | security |
| Go SCA | PR + push | govulncheck | PR/push/cron | any reachable vuln | **PR blocked** | update dep, or documented exception | ~3 min | maintainers |
| npm SCA | PR + push | npm audit (prod) | PR/push | high/critical (prod) | **PR blocked** | bump, or roadmap-tracked exception | ~1 min | frontend |
| Secret (CI) | PR + push | gitleaks | PR/push | any secret | **PR blocked** | `.gitleaksignore` w/ reason | ~1 min | security |
| IaC + FS | PR + push | Trivy | PR/push/cron | HIGH/CRITICAL | **PR blocked** | `.trivyignore` w/ reason | ~2 min | maintainers |
| License | PR | go-licenses | PR | report-only | warning | n/a | ~1 min | maintainers |
| SBOM | push + weekly | Syft (CycloneDX) | push/cron | artifact produced | job fails | n/a | ~1 min | maintainers |
| Coverage | PR (existing) | go test matrix | PR/push | 90–95% per module | **PR blocked** (existing) | raise tests | ~5 min | maintainers |
| Build+test+e2e | PR (existing) | ci.yml | PR/push | green | **PR blocked** (existing) | fix | ~10 min | maintainers |

**Total added PR critical-path time:** the security jobs run in parallel with the existing
`ci.yml` jobs; the longest new job (~3 min govulncheck incl. frontend build) is well inside
the existing e2e job's ~10 min, so **the PR wall-clock stays within the ≤15 min budget**.

## Baseline / allowlist handling

- **gosec** — the baseline is exempt, new findings are not. The `SAST (gosec)` job is
  `continue-on-error` and always green; the `gosec` code-scanning check built from its
  SARIF fails on alerts new to the diff. Verified false positives are documented in
  `SECURITY_AUDIT.md` Appendix C. Being *in* a class Appendix C pre-clears does not exempt
  a new finding — the criterion is new-in-this-diff, not member-of-a-triaged-class. Fix it,
  or annotate with `#nosec <rule> -- <reason>`; gosec reads `#nosec` and does **not** read
  golangci-lint's `//nolint:gosec`.
- **licenses** — report-only in full: the step is `continue-on-error` and uploads no SARIF,
  so nothing blocks on it.
- **semgrep** — planned, never implemented. No workflow defines a semgrep job, so the rules
  it was to enforce are enforced by nothing, and Appendix C's 13 semgrep findings did not
  come from this pipeline. Either wire it up or stop listing it as a gate; a gate named in
  the docs and absent from CI is worse than no gate, because it is budgeted for.
- **Trivy** — `.trivyignore` baselines the generic-chart IaC policy noise (registry-domain
  and `:latest` rules) with a reason per entry; the securityContext HIGH findings are already
  fixed (F-07), so those are **not** allowlisted.
- **npm** — prod-only (`--omit=dev`); the dev-toolchain criticals (vite/vitest/esbuild) are
  never shipped and are tracked as a major-version migration in `REMEDIATION_ROADMAP.md`.
- **errcheck** — deliberately not enabled yet (45 pre-existing findings); its baselined
  rollout is `REFACTOR_REPORT.md` D8.

## Time-boxed exception process (for P0-blocking gates)

When a blocking gate must be bypassed to ship, the exception is explicit and dated:
1. Open an issue titled `SECURITY EXCEPTION: <gate> — <finding>` with the finding, the risk
   acceptance, the owner, and an **expiry date (≤30 days)**.
2. Record the allowlist entry (`.trivyignore` / `.gitleaksignore` / `#nosec`) referencing the
   issue number.
3. CI re-checks on expiry; an expired exception fails the build until renewed or resolved.

## Not yet wired (next increment)

- **Container image scan** — `trivy image` against the built image on `push` (needs the image
  built/loaded in CI; the release workflow is the natural home).
- **Image signing + provenance** — `cosign` keyless (OIDC) signing and SLSA build provenance
  on release; SBOM attestation attached to the image (SSDF PS.2, CRA).
- **DAST** — ZAP baseline against an ephemeral environment on `push` (the pentest already
  runs it locally; the ephemeral-env plumbing is the missing piece).
- **Digest-pinning** of all actions repo-wide (currently major-version tags + Dependabot,
  consistent with the existing workflows).

These are scoped into `REMEDIATION_ROADMAP.md` rather than half-wired here.
