# Karakuri — Compliance Readiness Assessment

**Date:** 2026-08-13 · **Baseline:** `ec5a795` · **Assessor:** automated review (agent)
**Frameworks in scope (user-approved):** SOC 2 (Trust Services Criteria) · NIST SSDF SP 800-218 · GDPR · ISO/IEC 27001:2022 Annex A · EU Cyber Resilience Act (CRA)

> **This is an internal readiness assessment, not a certification, an audit opinion, or
> legal advice.** It evaluates only what is verifiable from this repository and pipeline.
> Certification against any of these frameworks requires an accredited auditor (SOC 2 /
> ISO), a Data Protection Officer / legal review (GDPR), or a conformity-assessment body
> (CRA), plus organizational evidence that does not live in code. Nothing here should be
> read as a claim of compliance.

## How to read this

Every control below is tagged:

- **[Code]** — verifiable from this repository (implemented, partially implemented, or not).
- **[Org]** — an organizational/process control that **cannot be evidenced from code**
  (policies, training, access reviews, vendor contracts, incident-response drills). These
  are listed, not scored, so nothing is silently dropped — but they are explicitly
  **out of scope for a code audit** and must be evidenced separately.

Verdicts: `Met` · `Partially met` · `Not met` · `N/A` · `Cannot verify from code`.

---

## 1. Data-flow inventory (GDPR Art. 30 foundation)

What personal/sensitive data the system handles, where it lives, and where it goes.

| Data | Category | Where collected | Where stored | Retention | Third-party processors | Cross-border |
|------|----------|-----------------|--------------|-----------|------------------------|--------------|
| Principal ID, display name | Identifier | Login / user creation (`handler/auth.go`) | `auth_principals` (SQLite/Postgres) | Life of account | — | Operator-hosted |
| Email, IdP attributes | PII | OIDC/SAML claims (`auth/provision.go:147-170`) | `auth_principals.attrs` | Life of account | Configured IdP (Keycloak/Okta/Azure) | Depends on IdP |
| Password hash | Credential | Set-password (`auth/credential.go`) | `auth_credentials` (PBKDF2, never plaintext) | Life of account | — | Operator-hosted |
| Refresh token (hashed) | Credential | Login (`auth/token.go`) | `auth_refresh_tokens` (SHA-256) | `refresh_ttl` 720h; **no sweeper (F-22)** | — | Operator-hosted |
| Per-tenant usage & cost ledger | Usage/billing | Quota/cost recording | `quota_*`, `cost_*` tables | `cost_retention_days` 90 | Optional LiteLLM gateway | Operator-hosted |
| Tool/audit events | Activity log | Authorization + tool calls (`internal/auth/audit.go`) | audit tables | Unbounded (no policy) | — | Operator-hosted |
| Logs & metrics | Operational (may contain IDs) | Everywhere | Local files + remote exporters | `max_age_days` 30 (local) | **Datadog, New Relic, Elasticsearch, Loki, AWS CloudWatch/S3, OTLP** (opt-in) | **Yes, if those exporters are enabled** |

**Key GDPR-relevant observations:**
- Personal data (email, IdP attributes) is minimal and purpose-bound (authorization).
- The **log fan-out is the main cross-border / processor exposure**: enabling Datadog,
  New Relic, Elasticsearch, Loki, CloudWatch or OTLP sends operational data (which can
  include principal IDs) to third-party processors, potentially outside the EU. This needs
  a DPA with each and a transfer mechanism — an **[Org]** control.
- **No documented retention policy** for audit events (unbounded) or a data-subject
  erasure path (see G-04 below).

---

## 2. SOC 2 — Trust Services Criteria (technical subset)

Scored where code is evidence; the majority of SOC 2 is **[Org]** (governance, HR, vendor
management) and is listed in §7.

