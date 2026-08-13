# Karakuri — Software Design & Development Best-Practices Audit

**Date:** 2026-08-13 · **Baseline:** `ec5a795` · **Auditor:** automated review (agent)
**Frameworks:** [NIST SSDF SP 800-218](https://csrc.nist.gov/pubs/sp/800/218/final) · [OWASP SAMM](https://owaspsamm.org/) · [Twelve-Factor App](https://12factor.net/) · [OpenSSF Scorecard](https://scorecard.dev/) · [ADR practice](https://github.com/joelparkerhenderson/architecture-decision-record) · [OpenTelemetry](https://opentelemetry.io/docs/)

## Executive summary

Karakuri is a **well-architected** codebase whose engineering weaknesses are
operational rather than structural. The layering is clean, inward-only, and backed by
**12 ADRs**; the module boundaries are real (10 Go modules, a standalone auth and quota
engine each with its own `go.mod`); the test strategy is genuinely strong (a healthy
0.56 test:code LOC ratio, 90–95% coverage gates enforced per module, a real-IdP
integration suite, and a browser e2e suite). Configuration and secrets management
follow twelve-factor discipline with fatal-on-missing startup checks.

The gaps cluster in three places:

1. **Declared guardrails that were not actually running or present.** The lint config
   was dead (F-08, fixed this branch), the langchaingo-boundary script `AGENTS.md`
   cited did not exist (fixed this branch), and the **OpenAPI contract `AGENTS.md:54`
   points to (`docs/openapi.yaml`) is missing entirely** (E-04). A guardrail nobody can
   run is a guardrail that has already failed.
2. **Data-model lifecycle.** The server runs GORM `AutoMigrate` at startup while a set
   of hand-written versioned SQL migrations sits unused (F-12) — schema drift and a
   permanent DDL grant for the app's DB role.
3. **Observability without correlation.** Rich metric/log/trace fan-out exists, but
   **no request or correlation ID** threads a request through logs (E-06), and internal
   errors leak to clients (F-09), so debugging a production incident is harder than the
   observability investment implies.

**Overall maturity: 69/100 — "Developing"** (Matrix A). Closing the migration,
correlation-ID, and contract-spec gaps would move it into "Strong."

---

## Matrix A — Engineering domain scorecard

Scores 0–5. Weighted % = Σ(weight×score)/(Σweight×5)×100. Weights total 100.

| ID | Domain | Ref | Weight % | Current | Target | Gap | Weighted | Evidence | Priority |
|----|--------|-----|:-------:|:-------:|:------:|:---:|:--------:|----------|:--------:|
| E1 | Architecture & layering (coupling/cohesion/dep direction) | Clean Arch; SAMM Design | 15 | 4 | 5 | 1 | 12.0 | `AGENTS.md`, ADRs 001–012, depguard | P2 |
| E2 | SOLID / DRY / YAGNI adherence | — | 8 | 4 | 4 | 0 | 6.4 | `AGENTS.md` principles, constructor injection | P2 |
| E3 | API/contract design & versioning | 12-factor; SSDF PW.4 | 10 | 3 | 4 | 1 | 6.0 | chi `/api/v1`, but **no OpenAPI file** | P1 |
| E4 | Data model & migration strategy | SSDF PW.9 | 10 | 2 | 4 | 2 | 4.0 | `bootstrap.go:60` AutoMigrate; `migrations/*.sql` unused | P1 |
| E5 | Error handling & resilience | SAMM Ops | 10 | 3 | 4 | 1 | 6.0 | RetryExporter, checkpoints; but F-09, limiter fail-open | P1 |
| E6 | Observability (logs/metrics/traces/correlation) | OpenTelemetry | 10 | 3 | 4 | 1 | 6.0 | OTel + 6 exporters; **no request/correlation ID** | P1 |
| E7 | Configuration & secrets management | 12-factor III/X | 8 | 4 | 5 | 1 | 6.4 | env-based, fatal-on-missing, no committed secrets | P2 |
| E8 | Testing strategy (pyramid, critical paths, quality) | SAMM Verification | 14 | 4 | 5 | 1 | 11.2 | 0.56 test:code ratio; 90–95% gates; real IdP + e2e | P2 |
| E9 | Documentation (README, ADRs, runbooks, onboarding) | — | 8 | 4 | 4 | 0 | 6.4 | 632-line README, 12 ADRs, hierarchical AGENTS.md | P2 |
| E10 | Dependency health & DX/CI | OpenSSF Scorecard; SLSA | 7 | 3 | 4 | 1 | 4.2 | Dependabot; but lint was dark, no SCA gates | P1 |
| | **Rollup** | | **100** | | | | **68.6** | | |

**Maturity band: 68.6 → "Developing" (60–74).**
**Verdict:** the architecture (E1) and testing (E8) domains are the two heaviest-weighted
and both score 4, which is why the codebase *feels* mature — and largely is. The score is
held out of the "Strong" band by three specific, addressable gaps (E3 contract, E4
migrations, E6 correlation) rather than any pervasive weakness.

---

## Findings (engineering)

### E-01 — Guardrails declared but not enforced *(largely fixed this branch)*
`AGENTS.md` asserts three enforcement mechanisms; reconnaissance found **all three were
inert**: the `.golangci.yml` was v1-format and unloadable (so the linter never ran),
`scripts/check_langchaingo_imports.sh` did not exist, and `docs/openapi.yaml` (the "API
contract") is absent. The first two are fixed (`REFACTOR_REPORT.md`); the OpenAPI gap is
E-04. **Lesson:** a claim in a docs file is not a control — CI is.

### E-02 — Route-table drift (also SECURITY_AUDIT F-24)
`internal/auth/routes.go:170-284` is the readable role-matrix the integration suite walks
to assert 200/403 per role, but it **omits `GET /auth/catalog`** (`server.go:265`). The
route works but is untested for authorization. A single source of truth (generate the
table from the router, or fail CI when they diverge) would prevent recurrence.

### E-03 — No request/correlation ID in logs (E-06)
`internal/api/middleware/logging.go` logs method, path and duration but **no request ID**,
and none is propagated into downstream logs or the OTel spans. With six observability
exporters wired (`deploy/values.yaml`), the fan-out is rich but a single request cannot be
followed across log lines — the highest-leverage observability gap. **Remediation:** a
request-ID middleware (chi ships `middleware.RequestID`) stamping a header and a `slog`
context value, echoed on error responses (dovetails with F-09's correlation ID).

### E-04 — Missing API contract spec
`AGENTS.md:54` makes the OpenAPI spec the contract of record — "do not break paths or JSON
fields without updating the spec" — but `docs/openapi.yaml` does not exist. There is no
machine-checkable contract, no generated client, and no CI gate on breaking changes.
**Remediation:** generate an OpenAPI 3.1 document from the chi routes (or hand-author and
lint it in CI), then validate responses against it in the integration suite.

### E-05 — Runtime AutoMigrate vs unused versioned SQL (SECURITY_AUDIT F-12)
`bootstrap.go:60` → `db.AutoMigrate` over 13 models at startup; `migrations/000001..06.sql`
have no runner. The live schema is whatever GORM infers, the SQL of record is documentation,
and the app's DB role needs DDL forever. **Remediation:** adopt golang-migrate (or GORM's
versioned migrator) driven by `migrations/`, drop AutoMigrate in production, and scope the
runtime DB role to DML. Proposed as ADR-013 below.

### E-06 — Error handling leaks internals (SECURITY_AUDIT F-09)
71 handler sites return `err.Error()` to the client. Beyond the security angle, this couples
the client to internal error text and makes a stable error contract impossible. A single
`writeError(w, r, status, publicMsg)` helper that logs detail with the request ID and
returns a typed error envelope fixes both. Proposed as ADR-014.

---

## Proposed ADRs

Two architectural decisions are significant enough to warrant an ADR; drafts are in
`docs/audit/adr/`:

- **ADR-013 — Versioned migrations replace runtime AutoMigrate** (`adr/013-versioned-migrations.md`)
- **ADR-014 — Uniform API error envelope with correlation IDs** (`adr/014-error-envelope-and-correlation-ids.md`)

---

## 30 / 60 / 90-day improvement roadmap

**Days 0–30 (correctness & contract):**
- Adopt versioned migrations; scope the runtime DB role to DML (E-05 / F-12, ADR-013).
- Add request-ID middleware + propagate to logs and OTel spans (E-03).
- Introduce the uniform error envelope with correlation IDs (E-06 / F-09, ADR-014).
- Add `GET /auth/catalog` to the role-matrix table; fail CI on router/table divergence (E-02).

**Days 30–60 (contract & supply chain):**
- Author/generate `docs/openapi.yaml`; validate responses in the integration suite (E-04).
- Wire golangci-lint, gosec, govulncheck into CI (now that the config loads); begin the
  baselined errcheck rollout (`REFACTOR_REPORT.md` D8).
- Add the full DevSecOps gate set from `CI_SECURITY_PIPELINE.md` (SCA, secret, IaC, SBOM).

**Days 60–90 (resilience & maturity):**
- Make the quota limiter's fail-open configurable per deployment; document the tradeoff.
- Add runbooks for the top three operational scenarios (login outage, DB failover, exporter
  backpressure) and a measured local-setup / CI-duration baseline for DX.
- Target OpenSSF Scorecard ≥ 7 and SLSA build provenance (dovetails with the release
  workflow hardening in `CI_SECURITY_PIPELINE.md`).

## Strengths (do not regress)

Recorded so a future refactor does not trade them away: the inward-only layering with
depguard enforcement; the standalone auth/quota modules with their own coverage gates; the
real-IdP (Keycloak) integration job and the browser e2e job; twelve-factor config with
fatal-on-missing secrets; 12 ADRs capturing the *why*; and a test:code ratio (0.56) and
per-module coverage bar (90–95%) most projects of this size do not hold.
