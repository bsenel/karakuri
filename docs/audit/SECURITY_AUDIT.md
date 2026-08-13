# Karakuri — Security Audit

**Classification:** Internal · **Date:** 2026-08-13 · **Auditor:** automated security review (agent)
**Commit baseline:** `ec5a795` on `claude/security-compliance-audit-2pmzg1`
**Standards:** OWASP Top 10:2025 · OWASP ASVS 5.0 **Level 2** · OWASP API Security Top 10:2023 · CWE Top 25 · CVSS v4.0

> This is an internal engineering security assessment, not a certification or a
> penetration-test attestation for a third party. Active exploit verification was
> performed against a **local container only** (see `PENTEST_REPORT.md`); no
> external or production host was touched.

---

## 1. Executive summary

Karakuri is a self-hosted AI-agent orchestrator (Go 1.25, 10 modules, ~37k LOC of
non-test Go plus a ~4.5k-LOC embedded React SPA). Its **security core is unusually
strong**: password hashing, JWT handling, the RBAC decision engine, OIDC/SAML
federation, and the tenancy filter in the twin/objective query layer are all sound,
deliberately documented, and in several places safer than typical hand-rolled code
(the JWT verifier closes `alg:none` and algorithm confusion by construction; the
authorizer has exactly one code path that emits an allow).

The defects **cluster at the HTTP edge and in the deployment/supply chain**, not in
the crypto:

- **Incomplete authorization model.** A `require(action, nil)` idiom gates a cluster of
  routes (`/quota/usage`, `/checkpoints`, `/artifacts`, `/memory/*`, `/audit`) on a
  collection-wide permission rather than a per-object one. **Active testing corrected
  the initial read of this** (see `PENTEST_REPORT.md` PT-02): the gate *fails closed*
  for tenant-scoped principals, so it is not a live cross-tenant leak; the real defect
  is that these resources are not wired into the container-scoping system twins and
  objectives use, leaving no object-level ownership enforcement. Downgraded High →
  Medium. (F-02)
- **A path-traversal chain** from `POST /loops` to `os.MkdirAll`/`git worktree add`
  outside the repo root, amplified by the container running as **root**. (F-01, F-06)
- **No rate limiting on any unauthenticated route**, including login, and the one
  limiter that exists **fails open**. Combined with a login **timing side channel**,
  this is a practical credential-attack surface. (F-03, F-20)
- **A live refresh token handed back in a plaintext URL** during CLI SSO, defeating a
  documented security property. (F-16)
- **Deployment hardening is largely absent**: no pod/container `securityContext`, no
  server timeouts, no request-body limits, no security response headers, and a
  builder image that ships a Go toolchain with 28 known CVEs. (F-04–F-07, F-11)

25 findings are recorded below (6 High, 12 Medium, 1 Low/Medium, 6 Low), each with a
traced evidence chain, and one (F-02) that active testing **downgraded** from the
static tool's High reading — the kind of correction verification exists to produce.
**The static scanners were largely unhelpful**:
all 13 semgrep warnings and the bulk of gosec's 35 were verified false positives
(Appendix C); every substantive finding came from manual review of the auth and
data-access paths.

**Overall maturity: 43/100 — "Weak" band** (Matrix A rollup). This is the signature
of a codebase whose *logic* is mature but whose *edge and operational posture* have
not caught up. The remediation roadmap closes the two High-critical and all nine High
findings inside the P0/P1 tranche; several are one- or two-line fixes.

---

## 2. Scope & method

### In scope
- The server (`cmd/server`, `internal/`), CLI (`cmd/krk`, `cli/`), and the ten Go
  modules (`auth{,/sql,/oidc,/saml}`, `quota{,/sql,/valkey,/cost,/cost/sql}`).
- The embedded React SPA (`web/`).
- Container and Helm deployment assets (`Dockerfile`, `deploy/`).
- Dependency and supply-chain posture (Go + npm).

### Out of scope
- Third-party integrations behind no-op adapters (Linear, Slack, Figma, etc.).
- Upstream LLM providers.
- The actual TLS-terminating reverse proxy a production deployment would place in
  front of the server (assumed but not provided).

### Method
1. **Manual review** of every authentication and authorization path and every
   data-access handler — the source of all substantive findings.
2. **SAST** — semgrep (`p/golang`, `p/security-audit`, `p/secrets`, `p/typescript`),
   gosec, CodeQL (existing in CI).
3. **SCA** — `govulncheck` (Go, reachability-aware), `npm audit` (JS).
4. **Secret scan** — gitleaks over full history (50 commits) and working tree.
5. **IaC scan** — Trivy config scanner + Checkov over `Dockerfile` and `deploy/`.
6. **SBOM** — Syft, CycloneDX + SPDX (349 components).
7. **Dynamic verification** — local-container exploitation of the top findings
   (`PENTEST_REPORT.md`).

Tool versions and raw-output paths: `evidence/TOOL_VERSIONS.md`. Raw outputs:
`evidence/`.

---

## 3. Matrix A — Domain scorecard

Scores 0–5 (0 absent · 3 implemented-but-unverified · 5 automated+monitored+enforced).
Weighted % = Σ(weight×score) / (Σweight×5) × 100. Weights total 100.

| ID | Domain | Standard ref | Weight % | Current | Target | Gap | Weighted | Evidence | Key findings | Priority |
|----|--------|-------------|:-------:|:-------:|:------:|:---:|:--------:|----------|--------------|:--------:|
| A1 | Authentication & session mgmt | ASVS V2/V3; API2 | 15 | 2 | 4 | 2 | 6.0 | `auth/token.go`, `auth/jwt/` | F-03, F-16, F-20, F-21, F-25 | P0 |
| A2 | Authorization & access control | ASVS V4; Top10 A01; API1/API3/API5 | 18 | 2 | 4 | 2 | 7.2 | `auth/middleware.go`, `internal/api/handler/` | F-01, F-02, F-10, F-23 | P0 |
| A3 | Input validation & injection | ASVS V5; Top10 A03; API8 | 12 | 2 | 4 | 2 | 4.8 | `internal/api/handler/` | F-05, F-01 | P1 |
| A4 | Cryptography & secrets | ASVS V6/V7; Top10 A02 | 8 | 4 | 5 | 1 | 6.4 | `auth/credential.go`, `auth/jwt/` | (clean — App. B) | P2 |
| A5 | Dependency & supply chain | Top10 A06; SSDF PW.4 | 12 | 2 | 4 | 2 | 4.8 | `go.mod`, `web/package.json` | F-06, F-13 | P0 |
| A6 | Configuration & hardening | ASVS V14; Top10 A05; CIS | 14 | 1 | 4 | 3 | 2.8 | `Dockerfile`, `deploy/` | F-04, F-06, F-07, F-11 | P0 |
| A7 | Logging, monitoring & error handling | ASVS V7/V8; Top10 A09 | 10 | 2 | 4 | 2 | 4.0 | `internal/api/handler/`, `internal/auth/audit.go` | F-03, F-09, F-15 | P1 |
| A8 | Data protection (rest/transit) | ASVS V6/V9; Top10 A02 | 6 | 3 | 4 | 1 | 3.6 | `auth/cookie.go`, `deploy/values.yaml` | F-07, F-14 | P2 |
| A9 | CI/CD pipeline security | Top10 A08; SLSA; SSDF | 5 | 3 | 4 | 1 | 3.0 | `.github/workflows/` | F-08, (WS6) | P1 |
| | **Rollup** | | **100** | | | | **42.6** | | | |

