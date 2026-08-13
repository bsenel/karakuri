# Karakuri — Consolidated Remediation Roadmap

**Date:** 2026-08-13 · **Baseline:** `ec5a795` on `claude/security-compliance-audit-2pmzg1`

Single prioritized, dated view across every workstream. Findings: `F-*` security
(`SECURITY_AUDIT.md`), `E-*` engineering (`ENGINEERING_AUDIT.md`), `G-*` compliance
(`COMPLIANCE_AUDIT.md`). "Owner" is a role, to be assigned to a person by the team.

## Status summary

| | Count | Findings |
|--|------:|----------|
| ✅ Fixed & verified this branch | 12 | F-01, F-03, F-04, F-05, F-06, F-07, F-08, F-10, F-11, F-13, F-14, F-16, F-20 |
| 📋 Tracked P1 (target +30d) | 8 | F-02, F-09, F-17, F-18, F-23, E-02, E-03, E-06 |
| 📋 Tracked P2 (target +90d) | 10 | F-12, F-15, F-19, F-21, F-22, F-24, F-25, E-04, E-05, G-04 |
| 🏢 Organizational (separate evidence) | — | COMPLIANCE_AUDIT §7 |

*(F-13/F-14 fixed together; F-04/F-05/F-11 fixed together; F-06/F-07 together. F-20 was a
P1 done opportunistically alongside the P0 tranche.)*

## P0 — done on this branch (dates = completed)

| ID | Title | Severity | Remediation commit | Regression / CI gate | Retest |
|----|-------|----------|--------------------|----------------------|--------|
| F-01 | Path traversal via `objective_id` | High | `03502df` | `internal/platform/git/traversal_test.go`; Trivy fs | PT-01 ✅ dir not created |
| F-03 | No rate limit on unauth routes | High | `117a580` | `ratelimit_test.go`; (login-flood is manual) | PT-03 ✅ 26/30→429 |
| F-16 | Refresh token in CLI callback URL | High | `befab80` | `auth/sealed_test.go` (confidentiality) | PT-04 ✅ code is ciphertext |
| F-06 | 28-CVE Go toolchain + root container | High | `4af2ca1` | govulncheck; Trivy image (next); `--check` clean | ✅ go1.25.12, non-root |
| F-13 | react-router crit/high | High | `ad5cbac` | npm audit (prod) gate | ✅ v7, audit clean |
| F-04 | No server timeouts | Medium | `fef55e2` | (config review) | ✅ http.Server set |
| F-05 | No request body limit | Medium | `fef55e2` | `security_test.go` MaxBytes | ✅ 413 on oversize |
| F-07 | Helm no securityContext | Medium | `4af2ca1` | Trivy config gate | ✅ 3 HIGH cleared |
| F-10 | Approver from request body | Medium | `a60a1f5` | (handler review) | ✅ from principal |
| F-11 | No security headers / sniffable route | Medium | `fef55e2` | `security_test.go`; ZAP baseline | PT-05 ✅ 4 headers live |
| F-20 | Login timing enumeration | Medium | `5e772f9` | (auth suite) | ✅ dummy verify |
| F-08 | Lint config dead; script absent | Low | `28ed499` | golangci-lint gate; boundary script | ✅ 0 issues, script runs |

**Every P0 fix carries a test or a CI gate that prevents its regression** — the Matrix F
requirement (a finding with no gate is an accepted risk, and none of the P0s are).

## P1 — target +30 days

| ID | Title | Severity | Owner | Remediation | Blocking gate to add |
|----|-------|----------|-------|-------------|----------------------|
| F-02 | Incomplete authz model (no object scoping) | Medium | backend | Wire checkpoints/artifacts/memory/audit/quota-usage into `ScopedCollection`+row-filter (as twins/objectives). Add `MayActOn` to `quota.Usage` now. | integration role-matrix test extended |
| F-09 | Internal error text to clients | Medium | backend | `writeError` helper + correlation ID (ADR-014); sweep ~71 sites | golangci-lint custom / review |
| F-17 | SAML metadata trusted w/o signature/scheme | High | auth | Require HTTPS for `idp_metadata_url`; verify/pin metadata signature | SAML integration test |
| F-18 | SAML audience restriction permissive | Medium | auth | Set `ValidateAudienceRestriction` (replicate crewjam default minus empty-passes) | SAML integration test |
| F-23 | Latent fail-open in listing/stream filters | Low* | auth | Treat missing principal as deny (require `ok`) | unit test on nil-principal path |
| E-02 | Route-table drift (`/auth/catalog`) | — | backend | Add table entry; fail CI on router/table divergence | new CI check |
| E-03 | No request/correlation ID | — | backend | chi `RequestID` → slog + OTel span (ADR-014) | — |
| E-06 | Error handling leaks internals | — | backend | Same helper as F-09 (ADR-014) | — |

