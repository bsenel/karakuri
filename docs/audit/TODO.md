# Audit & Hardening To-Do

Programme master checklist. Statuses: `[ ]` pending · `[~]` in progress · `[x]` done · `[!]` blocked · `[-]` descoped.
Exactly **one** `[~]` at a time. An item becomes `[x]` only when it links to evidence (file, commit SHA, or report section).

| # | Workstream | Owner | Status | Evidence | Updated |
|---|-----------|-------|--------|----------|---------|
| 0 | Recon & baseline | agent | [x] | Phase 0 report in plan; `/tmp/testbaseline.log`; findings F-01…F-25 | 2026-08-13 |
| 1 | Code cleanup & refactoring (descoped) | agent | [x] | `REFACTOR_REPORT.md`; commit 28ed499 | 2026-08-13 |
| 2 | Security audit → SECURITY_AUDIT.md | agent | [x] | `SECURITY_AUDIT.md`; `evidence/` | 2026-08-13 |
| 3 | Compliance audit → COMPLIANCE_AUDIT.md | agent | [x] | `COMPLIANCE_AUDIT.md` | 2026-08-13 |
| 4 | Design & dev best-practices audit → ENGINEERING_AUDIT.md | agent | [x] | `ENGINEERING_AUDIT.md`; `adr/013`, `adr/014` | 2026-08-13 |
| 5 | Penetration testing → PENTEST_REPORT.md | agent | [x] | `PENTEST_REPORT.md`; `evidence/pentest-transcript.txt`, `zap-baseline.json` | 2026-08-13 |
| 6 | CI automation of 2–5 → CI_SECURITY_PIPELINE.md | agent | [x] | `CI_SECURITY_PIPELINE.md`, `security-scan.yml`, `REMEDIATION_ROADMAP.md` | 2026-08-13 |

## Workstream 0 — Recon & baseline (done)

- [x] Map repo, stack, modules — Phase 0 stack table. `2026-08-13`
- [x] Baseline build/test/coverage — `go build`+`go test` green all 10 modules (`/tmp/testbaseline.log`); web build green. `2026-08-13`
- [x] Baseline scans — govulncheck (28), npm audit (7), semgrep (13 FP), gitleaks (0). `2026-08-13`
- [x] Confirm exploit chains F-01, F-16 end-to-end by reading source. `2026-08-13`

## Workstream 2 — Security audit (done)

- [x] Install trivy, syft, checkov, gosec; capture versions → `evidence/TOOL_VERSIONS.md`.
- [x] Generate CycloneDX + SPDX SBOM (349 components) → `evidence/sbom.*.json`.
- [x] Trivy config/secret scan (20 misconfigs) → `evidence/trivy-config.json`.
- [x] gosec (35) + govulncheck (28) + semgrep (13 FP) raw output → `evidence/`.
- [x] Checkov on Dockerfile → `evidence/checkov-dockerfile.json`.
- [x] Write `SECURITY_AUDIT.md`: 25 findings, Matrix A/B/C, roadmap, residual risk, 3 appendices.

## Workstream 5 — Penetration testing (done)

- [x] Ran server binary on loopback (127.0.0.1:8899) with throwaway secrets. Container
      build path is itself F-06, so the binary was used to isolate app behaviour.
- [x] F-01 CONFIRMED (dir created outside root); F-03 CONFIRMED (30 logins, 0×429);
      F-16 CONFIRMED (/auth/refresh takes bare token); **F-02 reframed** — gate fails
      closed for scoped principals, downgraded High→Medium. `PENTEST_REPORT.md` PT-01..05.
- [x] ZAP baseline (57 PASS / 4 WARN, all F-11 headers) → `evidence/zap-baseline.json`.
- [x] Wrote `PENTEST_REPORT.md`; retest notes reflect the P0 fixes (applied in WS-FIX).

## Workstream FIX — Remediate P0/P1 (separate commits, all P0 done + verified)