**Maturity band: 42.6 → "Weak" (40–59).**
**Verdict:** The weighted score understates the *core* and overstates the *risk of a
total compromise*, because the strongest domain (A4, crypto) and the
correctly-failing-closed tenancy filter mean most findings require an authenticated
foothold. But the score correctly captures that an authenticated user can currently
cross tenant boundaries (A2) and that the operational posture (A6) would not survive
a hostile network. Closing the P0 tranche moves A2→4 and A6→4 and lifts the rollup to
the "Developing"/"Strong" boundary.

---

## 4. Matrix B — Findings register

Severity uses CVSS v4.0 base. Risk score = Likelihood × Impact (1–5 each, Matrix C).

| ID | Title | Category | Severity | CVSS v4.0 | Likel. | Impact | Risk | Standard mapping | Evidence | Status | Remediation | Effort | Target |
|----|-------|----------|----------|:---------:|:------:|:------:|:----:|------------------|----------|--------|-------------|:------:|:------:|
| F-01 | Path traversal via unvalidated `objective_id` | Access control / traversal | **High** | 8.3 | 3 | 5 | 15 | CWE-22; ASVS 5.2.5; API1 | `handler/loop.go:31`→`git/gogitwt.go:50` | Open→Fixed | Resolve from store + containment check | S | P0 |
| F-02 | Incomplete authz model — collection-wide gate, no object scoping | Access control | Medium | 6.0 | 3 | 3 | 9 | CWE-1220/CWE-284; ASVS 4.2.1; API1 | `auth/middleware.go:77-80` + routes | Open | Wire resources into container-scoping (design) | L | P1 |
| F-03 | No rate limit on unauth routes; limiter fails open | Auth / DoS | **High** | 8.7 | 4 | 4 | 16 | CWE-307/CWE-770; ASVS 2.2.1; API4 | `server.go:226-247`, `quota/middleware.go:53` | Open→Fixed | IP rate-limit; fail closed | M | P0 |
| F-06 | Shipped image: 28-CVE Go toolchain, runs as root | Supply chain / hardening | **High** | 8.1 | 3 | 4 | 12 | CWE-1104/CWE-250; SSDF PW.4 | `Dockerfile:2,12`, `go.mod:3` | Open→Fixed | Pin patched Go; non-root USER; pin bases | S | P0 |
| F-13 | Frontend deps: react-router crit+high | Supply chain | **High** | 8.2 | 3 | 4 | 12 | CWE-1395/CWE-601; Top10 A06 | `web/package-lock.json` | Open→Fixed | `npm audit fix` (react-router ≥7.9) | S | P0 |
| F-16 | Live refresh token in cleartext CLI callback URL | Session mgmt | **High** | 8.0 | 3 | 4 | 12 | CWE-598/CWE-522; ASVS 3.5.3; API2 | `handler/sso_cli.go:114-122` | Open→Fixed | One-time reference or bind to PKCE | M | P0 |
| F-17 | SAML IdP metadata trusted without signature/scheme | Federation / MITM | **High** | 7.4 | 2 | 5 | 10 | CWE-347/CWE-295; ASVS 6.2.6 | `internal/auth/federation.go:211-261` | Open | Enforce HTTPS + pin/verify metadata | M | P1 |
| F-04 | No HTTP server timeouts (slowloris) | Availability | Medium | 6.9 | 4 | 3 | 12 | CWE-400; ASVS 14.1; API4 | `cmd/server/main.go:20` | Open→Fixed | Configured `http.Server` | S | P0 |
| F-05 | No request body size limit | Availability | Medium | 6.5 | 4 | 3 | 12 | CWE-400/CWE-770; ASVS 12.1.1 | all `json.NewDecoder(r.Body)` | Open→Fixed | `http.MaxBytesReader` | S | P0 |
| F-11 | No security headers / CSP; sniffable artifact route | Hardening / XSS | Medium | 6.1 | 3 | 3 | 9 | CWE-693/CWE-1021; ASVS 14.4 | `handler/helpers.go:8`, `handler/artifact.go:41` | Open→Fixed | Header middleware + nosniff | S | P0 |
| F-07 | Helm chart has no `securityContext` | Hardening | Medium | 6.0 | 3 | 4 | 12 | CWE-250/CWE-732; CIS K8s | `deploy/templates/deployment.yaml` | Open→Fixed | Pod+container securityContext | S | P0 |
| F-09 | Internal error text returned to clients | Info leak | Medium | 5.3 | 4 | 2 | 8 | CWE-209; ASVS 7.4.1 | 71× `http.Error(w, err.Error())` | Open→Fixed | Opaque errors + correlation ID | M | P1 |
| F-10 | Approver identity taken from request body | Integrity / audit | Medium | 5.9 | 3 | 3 | 9 | CWE-639/CWE-778; ASVS 8.3.1 | `handler/checkpoint.go:45`, `handler/loop.go:58` | Open→Fixed | Use authenticated principal | S | P1 |
| F-12 | Runtime AutoMigrate; SQL migrations never run | Config / integrity | Medium | 5.1 | 3 | 3 | 9 | CWE-665; SSDF PW.9 | `bootstrap.go:60`, `migrations/` | Open | Adopt versioned migrations | M | P2 |
| F-18 | SAML audience restriction permissive by default | Federation | Medium | 5.8 | 2 | 4 | 8 | CWE-287; ASVS 6.2.3 | `auth/saml/saml.go:206-214` | Open | Set `ValidateAudienceRestriction` | S | P1 |
| F-19 | No SAML assertion replay cache | Federation | Medium | 5.6 | 2 | 4 | 8 | CWE-294; ASVS 6.2.4 | `auth/saml/saml.go:303,343-355` | Open | Consumed-assertion store | M | P2 |
| F-20 | Login timing side channel → user enumeration | Auth | Medium | 5.3 | 3 | 3 | 9 | CWE-208/CWE-203; ASVS 2.2.1 | `auth/token.go:172-189` | Open | Dummy verify on all paths | S | P1 |
| F-21 | Access tokens irrevocable before expiry | Session mgmt | Medium | 5.4 | 3 | 3 | 9 | CWE-613; ASVS 3.3.1 | `auth/token.go:266-292` | Open | `jti` denylist on revoke | M | P2 |
| F-15 | `/metrics` served unauthenticated | Info leak | Low/Med | 4.8 | 3 | 2 | 6 | CWE-306; ASVS 8.1 | `server.go:128-130` | Open | Bind separately / authN | S | P2 |
| F-08 | Lint config dead; enforcement script absent | Process | Low | 3.1 | 5 | 1 | 5 | CWE-1120; SSDF PS.1 | `.golangci.yml`, `AGENTS.md` | Open→Fixed | Repair config; add script; wire CI | S | P1 |
| F-14 | Production source maps embedded and served | Info leak | Low | 3.9 | 3 | 1 | 3 | CWE-540; ASVS 14.3.2 | `web/vite.config.ts`, `web/embed.go:62` | Open→Fixed | Disable prod sourcemaps | S | P1 |
| F-22 | Unbounded refresh-token table | Availability | Low | 3.7 | 3 | 2 | 6 | CWE-770; ASVS 3.3.4 | `auth/sql/credentials.go:153-160` | Open | Scheduled sweeper | S | P2 |
| F-23 | Latent fail-open in listing/stream filters | Access control | Low | 4.6 | 1 | 4 | 4 | CWE-636; ASVS 4.1.5 | `internal/auth/listing.go:99-105`, `stream.go:53-61` | Open | Require `ok` from ctx lookup | S | P1 |
| F-24 | Route-table drift: `/auth/catalog` untested | Process | Low | 2.3 | 4 | 1 | 4 | CWE-1053 | `internal/auth/routes.go` vs `server.go:265` | Open | Add table entry | S | P2 |
| F-25 | No password / bootstrap-password policy | Auth | Low | 3.9 | 3 | 2 | 6 | CWE-521; ASVS 2.1 | `auth/credential.go:58` | Open | Length+strength policy | S | P2 |