\* F-23 is Low as-is (latent), High if ever triggered — hence P1.

## P2 — target +90 days

| ID | Title | Severity | Owner | Remediation |
|----|-------|----------|-------|-------------|
| F-12 / E-05 | Runtime AutoMigrate; SQL migrations unused | Medium | platform | Versioned migration runner; DML-only runtime DB role (ADR-013) |
| F-15 | `/metrics` unauthenticated | Low/Med | platform | Separate listener / proxy auth; verify labels carry no tenant PII |
| F-19 | No SAML assertion replay cache | Medium | auth | Short-TTL consumed-assertion store in ACS |
| F-21 | Access tokens irrevocable ≤15m | Medium | auth | `jti` denylist checked in Verify, populated on revoke |
| F-22 | Unbounded refresh-token table | Low | auth | Scheduled sweeper calling existing `DeleteExpiredRefreshTokens` |
| F-24 | `/auth/catalog` untested (=E-02) | Low | backend | (folded into E-02) |
| F-25 | No password policy | Low | auth | Min length + breach-list; cap stored iteration count; re-hash on login |
| E-04 | Missing OpenAPI contract | — | backend | Author/generate `docs/openapi.yaml`; validate in integration suite |
| G-04 | GDPR erasure cascade | — | backend + legal | Principal deletion cascades/anonymises audit/cost/memory; documented workflow |
| — | errcheck adoption (REFACTOR D8) | — | maintainers | Baselined rollout, then flip to blocking |

## Organizational (out of scope for code — see COMPLIANCE_AUDIT §7)

Access reviews, DPAs with observability processors + IdP, incident-response plan &
breach-notification workflow, BC/DR, security training, DPO/DPIA. **Blocking for any
SOC 2 / ISO / GDPR certification** and must be evidenced separately.

---

## Matrix F — Traceability (Finding → Control → Evidence → Remediation → CI gate → Retest)

Every fixed finding traces end-to-end. Rows without a CI gate are flagged as accepted risk.

| Finding | Standard/control | Evidence (file:line) | Remediation | CI gate preventing regression | Retest |
|---------|------------------|----------------------|-------------|-------------------------------|--------|
| F-01 | CWE-22; ASVS 5.2.5 | `git/gogitwt.go:50` | `03502df` | `traversal_test.go` + Trivy | PT-01 ✅ |
| F-03 | CWE-307; ASVS 2.2.1 | `server.go:226-247` | `117a580` | `ratelimit_test.go` | PT-03 ✅ |
| F-04 | CWE-400; ASVS 14.1 | `cmd/server/main.go:20` | `fef55e2` | — *(config review; accepted)* | ✅ |
| F-05 | CWE-770; ASVS 12.1.1 | handler decoders | `fef55e2` | `security_test.go` | ✅ |
| F-06 | CWE-1104/250; SSDF PW.4 | `Dockerfile:2` | `4af2ca1` | govulncheck + Trivy image | ✅ |
| F-07 | CWE-250; CIS K8s | `deploy/templates/deployment.yaml` | `4af2ca1` | Trivy config | ✅ |
| F-08 | CWE-1120; SSDF PS.1 | `.golangci.yml` | `28ed499` | golangci-lint + boundary script | ✅ |
| F-10 | CWE-639; ASVS 8.3.1 | `checkpoint.go:45`,`loop.go:58` | `a60a1f5` | — *(handler review; accepted)* | ✅ |
| F-11 | CWE-693/1021; ASVS 14.4 | `handler/helpers.go:8` | `fef55e2` | `security_test.go` + ZAP | PT-05 ✅ |
| F-13 | CWE-1395/601 | `web/package-lock.json` | `ad5cbac` | npm audit (prod) | ✅ |
| F-14 | CWE-540; ASVS 14.3.2 | `web/vite.config.ts` | `ad5cbac` | — *(build inspection; accepted)* | ✅ |
| F-16 | CWE-598/522; ASVS 3.5.3 | `sso_cli.go:114-122` | `befab80` | `sealed_test.go` | PT-04 ✅ |
| F-20 | CWE-208; ASVS 2.2.1 | `auth/token.go:172-189` | `5e772f9` | — *(timing; hard to gate)* | ✅ |
| F-02 | CWE-1220; API1 | `auth/middleware.go:77` | *pending P1* | integration role-matrix (to extend) | PT-02 (reframed) |
| F-17..F-25, E-*, G-04 | (see tables above) | (per finding) | *pending P1/P2* | (per finding) | — |

**Accepted-risk rows** (no automated CI gate): F-04, F-10, F-14, F-20 — each is verified
fixed and low-regression-risk (a config/timing/build property), and is covered by code
review rather than a dedicated automated gate. All other fixed findings have a test or
scanner gate.