- [x] F-01 — worktree containment check + component validation (`03502df`); regression test + pentest PT-01.
- [-] F-02 — **reframed** to design change (P1) after pentest disproved the live IDOR; in roadmap.
- [x] F-03 — per-IP rate limiter on `/auth/*` (`117a580`); verified live 26/30 → 429.
- [x] F-04/05/11 — http.Server timeouts, MaxBytesReader, security headers (`fef55e2`); 4 headers live.
- [x] F-16 — encrypt CLI handoff code, refresh token no longer recoverable (`befab80`); sealer tests.
- [x] F-10 — approver from authenticated principal (`a60a1f5`).
- [x] F-20 — login timing dummy-verify (`5e772f9`).
- [x] F-06 — Dockerfile patched Go/non-root/digest-pinned (`4af2ca1`); builder + non-root validated.
- [x] F-07 — Helm pod+container securityContext (`4af2ca1`); Trivy 3 HIGH cleared.
- [x] F-13/14 — react-router v7, prod source maps off (`ad5cbac`); build+23 unit+5 e2e green.
- [x] F-08 — lint repair (done in WS1, `28ed499`).
- [-] F-09, F-17, F-18, F-19, F-21..F-25, F-15, F-12 — documented in `REMEDIATION_ROADMAP.md` (P1/P2).

Validation: full `go test ./...` green across all 10 modules; Playwright e2e 5/5 green
(CSP + react-router v7 confirmed in a real browser).

## Workstream 1 — Refactor (done, descoped to repairs + enablers)

- [x] F-08 — `.golangci.yml` migrated to v2; loads and runs (0 issues root/auth/quota).
- [x] F-08 — added `scripts/check_langchaingo_imports.sh`.
- [x] Fixed 7 real lint issues surfaced; shared authz helper deferred to F-02 impl (YAGNI).
- [x] Wrote `REFACTOR_REPORT.md` with Matrix D (rejected candidates + rationale).

## Workstream 4 — Engineering audit (done)

- [x] Wrote `ENGINEERING_AUDIT.md`: Matrix A (68.6, Developing), findings E-01..E-06, 30/60/90 roadmap.
- [x] ADR-013 (versioned migrations), ADR-014 (error envelope + correlation IDs).

## Workstream 3 — Compliance audit (done)

- [x] Data-flow inventory (GDPR Art. 30 basis) with processor/cross-border flags.
- [x] Wrote `COMPLIANCE_AUDIT.md`: SOC 2 / SSDF / GDPR / ISO 27001 / EU CRA, [Code] vs [Org]
      separation, cross-mapping table, readiness actions. Stated: readiness, not certification.

## Workstream 6 — CI automation (done)

- [x] `.github/workflows/security-scan.yml` (golangci-lint, gosec, govulncheck, npm audit,
      gitleaks, Trivy fs+IaC, licenses, Syft SBOM); actionlint-clean, SARIF upload, least-priv.
- [x] `.pre-commit-config.yaml` + `.trivyignore` baseline (per-entry reasons).
- [x] Wrote `CI_SECURITY_PIPELINE.md` with Matrix E + time-boxed exception process.
- [x] `REMEDIATION_ROADMAP.md` consolidated (12 fixed, 8 P1, 10 P2) + Matrix F traceability.

## Blockers & Open Questions

None outstanding. All four Phase-0 gate questions were answered on 2026-08-13.

## Assumptions Register

| Date | Assumption | Approved by | Basis |
|---|---|---|---|
| 2026-08-13 | Compliance scope = SOC 2 + NIST SSDF + GDPR + ISO/IEC 27001 + EU CRA | user | Q1 answer |
| 2026-08-13 | Pentest = local container on loopback only, no external host | user | Q2 answer (Prime Directive 5) |
| 2026-08-13 | Fix P0/P1 on this branch, document the rest | user | Q3 answer |
| 2026-08-13 | Refactor descoped to guardrail repairs + enablers | user | Q4 answer |
| 2026-08-13 | Security audit target = OWASP ASVS 5.0 Level 2 | agent default | Sensitive business data, not highest-assurance tier |
| 2026-08-13 | PR pipeline budget ≤ 15 min wall clock | agent default | Existing pipeline already runs Keycloak + Playwright |