| TSC | Control | Verdict | Evidence / Gap |
|-----|---------|---------|----------------|
| CC6.1 | Logical access — authentication | **[Code] Partially met** | PBKDF2 passwords, JWT with pinned alg, OIDC/SAML federation. Gaps: no password policy (F-25), no login rate limit until this branch (F-03 fixed). |
| CC6.1 | Logical access — authorization | **[Code] Met** | Default-deny RBAC, single allow site, privilege-escalation guards (`auth/authorizer.go`, `grant.go`). Object-level scoping incomplete for some resources (F-02). |
| CC6.2 | Credential lifecycle | **[Code] Partially met** | Refresh rotation + family revocation; but no expired-token sweeper (F-22), access tokens irrevocable ≤15m (F-21). |
| CC6.6 | Transmission security | **[Code] Partially met** | Secure/HttpOnly/SameSite cookies; TLS assumed at proxy (not in repo). HSTS not emitted (by design). |
| CC6.7 | Encryption of secrets | **[Code] Met** | No secrets in repo (gitleaks clean, 50 commits); env-based; CLI handoff now encrypted (F-16). |
| CC7.1 | Vulnerability detection | **[Code] Partially met** | CodeQL + Dependabot existed; SCA/secret/IaC/SAST gates added this branch (WS6). |
| CC7.2 | Monitoring / anomaly detection | **[Code] Partially met** | Rich observability exporters; but no correlation IDs (E-03), no failed-login alerting (F-03 discussion). |
| CC7.2 | Audit logging | **[Code] Partially met** | Authorization denials audited with full trace (`internal/auth/audit.go`); log-injection sanitised. Gap: unbounded retention, no tamper-evidence. |
| CC8.1 | Change management | **[Code] Partially met** | Branch protection + CI gates + Conventional Commits; ADRs. Gap: no signed commits/provenance yet (WS6 release hardening). |
| A1.2 | Availability / resilience | **[Code] Partially met** | RetryExporter, checkpoint/resume loops. Gaps: no server timeouts until this branch (F-04), limiter fails open. |

---

## 3. NIST SSDF (SP 800-218) — the framework this repo can most directly evidence

| Practice | Control | Verdict | Evidence |
|----------|---------|---------|----------|
| PO.1/PO.3 | Define security requirements; toolchain | **[Code] Partially met** | `AGENTS.md`, ADRs, CI toolchain; lint was dark until F-08 fixed. |
| PS.1 | Protect code from unauthorized access | **[Org]** | Repo access controls — out of scope for code. |
| PS.2 | Provide provenance / integrity | **[Code] Not met → planned** | No SBOM attestation or signed releases yet; SBOM generated (WS2), signing + provenance in `CI_SECURITY_PIPELINE.md`. |
| PW.4 | Reuse secure components | **[Code] Partially met** | Dependabot; SCA (govulncheck/Trivy/npm audit) now gated. 28 stdlib CVEs in the shipped image fixed (F-06). |
| PW.5/PW.6 | Secure coding; static analysis | **[Code] Partially met** | CodeQL; gosec/semgrep/golangci-lint added. |
| PW.7 | Review code | **[Code] Met** | Branch protection + CODEOWNERS + PR CI. |
| PW.8 | Test executable code | **[Code] Met** | 90–95% coverage gates, integration (real IdP) + e2e suites. |
| PW.9 | Secure default configuration | **[Code] Partially met** | Fatal-on-missing-secret startup; Postgres `sslmode` default fixed to `require` (F-07). |
| RV.1 | Identify vulnerabilities | **[Code] Met (this assessment)** | 25 findings with remediation. |
| RV.2/RV.3 | Respond / remediate | **[Code] Partially met** | P0 fixed this branch; roadmap for the rest. SECURITY.md defines private disclosure. |

**SSDF is the strongest fit:** a majority of its practices are code/pipeline controls, and
most are Met or Partially met.

---

## 4. GDPR (technical measures — Art. 5, 25, 30, 32)