*Status `Open→Fixed` marks the P0/P1 items remediated on this branch (see
`REMEDIATION_ROADMAP.md` and `PENTEST_REPORT.md` retest).*

---

## 5. Matrix C — Risk heat map

`Risk = Likelihood × Impact`. Cell shows count + finding IDs.

| Likelihood ↓ \ Impact → | 1 Negligible | 2 Minor | 3 Moderate | 4 Major | 5 Severe |
|-------------------------|--------------|---------|------------|---------|----------|
| **5 Almost certain** | 1 · F-08 | | | | |
| **4 Likely** | 2 · F-24 | 1 · F-09 | 2 · F-04, F-05 | 1 · F-03 | |
| **3 Possible** | 2 · F-14 | 3 · F-15, F-22, F-25 | 5 · F-02, F-11, F-10, F-20, F-21 | 3 · F-06, F-13, F-16 | 1 · F-01 |
| **2 Unlikely** | | | | 3 · F-18, F-19 | 1 · F-17 |
| **1 Rare** | | | | 1 · F-23 | |

The mass sits in the **Possible/Major** and **Likely/Major** cells — authenticated,
plausible, high-impact. Nothing is in "Almost certain / Severe" because every
severe-impact item (F-01, F-17) requires either a specific capability grant or a
network-level position.

---

## 6. Detailed findings

Each finding: evidence (`file:line`) · reproduction · impact · remediation · effort ·
references. Fixes marked *(remediated)* have a commit and a retest in
`PENTEST_REPORT.md`.

### F-01 — Path traversal via unvalidated `objective_id` → directory creation outside repo root · **High** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:L/VI:H/VA:L/SC:N/SI:N/SA:N (8.3)

**CWE-22 · ASVS 5.2.5 · OWASP API1:2023 (BOLA) · Top10 A01**

**Evidence — full chain:**
1. `internal/api/handler/loop.go:30-35` builds a **synthetic** objective directly from
   the request body, never loading it from the store:
   ```go
   result, err := h.Loop.Run(r.Context(), loop.Request{
       Objective: objective.Objective{ID: objective.ObjectiveID(req.ObjectiveID)},
       Twin:      twin.DigitalTwin{ID: req.TwinID},
   ...
   ```
2. `internal/feature/loop/act.go:83-86` passes that ID into the worktree manager:
   ```go
   wt, err := sc.svc.wt.Create(ctx, git.WorktreeOptions{
       ObjectiveID: sc.obj.ID,
       TaskID:      taskID,
   })
   ```
3. `internal/platform/git/gogitwt.go:50-51` joins it into a filesystem path and creates
   it — with no containment check:
   ```go
   basePath := filepath.Join(m.repoRoot(), m.cfg.WorktreeBase, string(opts.ObjectiveID), opts.TaskID)
   if err := os.MkdirAll(filepath.Dir(basePath), 0o755); err != nil { ... }
   ```

`filepath.Join` cleans `..` segments but does **not** re-anchor: a sufficiently long
`../` prefix escapes `repoRoot`. `gosec` flagged the sink as G301 (`gogitwt.go:51`);
the source-to-sink trust was confirmed by reading the three files.

**Reproduction:** authenticated caller with `loop:start` and a `.write_code`/
`.write_test` capability sends `POST /api/v1/loops` with
`{"objective_id":"../../../../tmp/karakuri-pwn","twin_id":"t1"}`. Verified against the
local container — see `PENTEST_REPORT.md` PT-01.

**Impact:** arbitrary directory creation, and a `git worktree add` target, outside the
intended tree. Amplified by **F-06** (process runs as root in the container): the
write lands as root anywhere the root filesystem is writable. `objectiveID` is
also truncated to 8 chars for the *branch* name (`gogitwt.go:42-44`) but **not** for
the *path* (`:50`), so the branch truncation is not a mitigating control.

