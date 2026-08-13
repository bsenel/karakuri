# Audit & Hardening To-Do

Programme master checklist. Statuses: `[ ]` pending · `[~]` in progress · `[x]` done · `[!]` blocked · `[-]` descoped.
Exactly **one** `[~]` at a time. An item becomes `[x]` only when it links to evidence (file, commit SHA, or report section).

| # | Workstream | Owner | Status | Evidence | Updated |
|---|-----------|-------|--------|----------|---------|
| 0 | Recon & baseline | agent | [x] | Phase 0 report in plan; `/tmp/testbaseline.log`; findings F-01…F-25 | 2026-08-13 |
| 1 | Code cleanup & refactoring (descoped) | agent | [ ] | | |
| 2 | Security audit → SECURITY_AUDIT.md | agent | [x] | `SECURITY_AUDIT.md`; `evidence/` | 2026-08-13 |
| 3 | Compliance audit → COMPLIANCE_AUDIT.md | agent | [ ] | | |
| 4 | Design & dev best-practices audit → ENGINEERING_AUDIT.md | agent | [ ] | | |
| 5 | Penetration testing → PENTEST_REPORT.md | agent | [x] | `PENTEST_REPORT.md`; `evidence/pentest-transcript.txt`, `zap-baseline.json` | 2026-08-13 |
| 6 | CI automation of 2–5 → CI_SECURITY_PIPELINE.md | agent | [ ] | | |

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
- [ ] F-08 — lint repair (done in WS1).
- [-] F-09, F-17, F-18, F-19, F-21..F-25, F-15, F-12 — documented in `REMEDIATION_ROADMAP.md` (P1/P2).

Validation: full `go test ./...` green across all 10 modules; Playwright e2e 5/5 green
(CSP + react-router v7 confirmed in a real browser).

## Workstream 1 — Refactor (descoped to repairs + enablers)

- [ ] F-08 — repair `.golangci.yml` to v2 schema.
- [ ] F-08 — add `scripts/check_langchaingo_imports.sh`.
- [ ] Extract shared authorization helper for the F-02 fixes.
- [ ] Write `REFACTOR_REPORT.md` with Matrix D (incl. rejected candidates).

## Workstream 4 — Engineering audit

- [ ] Write `ENGINEERING_AUDIT.md`: Matrix A, ADRs for gaps, 30/60/90 roadmap.

## Workstream 3 — Compliance audit

- [ ] Data-flow inventory.
- [ ] Write `COMPLIANCE_AUDIT.md`: SOC 2 / SSDF / GDPR / ISO 27001 / EU CRA, technical vs org controls, cross-mapping.

## Workstream 6 — CI automation

- [ ] `security-scan.yml` workflow (SAST/SCA/secret/IaC/SBOM/SARIF/licence).
- [ ] `.pre-commit-config.yaml` + allowlist/baseline files.
- [ ] Write `CI_SECURITY_PIPELINE.md` with Matrix E.
- [ ] `REMEDIATION_ROADMAP.md` consolidated + Matrix F traceability.

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