| Article | Requirement | Verdict | Evidence / Gap |
|---------|-------------|---------|----------------|
| Art. 5(1)(c) | Data minimisation | **[Code] Met** | Only auth-necessary PII (ID, email, IdP attrs) stored. |
| Art. 25 | Data protection by design/default | **[Code] Partially met** | httpOnly cookies, no PII in tokens, no client-side token storage; but log fan-out can export IDs to processors. |
| Art. 30 | Records of processing | **[Code] Partially met** | Data-flow inventory (§1) is the technical basis; the formal RoPA is **[Org]**. |
| Art. 32 | Security of processing | **[Code] Partially met** | Encryption of credentials, access control, resilience — see SOC 2 CC6. |
| Art. 17 | Right to erasure | **[Code] Not met (G-04)** | `DeleteUser` exists (`handler/auth.go`), but there is **no cascade** to audit/cost/memory rows tied to a principal, and no documented erasure workflow. |
| Art. 33 | Breach notification (72h) | **[Org]** | Process control; SECURITY.md covers researcher disclosure, not controller→DPA notification. |
| Ch. V | International transfers | **[Org] + [Code] flag** | Enabling remote exporters transfers data to third-country processors; needs SCCs/DPAs. Flagged in §1. |

---

## 5. ISO/IEC 27001:2022 — Annex A (technical controls only)

The bulk of ISO 27001 is an ISMS (Clauses 4–10) and organizational Annex A controls —
**[Org]**, listed in §7. Technical Annex A controls verifiable from code:

| Control | Name | Verdict | Evidence |
|---------|------|---------|----------|
| A.8.2/8.3 | Privileged & information access | **[Code] Met** | RBAC with least-privilege grants, `MayGrant` bounding. |
| A.8.5 | Secure authentication | **[Code] Partially met** | MFA is **[Org]/IdP**; password + federation in code; rate limit added (F-03). |
| A.8.9 | Configuration management | **[Code] Partially met** | Helm values, config defaults; securityContext added (F-07). |
| A.8.24 | Use of cryptography | **[Code] Met** | PBKDF2, AES-GCM (F-16), HMAC, pinned-alg JWT; `crypto/rand` throughout. |
| A.8.25/8.28 | Secure development / coding | **[Code] Partially met** | ADRs, depguard, SAST; guardrails revived (F-08). |
| A.8.16 | Monitoring activities | **[Code] Partially met** | Observability + audit log; correlation IDs missing (E-03). |
| A.8.8 | Management of technical vulnerabilities | **[Code] Partially met** | Dependabot + SCA gates (WS6); this assessment. |

---

## 6. EU Cyber Resilience Act (CRA) — essential requirements

CRA applies if Karakuri ships as a product with digital elements into the EU. Selected
Annex I essential requirements:

| Requirement | Verdict | Evidence / Gap |
|-------------|---------|----------------|
| Secure by default | **[Code] Partially met** | Fatal-on-missing-secret, secure cookies, securityContext (F-07); Postgres TLS default fixed. |
| No known exploitable vulnerabilities at release | **[Code] Partially met** | Shipped image's 28 stdlib CVEs fixed (F-06); SCA now gated. |
| Protect confidentiality/integrity | **[Code] Met** | Encryption at rest for credentials, RBAC, encrypted handoff (F-16). |
| Minimise attack surface | **[Code] Partially met** | Non-root container, dropped caps, read-only FS (F-07); `/metrics` public by design (F-15). |
| **SBOM** | **[Code] Met (this branch)** | CycloneDX + SPDX generated (`evidence/sbom.*.json`); attestation planned (WS6). |
| Coordinated vulnerability disclosure | **[Code] Met** | `.github/SECURITY.md` (private reporting, 5-day response). |
| Security updates / patchability | **[Org] + [Code]** | Dependabot; documented supported-versions policy in SECURITY.md; update mechanism is **[Org]**. |
| Vulnerability handling & reporting to ENISA | **[Org]** | Process control — out of scope for code. |

---

## 7. Organizational controls — out of scope for a code audit (listed, not dropped)