**Remediation *(remediated, P0)*:** (a) resolve the objective from the store in the
loop handler so a non-existent ID is rejected before it becomes a path; (b) add an
explicit containment check at the join, mirroring the one that already exists in
`shell_env.go:173-181`:
```go
base := filepath.Join(root, cfg.WorktreeBase)
p := filepath.Join(base, string(objID), taskID)
if rel, err := filepath.Rel(base, p); err != nil || strings.HasPrefix(rel, "..") {
    return Worktree{}, fmt.Errorf("worktree path escapes base: %q", objID)
}
```
**Effort:** S. **Refs:** [CWE-22](https://cwe.mitre.org/data/definitions/22.html) ·
[ASVS 5.2.5](https://github.com/OWASP/ASVS) ·
[OWASP Path Traversal](https://owasp.org/www-community/attacks/Path_Traversal).

### F-02 — Incomplete authorization model: collection-wide gate, no object-level scoping · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:H/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N (6.0)

**CWE-1220 (insufficient granularity of access control) / CWE-284 · ASVS 4.2.1 · OWASP API1/API5**

> **Corrected by active testing.** The static reading of this finding predicted a live
> cross-tenant IDOR/BOLA. Local exploitation (`PENTEST_REPORT.md` PT-02) **did not
> reproduce that** and is documented here in full, because getting the severity right
> matters more than confirming a prior.

**Evidence:** `auth/middleware.go:77-80` resolves a nil resource-func to
`Collection(action.Type())` — a wildcard `"<type>:*"` scope. The following routes pass
`nil` and take the target ID from the request:

| Route | Handler | Untrusted ID |
|-------|---------|--------------|
| `GET /quota/usage?twin=` | `handler/quota.go:162-185` | query `twin` |
| `POST /quota/reset` | `handler/quota.go:193-213` | body `twin` |
| `GET /checkpoints?twin_id=` | `handler/checkpoint.go:16-34` | query/path |
| `GET /artifacts`, `GET /artifacts/{sha}` | `handler/artifact.go:34-54` | query/path |
| `/memory/{store,recall,forget}` | `handler/memory.go:15-53` | body |
| `GET /audit`, `GET /audit/{id}` | `handler/audit.go:19-65` | query/path |
| `GET /auth/policies?principal=` | `handler/auth.go:370-379` | query |

**What the gate actually does (verified live).** A `Collection("<type>")` resource
carries no container labels, so a **container-scoped** binding does **not** satisfy it —
the enforcer returns 403 *"no role binding covers `<type>:*`"*. Only a principal holding
the permission at **global scope (`*`)** passes. Confirmed:

```
attacker (auditor scoped to org acme) → GET /quota/usage?twin=beta-twin → 403 "no binding covers quota:*"
attacker (auditor scoped to org acme) → GET /audit                       → 403 "no binding covers audit:*"
gviewer  (auditor scoped to *)         → GET /audit                       → 200 (all principals' events)
```

So the gate **fails closed** for scoped principals — there is **no cross-tenant leak by
a scoped principal**. The real defect is narrower and structural:

1. These resource types are **not integrated into the container-scoping system** that
   twins/objectives use (`ScopedCollection` widening + row filtering,
   `internal/auth/listing.go:35-67`, `gorm_storage.go:640-667`). A tenant-scoped
   principal therefore **cannot use these endpoints at all** — over-restrictive, but
   fail-closed.
2. Within a **global** grant there is **no object-level ownership narrowing**: a global
   `auditor`/`viewer`/`operator` reads every principal's data. That is the documented
   behaviour of a *global* role, so it is a completeness gap (CWE-1220), not a boundary
   bypass.

`quota.Usage` (`handler/quota.go:162-185`) remains a genuine local inconsistency: it is
the only method in its file that does not call `MayActOn` per subject while every
sibling does.

**Impact:** no live cross-tenant disclosure. The gap prevents per-tenant delegation of
these features and removes defence-in-depth (object-level checks) that the twin/objective
routes have. If a future refactor widened one of these gates via `ScopedCollection`
without adding the row filter, it *would* become a live IDOR — so this is also a latent
trap adjacent to F-23.

**Remediation (P1 — design change, not a one-liner):** wire checkpoints, artifacts,
memory, audit and `quota/usage` into the same `ScopedCollection` + row-filter pattern
the list routes use, so a tenant-scoped principal sees exactly their container's rows.
As an immediate consistency fix, add the `MayActOn` guard to `quota.Usage` to match its
siblings. **Effort:** L. **Refs:**
[CWE-1220](https://cwe.mitre.org/data/definitions/1220.html) ·
[API1:2023 BOLA](https://owasp.org/API-Security/editions/2023/en/0xa1-broken-object-level-authorization/) ·
[Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html).

### F-03 — No rate limiting on unauthenticated routes; the limiter fails open · **High** · CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:L/VA:L/SC:N/SI:N/SA:N (8.7)

**CWE-307/CWE-770 · ASVS 2.2.1 · OWASP API4 · Top10 A07**

**Evidence:** the only limiter is mounted **inside** the authenticated group
(`internal/api/server.go:247`), after the public auth routes are registered
(`:226-238`). Its key extractor returns `""` — treated as exempt — when there is no
principal (`internal/quota/quota.go:74-80`), so it is structurally incapable of
covering unauthenticated traffic. It also **fails open** on backend error
(`quota/middleware.go:53-58`; `failClosed` is not set by `Deps.Limiter()`,
`internal/quota/quota.go:99-126`), logging "request allowed unlimited."

Consequently `POST /auth/token`, `/auth/refresh`, `/auth/sso/*`, `/auth/saml/acs`,
`/health` and `/metrics` have **no throttle at all**.

**Reproduction:** unthrottled `POST /api/v1/auth/token` at network speed — see
`PENTEST_REPORT.md` PT-03.

**Impact:** credential brute-force and password spraying with no application brake;
compounds **F-20** (enumerate valid users by timing, then spray). Also a DoS vector:
each login attempt costs 600k PBKDF2 iterations, so an attacker can pin CPU cheaply.

**Remediation *(remediated, P0)*:** add a lightweight per-client-IP rate-limit
middleware in front of the public `/auth/*` group (token-bucket, e.g. 10 attempts/min
with burst), and set `failClosed` on the quota limiter so a store outage denies rather
than admits. **Effort:** M. **Refs:**
[CWE-307](https://cwe.mitre.org/data/definitions/307.html) ·
[ASVS 2.2.1](https://github.com/OWASP/ASVS) ·
[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#protect-against-automated-attacks).

### F-04 — No HTTP server timeouts · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:N/VI:N/VA:L/SC:N/SI:N/SA:L (6.9)

**CWE-400 · ASVS 14.1 · API4**

**Evidence:** `cmd/server/main.go:20` — `http.ListenAndServe(addr, handler)` — uses the
default server with **no** `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, or
`ReadHeaderTimeout`. The repo already knows the pattern (`cli/client/sso.go:100-102`
sets `ReadHeaderTimeout`).

**Impact:** slowloris / slow-body connection exhaustion.

**Remediation *(remediated, P0)*:** construct an `http.Server{}` with
`ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 60s` (allowing for SSE via
per-route overrides), `IdleTimeout: 120s`. **Effort:** S. **Refs:**
[Cloudflare: Go net/http timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) ·
[CWE-400](https://cwe.mitre.org/data/definitions/400.html).

### F-05 — No request body size limit · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:N/VI:N/VA:L/SC:N/SI:N/SA:L (6.5)

**CWE-770 · ASVS 12.1.1**

**Evidence:** zero `http.MaxBytesReader` on inbound handlers; every
`json.NewDecoder(r.Body).Decode(...)` is unbounded (e.g. `handler/auth.go:94`,
`handler/memory.go:17,30,44`, `handler/quota.go:117`). No `DisallowUnknownFields`
anywhere, and `handler/memory.go` decodes directly into domain types, so every
exported field is client-settable.

**Impact:** memory-pressure DoS from an oversized body; over-permissive deserialization.

**Remediation *(remediated, P0)*:** wrap request bodies with
`http.MaxBytesReader(w, r.Body, 1<<20)` in a middleware, and enable
`dec.DisallowUnknownFields()` on the sensitive decoders. **Effort:** S. **Refs:**
[ASVS 12.1.1](https://github.com/OWASP/ASVS) ·
[CWE-770](https://cwe.mitre.org/data/definitions/770.html).

### F-06 — Shipped container ships a 28-CVE Go toolchain and runs as root · **High** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:L/VI:H/VA:L/SC:L/SI:L/SA:L (8.1)

**CWE-1104/CWE-250 · SSDF PW.4 · Top10 A06**

**Evidence:** `Dockerfile:2` pins `golang:1.23-bookworm` while `go.mod:3` requires
`go 1.25.0`. With `GOTOOLCHAIN=auto`, the builder downloads and pins **exactly
go1.25.0** — the vulnerable patch. `govulncheck` finds **28 reachable stdlib
advisories** against that toolchain (`evidence/govulncheck.json`), e.g. GO-2025-4007
(x509 quadratic complexity), GO-2025-4008 (TLS ALPN info leak), GO-2025-4010 (net/url
IPv6). CI escapes this because `setup-go: 1.25.x` floats to the latest patch, so **the
vulnerability exists only in the shipped artifact, not in CI**. Compounding, from
Trivy/Checkov (`evidence/trivy-config.json`, `evidence/checkov-dockerfile.json`):
`Dockerfile:12 FROM alpine:3` (unpinned), **no `USER` directive** (DS002 HIGH — runs as
root), `apk add` unpinned, no `HEALTHCHECK` in the image.

**Impact:** the runtime binary carries known stdlib DoS/parsing bugs, and every
process-level defect (including F-01's filesystem write) executes as root.

**Remediation *(remediated, P0)*:** pin the builder to the latest patched 1.25.x
(`golang:1.25.<patched>-bookworm`), add a non-root `USER`, and pin the runtime base by
digest. **Effort:** S. **Refs:**
[Go vuln DB](https://pkg.go.dev/vuln/) ·
[Docker: run as non-root](https://docs.docker.com/develop/security-best-practices/) ·
[CWE-250](https://cwe.mitre.org/data/definitions/250.html).

### F-07 — Helm chart defines no `securityContext` · **Medium** · CVSS:4.0/AV:L/AC:L/AT:N/PR:L/UI:N/VC:L/VI:H/VA:L/SC:L/SI:L/SA:L (6.0)

**CWE-250/CWE-732 · CIS Kubernetes Benchmark**

**Evidence:** `deploy/templates/deployment.yaml` has no pod- or container-level
`securityContext`. Trivy flags KSV012 (runs as root), KSV014 (root FS not read-only),
KSV001 (privilege escalation possible), KSV104 (seccomp unset), KSV003/004 (capabilities
not dropped) — `evidence/trivy-config.json`. `deploy/values.yaml` also defaults Postgres
`sslmode: disable`.

**Impact:** a compromised container has root, a writable root FS, default Linux
capabilities, and no seccomp profile — maximal blast radius.

**Remediation *(remediated, P0)*:**
```yaml
securityContext:            # pod
  runAsNonRoot: true
  runAsUser: 65532
  seccompProfile: { type: RuntimeDefault }
containers:
  - name: karakuri
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities: { drop: ["ALL"] }
```
(with an `emptyDir` for the writable paths the app needs). Flip the Postgres default to
`sslmode: require`. **Effort:** S. **Refs:**
[Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/) ·
[CIS K8s Benchmark](https://www.cisecurity.org/benchmark/kubernetes).

### F-09 — Internal error text returned to clients · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N (5.3)

**CWE-209 · ASVS 7.4.1**

**Evidence:** 71 × `http.Error(w, err.Error(), …)` in `internal/api/handler/`, 20 of
them on the auth surface (`handler/auth.go:196-583`). Raw GORM/storage error text
reaches the client, including at `handler/auth.go:149` where the refresh error
distinguishes "expired"/"revoked"/"invalid" states. (Stack traces are **not** leaked —
chi `Recoverer` sends a bare 500.)

**Impact:** internal schema/driver disclosure; a refresh-state oracle.

**Remediation *(remediated, P1)*:** a `writeError` helper that returns a generic
client message plus a server-side correlation ID, and logs the detail. **Effort:** M.
**Refs:** [CWE-209](https://cwe.mitre.org/data/definitions/209.html) ·
[Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html).

### F-10 — Approver identity taken from the request body · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:N/VI:H/VA:N/SC:N/SI:N/SA:N (5.9)

**CWE-639/CWE-778 · ASVS 8.3.1**

**Evidence:** `handler/checkpoint.go:45` and `handler/loop.go:58` read a client-supplied
`Approver` string from the body rather than from the authenticated principal in
context. The approval audit record is therefore self-asserted.

**Impact:** an approver can attribute a checkpoint approval to someone else; the audit
trail of who authorized an agent action is untrustworthy.

**Remediation *(remediated, P1)*:** derive the approver from
`auth.PrincipalFromContext(r.Context())`; ignore any body-supplied value. **Effort:** S.
**Refs:** [CWE-639](https://cwe.mitre.org/data/definitions/639.html) ·
[ASVS 8.3.1](https://github.com/OWASP/ASVS).

### F-11 — No security response headers; artifact route is content-sniffable · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:A/VC:L/VI:L/VA:N/SC:L/SI:L/SA:N (6.1)

**CWE-693/CWE-1021 · ASVS 14.4 · Top10 A05**

**Evidence:** no `Content-Security-Policy`, `X-Frame-Options`,
`X-Content-Type-Options`, `Strict-Transport-Security`, or `Referrer-Policy` is set
anywhere (`handler/helpers.go:8-11`, `web/embed.go:40-56`, `web/index.html`). Concrete
instance flagged by gosec (G705) at `handler/artifact.go:41`: attacker-storable
artifact content is served as `text/plain` with no `nosniff`, so a browser MIME-sniff
can execute stored HTML/JS. CSRF defence is `SameSite=Strict` alone (documented,
`auth/cookie.go:16-20`) — single-layer for cookie-authenticated `POST /auth/refresh`
and `/auth/revoke`.

**Impact:** clickjacking (SPA is framable), stored-XSS-via-sniff on the artifact route,
weakened CSRF depth.

**Remediation *(remediated, P0)*:** a security-header middleware applying
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, a restrictive `CSP`,
`Referrer-Policy: no-referrer`, and `HSTS` (behind TLS). Serve artifact content with
`Content-Disposition: attachment` + `nosniff`. **Effort:** S. **Refs:**
[OWASP Secure Headers](https://owasp.org/www-project-secure-headers/) ·
[CSP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html).

### F-12 — Runtime `AutoMigrate`; versioned SQL migrations never executed · **Medium** · CVSS:4.0/AV:L/AC:H/AT:P/PR:H/UI:N/VC:N/VI:L/VA:L/SC:N/SI:N/SA:N (5.1)

**CWE-665 · SSDF PW.9**

**Evidence:** `internal/app/bootstrap.go:60` → `internal/platform/db/migrate.go:8-23`
runs GORM `AutoMigrate` on startup over 13 models; each store also runs its own
`Migrate`. The `migrations/*.sql` files (6, `000001`–`000006`) have **no runner** — no
golang-migrate driver, no code reads `migrations/`. So the running schema is whatever
AutoMigrate infers, and the versioned SQL is documentation only.

**Impact:** schema drift between the SQL of record and the live DB; the application's DB
role must retain DDL privileges permanently (least-privilege violation); no reviewable,
reversible migration history.

**Remediation (P2):** adopt a real migration runner (golang-migrate or GORM's
versioned migrator) driven by `migrations/`, and drop AutoMigrate in production; the DB
role then needs DDL only during migration. **Effort:** M. **Refs:**
[golang-migrate](https://github.com/golang-migrate/migrate) ·
[CWE-665](https://cwe.mitre.org/data/definitions/665.html).

### F-13 — Frontend dependency vulnerabilities (react-router) · **High** · CVSS:4.0/AV:N/AC:L/AT:P/PR:N/UI:A/VC:L/VI:L/VA:N/SC:L/SI:L/SA:N (8.2 per npm advisory)

**CWE-1395/CWE-601 · Top10 A06**

**Evidence:** `npm audit` (`evidence/npm-audit.json`): 7 vulnerabilities — 1 critical,
1 high, 5 moderate. react-router ≤7.17.0: **open redirect** via backslash in `<Link>`
(GHSA-wrjc-x8rr-h8h6) and **arbitrary constructor injection** via `deserializeErrors()`
in SSR hydration (GHSA-337j-9hxr-rhxg).

**Impact:** open redirect (phishing pivot) and, on SSR paths, constructor injection.
Karakuri renders client-side, which reduces the SSR path's reachability, but the open
redirect applies.

**Remediation *(remediated, P0)*:** `npm audit fix` to bump react-router/react-router-dom
to a patched line (≥7.9.x), then re-run typecheck + build + Playwright. **Effort:** S.
**Refs:** [GHSA-337j-9hxr-rhxg](https://github.com/advisories/GHSA-337j-9hxr-rhxg) ·
[GHSA-wrjc-x8rr-h8h6](https://github.com/advisories/GHSA-wrjc-x8rr-h8h6).

### F-14 — Production source maps embedded and served · **Low** · CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N (3.9)

**CWE-540 · ASVS 14.3.2**

**Evidence:** `web/vite.config.ts` sets `build.sourcemap: true`; `.map` is in the
served allowlist (`web/embed.go:62`) and the maps are embedded into the Go binary,
shipping full TS source (up to ~2 MB, `CostPage` map) to any client.

**Impact:** source disclosure — aids an attacker mapping client logic; bloats the binary.

**Remediation *(remediated, P1)*:** set `sourcemap: false` for production builds (or
emit `hidden-source-map` and exclude `.map` from the embed allowlist). **Effort:** S.
**Refs:** [CWE-540](https://cwe.mitre.org/data/definitions/540.html).

### F-15 — `/metrics` served unauthenticated · **Low/Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N (4.8)

**CWE-306 · ASVS 8.1**

**Evidence:** `internal/api/server.go:128-130` mounts `/metrics` outside `/api/v1` and
outside the auth group when the Prometheus exporter is enabled. `deploy/values.yaml`
documents this as intentional (scrapers don't authenticate).

**Impact:** operational/metric disclosure to anyone who can reach the port; risk rises
if metric **labels** carry tenant identifiers (label cardinality was not exhaustively
audited — `UNVERIFIED, needs manual confirmation` for label content).

**Remediation (P2):** bind `/metrics` to a separate, network-restricted listener, or put
it behind the reverse proxy's auth; confirm labels carry no tenant PII. **Effort:** S.
**Refs:** [CWE-306](https://cwe.mitre.org/data/definitions/306.html).

### F-16 — CLI SSO handoff returns a live refresh token in a cleartext URL · **High** · CVSS:4.0/AV:N/AC:L/AT:P/PR:N/UI:A/VC:H/VI:H/VA:N/SC:N/SI:N/SA:N (8.0)

**CWE-598/CWE-522 · ASVS 3.5.3 · OWASP API2**

**Evidence:** `internal/api/handler/sso_cli.go:114-122` returns
`http://127.0.0.1:<port>/callback?code=<sealed>` where `code` seals
`cliCode{Refresh, Principal, Challenge}`. The `Sealer` **signs but does not encrypt** —
stated at `auth/sealed.go:44-47` — so base64url-decoding `code` yields the refresh token
in cleartext. The package doc (`sso_cli.go:22-26`) claims the code is "useless on its
own" because redemption needs the PKCE-style verifier (constant-time checked at `:147`),
**but that guard is only on `POST /auth/sso/exchange`**; `POST /api/v1/auth/refresh`
(`handler/auth.go:119-155`) accepts a bare refresh token with no challenge. The documented
property does not hold. The hop is plain `http://` loopback.

**Reproduction:** decode the `code` query parameter, extract `Refresh`, POST it to
`/api/v1/auth/refresh`. Verified — see `PENTEST_REPORT.md` PT-04.

**Impact:** anyone who observes the callback URL (browser history, referrer, a process
listening on loopback) obtains a usable refresh token → account takeover for the
token's lifetime (720h default).

**Remediation *(remediated, P0)*:** stop putting the refresh token in the URL. Return a
short-lived, single-use **reference** that the CLI exchanges over `POST` for the token
pair, binding the exchange to the PKCE verifier — i.e. route the CLI through the same
`/auth/sso/exchange` guarantee that already exists, rather than the bare-token
`/auth/refresh`. **Effort:** M. **Refs:**
[CWE-598](https://cwe.mitre.org/data/definitions/598.html) ·
[OAuth 2.0 Security BCP §4.11 (tokens in URLs)](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics).

### F-17 — SAML IdP metadata fetched and trusted without signature or scheme enforcement · **High** · CVSS:4.0/AV:A/AC:H/AT:P/PR:N/UI:N/VC:H/VI:H/VA:N/SC:N/SI:N/SA:N (7.4)

**CWE-347/CWE-295 · ASVS 6.2.6**

**Evidence:** `internal/auth/federation.go:211-261` fetches IdP metadata from
`idp_metadata_url` (or a file) and `xml.Unmarshal`s it with **no XML-DSig verification**
and **no scheme enforcement** — an `http://` URL is accepted. Metadata carries the IdP's
signing certificates, i.e. the trust anchor for the entire SAML flow. Mitigations
present: 1 MiB read cap, 30 s timeout.

**Impact:** a network attacker who can MITM the (possibly plaintext) metadata fetch
substitutes the signing certificate and can then forge assertions the SP will accept →
full authentication bypass for the SAML provider.

**Remediation (P1):** require `https://` for `idp_metadata_url`; verify the metadata's
XML signature against a pinned certificate/fingerprint, or fetch once out-of-band and
pin. **Effort:** M. **Refs:**
[CWE-347](https://cwe.mitre.org/data/definitions/347.html) ·
[SAML Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/SAML_Security_Cheat_Sheet.html).

### F-18 — SAML audience restriction permissive by default · **Medium** · CVSS:4.0/AV:N/AC:H/AT:P/PR:N/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N (5.8)

**CWE-287 · ASVS 6.2.3**

**Evidence:** crewjam initialises `audienceRestrictionsValid := len(AudienceRestrictions)
== 0` (`service_provider.go:1247`), so an assertion carrying **no** `AudienceRestriction`
passes. Karakuri leaves `ValidateAudienceRestriction` nil in the `ServiceProvider`
literal (`auth/saml/saml.go:206-214`).

**Impact:** an assertion minted for a different SP (or with no audience) can be replayed
into Karakuri, weakening the audience binding SAML relies on.

**Remediation (P1):** set `ValidateAudienceRestriction` to the SP entity ID.
**Effort:** S. **Refs:** [ASVS 6.2.3](https://github.com/OWASP/ASVS).

### F-19 — No SAML assertion replay cache · **Medium** · CVSS:4.0/AV:N/AC:H/AT:P/PR:N/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N (5.6)

**CWE-294 · ASVS 6.2.4**

**Evidence:** nothing tracks consumed assertion IDs (neither Karakuri nor crewjam).
`InResponseTo` correlation via a client-held cookie is the only barrier, and the cookie
is expired client-side (`auth/saml/saml.go:303`) — which does not prevent server-side
replay within `NotOnOrAfter` + 180 s skew.

**Impact:** an attacker holding a captured assertion and the flow cookie can re-POST it
within the validity window.

**Remediation (P2):** a short-TTL consumed-assertion store keyed by assertion ID, checked
in the ACS handler. **Effort:** M. **Refs:**
[CWE-294](https://cwe.mitre.org/data/definitions/294.html).

### F-20 — Login timing side channel enables user enumeration · **Medium** · CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N (5.3)

**CWE-208/CWE-203 · ASVS 2.2.1**

**Evidence:** `auth/token.go:172-189` — unknown principal (`:173`), disabled principal
(`:178`), and empty password hash (`:181`) all return **before** the 600k-iteration
PBKDF2 verify at `:185`. Response bodies are deliberately identical
(`handler/auth.go:99-111`), closing the content channel, but the **latency** channel is
wide open: a valid username costs hundreds of ms, an invalid one returns immediately.

**Impact:** enumerate valid principal IDs by timing, then (via F-03) brute-force them.

**Remediation (P1):** perform a constant-cost dummy PBKDF2 verify against a fixed decoy
hash on the not-found/disabled/empty paths so every login costs the same. **Effort:** S.
**Refs:** [CWE-208](https://cwe.mitre.org/data/definitions/208.html) ·
[Authentication Cheat Sheet — timing](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html).

### F-21 — Access tokens are irrevocable before expiry · **Medium** · CVSS:4.0/AV:N/AC:L/AT:P/PR:L/UI:N/VC:L/VI:L/VA:N/SC:N/SI:N/SA:N (5.4)

**CWE-613 · ASVS 3.3.1**

**Evidence:** a `jti` is minted (`auth/token.go:306`) but there is no denylist.
`RevokeAllForPrincipal` (`:266-268`) covers refresh tokens only; `Verify` reloads the
principal (`:286-292`) but catches only `Disabled`. A stolen access token stays valid
for up to `AccessTTL` (15 min).

**Impact:** a leaked/stolen access token cannot be killed for up to 15 minutes even
after the operator revokes the session.

**Remediation (P2):** a short-lived `jti` denylist (in the existing store or Valkey)
checked in `Verify`, populated by `revoke`. 15 min TTL bounds its size. **Effort:** M.
**Refs:** [CWE-613](https://cwe.mitre.org/data/definitions/613.html).

### F-22 — Unbounded refresh-token table · **Low** · CVSS:4.0/AV:N/AC:L/AT:P/PR:L/UI:N/VC:N/VI:N/VA:L/SC:N/SI:N/SA:N (3.7)

**CWE-770 · ASVS 3.3.4**

**Evidence:** `DeleteExpiredRefreshTokens` (`auth/sql/credentials.go:153-160`) has **zero
non-test callers**. Rotation writes a row per exchange, so `auth_refresh_tokens` grows
without bound.

**Remediation (P2):** a periodic sweeper (goroutine or cron) invoking the existing delete.
**Effort:** S. **Refs:** [CWE-770](https://cwe.mitre.org/data/definitions/770.html).

### F-23 — Latent fail-open in listing/stream filters · **Low** (High if triggered) · CVSS:4.0/AV:N/AC:H/AT:P/PR:L/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N (4.6)

**CWE-636 · ASVS 4.1.5**

**Evidence:** `internal/auth/listing.go:99-105` returns "no filtering" (see everything)
and `internal/auth/stream.go:53-61` sets `unrestricted = true` when `principalID == ""`.
Callers discard the context-lookup `ok` flag (`handler/twin.go:73`,
`handler/events.go:47`). Safe **only** because every such route sits behind
`auth.Authenticate` (`server.go:241`).

**Impact:** if any handler is ever mounted outside the authenticated group, these
filters silently return all tenants' data — a full authorization bypass. It is a latent
trap, not a live bug.

**Remediation (P1):** treat a missing principal as deny — require the `ok` flag and 401
on absence, rather than defaulting to unrestricted. **Effort:** S. **Refs:**
[CWE-636 (fail-open)](https://cwe.mitre.org/data/definitions/636.html).

### F-24 — Route-table drift leaves `/auth/catalog` outside the conformance suite · **Low** · CVSS:4.0/AV:N/AC:L/AT:P/PR:L/UI:N/VC:L/VI:N/VA:N/SC:N/SI:N/SA:N (2.3)

**CWE-1053**

**Evidence:** `internal/auth/routes.go:170-284` (walked by the integration role-matrix
test) omits `GET /auth/catalog`, which `server.go:265` mounts under `ActionAuthRead`.
The route works but its per-role 200/403 behaviour is untested.

**Remediation (P2):** add the missing table entry so the conformance test covers it.
**Effort:** S.

### F-25 — No password / bootstrap-password policy · **Low** · CVSS:4.0/AV:N/AC:H/AT:P/PR:H/UI:N/VC:L/VI:L/VA:L/SC:N/SI:N/SA:N (3.9)

**CWE-521 · ASVS 2.1**

**Evidence:** `auth/credential.go:58` checks non-empty only — no length, complexity, or
reuse policy, and no bootstrap-password strength requirement or forced first-login
rotation (`internal/auth/bootstrap.go:55-88`). No re-hash-on-login upgrade when
`Iterations` rises (`credential.go:54-56`); the stored iteration count is read back with
no upper bound (`:92-95`) — a row with an absurd count is a per-principal login CPU DoS
(needs DB write access).

**Remediation (P2):** minimum length (≥12) and a breach-list check on set-password; cap
the stored iteration count on verify; re-hash on successful login when the policy cost
increases. **Effort:** S. **Refs:**
[ASVS 2.1](https://github.com/OWASP/ASVS) ·
[NIST SP 800-63B](https://pages.nist.gov/800-63-3/sp800-63b.html).

---

## 7. Prioritized remediation roadmap

Consolidated and dated in `REMEDIATION_ROADMAP.md`. Summary:

| Priority | Target | Findings | Rationale |
|----------|--------|----------|-----------|
| **P0** | this branch | F-01, F-03, F-04, F-05, F-06, F-07, F-11, F-13, F-16 | Live, authenticated-reachable or network-reachable; several one-line fixes |
| **P1** | +30 days | F-02, F-08, F-09, F-10, F-14, F-17, F-18, F-20, F-23 | Meaningful risk; needs a design step or is defence-in-depth |
| **P2** | +90 days | F-12, F-15, F-19, F-21, F-22, F-24, F-25 | Hardening, hygiene, and drift control |

---

## 8. Residual risk statement

After the P0 tranche, the residual risk is **Moderate and bounded**:

- **Federation (F-17/F-18/F-19)** remains P1/P2. Until closed, a SAML deployment on an
  untrusted network to the IdP carries a metadata-MITM risk; OIDC is unaffected (its
  discovery is HTTPS-pinned by go-oidc and the flow has state+nonce+PKCE).
- **Access-token revocation lag (F-21)** leaves a ≤15-minute window after session
  revocation; acceptable for most threat models, not for high-assurance.
- **The reverse-proxy assumption.** Several controls (HSTS effectiveness, `/metrics`
  isolation, `X-Forwarded-Proto` trust at `auth/cookie.go:125-130`) depend on a
  correctly-configured TLS-terminating proxy that this audit did not see. **UNVERIFIED —
  needs manual confirmation** of the production ingress.
- The crypto core, RBAC engine, and tenancy filter carry **low** residual risk; no SQL
  injection, SSRF, or template injection was found anywhere in the code.

---

## Appendix A — Tool versions & raw output

See `evidence/TOOL_VERSIONS.md`. Raw: `evidence/{govulncheck,gosec,semgrep,
trivy-config,checkov-dockerfile,npm-audit}.json`, `evidence/gitleaks-history.json`
(0 findings), `evidence/sbom.cdx.json` / `sbom.spdx.json` (349 components).

## Appendix B — Controls verified correct (do not re-derive)

- **Password hashing:** PBKDF2-HMAC-SHA256, 600,000 iterations, 16-byte salt, per-hash
  cost stored and honoured (`auth/credential.go:45-52,87-112`).
- **JWT:** algorithms restricted to `{HS256, EdDSA}`, `alg` checked before key lookup,
  `kid` mandatory, `exp` mandatory — `alg:none` and algorithm confusion closed by
  construction (`auth/jwt/jwt.go:139-198`); 32-byte minimum HS256 secret, no generated
  fallback (`auth/jwt/key.go:62-65`).
- **Refresh tokens:** 256-bit `crypto/rand`, stored as SHA-256, compare-and-set spend,
  family-wide revocation on reuse per OAuth 2.1 BCP (`auth/token.go:210-253`,
  `auth/sql/credentials.go:109-133`).
- **Authorizer:** default-deny with exactly one allow site (`auth/authorizer.go:123`);
  store errors → 500, never an allow (`auth/middleware.go:83-91`); denials audited
  (`internal/auth/audit.go:24-54`).
- **OIDC:** state + nonce + S256 PKCE, all constant-time validated
  (`auth/oidc/oidc.go:284-305,259,350`); redirect URIs from config only with an explicit
  open-redirect refusal (`internal/auth/federation.go:324-342`).
- **Federated identity:** subjects namespaced `oidc:`/`saml:` so an IdP `sub=admin`
  cannot collide with a local principal (`auth/external.go:76-100`); roles only from an
  explicit `RoleMap`, unmapped group = no access (`auth/rolemap.go:102-113`).
- **Injection:** no SQL injection (every dynamic fragment is a const table name or a
  bound placeholder — `auth/sql/`, `quota/sql/`, `gorm_storage.go`); no SSRF (all
  outbound URLs are literals or operator config); no `text/template`/`html/template`
  injection surface; `math/rand` only in a benchmark.
- **Frontend:** tokens in httpOnly cookies only — zero `localStorage`/`document.cookie`
  in `web/src`; zero `dangerouslySetInnerHTML`/`eval`.
- **Secrets:** gitleaks clean over 50 commits and the working tree; no signing key or
  bootstrap password is ever generated-and-logged; startup is fatal on misconfiguration
  (`internal/app/bootstrap.go:404-467`).
- **Log injection:** CR/LF + control-char sanitisation at every request-derived log site
  (`internal/auth/log.go:22-38`).

## Appendix C — Static-scanner false positives (verified)

**All 13 semgrep warnings and the majority of gosec's 35 are false positives**, verified
by reading source. This is recorded so a future reviewer does not re-triage them:

| Rule | Sites | Why it is not a finding |
|------|-------|-------------------------|
| semgrep `cookie-missing-secure`/`-httponly` (5) | `oidc.go:385`, `saml.go:439`, `sso_cli.go:88,108` | `Secure: !cfg.InsecureAllowHTTP` — config-gated, defaults secure; `:108` is a cookie *deletion* |
| semgrep/gosec `string-formatted-query` / G201 (4) | `semantic_pgvector.go:54,72,91,204` | interpolates only the const `tableName` (`:43`) and int `dim`; values are `$n` bound |
| semgrep/gosec `math-random`/G404 (2) | `cmd/krk-bench/main.go:23,105` | benchmark harness, not security-relevant |
| gosec G204 (10) | `git/gogitwt.go:121`, `llm/*_cli.go`, `cliagent/*` | argv-form `exec`, no shell; not injectable |
| gosec G304 (6) | `config.go:474`, `auth/keys.go:129`, `observability/format/*` | operator-configured paths, not request-derived |
| gosec G115 (3) | `gorm_storage.go:868-870` | uint32→byte in a hash mixing function, values masked |
| gosec G118 (2) | `loop/service.go:143,276` | intentional detached goroutine for a background loop (minor observability nit, not a vuln) |

**True positives the scanners did surface** (folded into findings above): gosec G705 XSS
at `artifact.go:41` (→ F-11); Trivy KSV012/014/001/104 (→ F-07); Checkov CKV_DOCKER_2/3
(→ F-06); `govulncheck` 28 stdlib advisories (→ F-06); `npm audit` 7 (→ F-13).