These are required by the frameworks above but **cannot be evidenced from this repository**.
They must be satisfied and evidenced separately before any certification:

- **Governance:** security policy, risk assessment/treatment, Statement of Applicability
  (ISO), management review, defined roles (SOC 2 CC1–CC5, ISO Clauses 4–10).
- **People:** background checks, security training, onboarding/offboarding, acceptable-use
  policy (SOC 2 CC1.4, ISO A.6).
- **Access reviews:** periodic recertification of who holds which grants (SOC 2 CC6.3).
- **Vendor / processor management:** DPAs with every enabled observability exporter and the
  IdP; SCCs for international transfers (GDPR Ch. V, SOC 2 CC9, ISO A.5.19–5.23).
- **Incident response:** documented IR plan, breach-notification workflow (GDPR 72h), drills
  (SOC 2 CC7.3–7.5, ISO A.5.24–5.28, CRA reporting to ENISA).
- **Business continuity / DR:** backup, restore testing, RTO/RPO (SOC 2 A1.2–A1.3, ISO A.5.29–5.30).
- **Physical & environmental** security of the hosting environment (ISO A.7).
- **DPO / DPIA, lawful basis, privacy notice, data-subject request process** (GDPR Arts. 6, 13–15, 35).

---

## 8. Control-to-framework cross-mapping

One piece of evidence often satisfies several frameworks. Highest-leverage mappings:

| Evidence (code) | SOC 2 | SSDF | GDPR | ISO 27001 | CRA |
|-----------------|-------|------|------|-----------|-----|
| RBAC default-deny + least privilege (`auth/authorizer.go`) | CC6.1/6.3 | PW.5 | Art. 25/32 | A.8.2/8.3 | Conf/Integrity |
| Cryptography (PBKDF2, AES-GCM, pinned JWT) | CC6.1/6.7 | PW.5 | Art. 32 | A.8.24 | Conf/Integrity |
| SBOM + SCA gates (WS2/WS6) | CC7.1 | PW.4/PS.2 | — | A.8.8 | SBOM / no-known-vulns |
| Secure defaults + securityContext (F-06/F-07) | CC8.1/A1.2 | PW.9 | Art. 25 | A.8.9 | Secure-by-default |
| Audit logging (`internal/auth/audit.go`) | CC7.2 | — | Art. 30 | A.8.15/8.16 | — |
| Rate limiting + timeouts (F-03/F-04) | CC6.1/A1.2 | PW.9 | Art. 32 | A.8.5 | Attack-surface |
| Private vuln disclosure (SECURITY.md) | CC7.4 | RV.1 | — | A.5.7 | CVD |

---

## 9. Priority readiness actions (technical)

1. **GDPR erasure cascade (G-04)** — implement principal deletion that cascades or anonymises
   audit/cost/memory rows, with a documented workflow. *(P1)*
2. **Retention policies** — bound audit-event retention; wire the refresh-token sweeper
   (F-22); document all retention windows. *(P1/P2)*
3. **Processor register + DPAs** — before enabling any remote exporter in production, record
   the processor and its transfer basis. *(Org, blocking for GDPR)*
4. **Release provenance** — SBOM attestation + signed releases (SSDF PS.2, CRA) via WS6. *(P1)*
5. **Access-review + IR runbooks** — the two highest-value [Org] controls to start. *(Org)*

## References
[AICPA TSC / SOC 2](https://www.aicpa-cima.com/topic/audit-assurance/audit-and-assurance-greater-than-soc-2) ·
[NIST SSDF SP 800-218](https://csrc.nist.gov/pubs/sp/800/218/final) ·
[GDPR text](https://gdpr-info.eu/) ·
[ISO/IEC 27001](https://www.iso.org/standard/27001) ·
[EU CRA](https://eur-lex.europa.eu/eli/reg/2024/2847/oj) ·
[NIST CSF 2.0](https://www.nist.gov/cyberframework) ·
[CIS Benchmarks](https://www.cisecurity.org/cis-benchmarks)
