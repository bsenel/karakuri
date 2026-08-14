# Karakuri — Refactor Report

**Date:** 2026-08-13 · **Scope:** guardrail repairs + lint hygiene (Workstream 1, descoped)
**Baseline commit:** `ec5a795` · **Standard:** [Refactoring catalog](https://refactoring.com/catalog/) · [Conventional Commits](https://www.conventionalcommits.org/)

## Why this workstream was descoped

The original brief asked for a full refactoring programme (dead code → duplication →
oversized functions → naming → control flow → module boundaries → type tightening
across ~37k LOC). **That programme was deliberately rejected**, with the user's
approval, for three evidenced reasons:

1. **The suite is green and the code is already disciplined.** `go test ./...` passes
   across all 10 modules; the codebase visibly follows the YAGNI/KISS/DRY rules in
   `AGENTS.md`, with clean layering enforced by ADRs 001–012.
2. **Churn would bury the security diffs.** This branch's primary purpose is closing
   live vulnerabilities (F-01, F-03, F-16, …). A 37k-LOC cosmetic sweep landing
   alongside those fixes would make the security changes unreviewable.
3. **The highest-value cleanup was not "make it prettier" — it was "make the
   guardrails actually run."** Reconnaissance found that **two architectural
   guardrails `AGENTS.md` claims are enforced were silently not running** (F-08). That
   is a real defect, and fixing it is worth more than any amount of style churn.

So Workstream 1 was narrowed to: repair the dead lint config, supply the missing
enforcement script, and make the linter green (the small real issues it surfaces).

## What changed

### 1. Repaired the dead lint configuration (F-08)

`.golangci.yml` was **v1-format**, which golangci-lint **v2.5.0 refuses to load** —
so the linter had silently not run at all (CI only invoked `go vet`). Rewritten to v2
schema (`version: "2"`, `linters.settings`, `linters.exclusions.rules`), preserving the
team's exact intent:

- **`default: none`** + the same four linters the v1 config enabled
  (`govet`, `staticcheck`, `unused`, `depguard`). v2 otherwise enables a wider default
  set (`errcheck`, `ineffassign`, …) that would surface 45 pre-existing `errcheck`
  findings and block every PR on day one — expanding the set is a separate, deliberate
  decision (tracked in `CI_SECURITY_PIPELINE.md`).
- The two **depguard** architectural rules (no langchaingo in core/feature; no domains
  in core) are carried over unchanged.

**Result:** `golangci-lint run ./...` now loads and reports **0 issues** in all three
modules (root, auth, quota), where before it errored out and linted nothing.

### 2. Supplied the missing enforcement script (F-08)

`AGENTS.md` and `ADR 001` state the langchaingo import boundary is *"Enforced by
`scripts/check_langchaingo_imports.sh`"* — but `scripts/` did not exist. Added the
script (dependency-free `bash`, suitable for pre-commit and CI). It and the depguard
rule now enforce the same boundary from two angles.

### 3. Fixed the small real issues the revived linter surfaced

With the linter running again, it found **7 genuine issues** (5 `staticcheck`,
2 `unused`) that had accumulated while it was dark. All are behaviour-preserving and
verified by the existing suite:

| File | Issue | Fix |
|------|-------|-----|
| `internal/feature/loop/service.go:17-18` | `ST1019` same package imported twice (`loop` + `coreloop`) | drop the `coreloop` alias, use `loop` |
| `internal/feature/loop/reason.go:44` | `S1001` copy loop | `copy(memEntries, sc.memEntries)` |
| `internal/platform/executor/celery.go:52` | `unused` field `err` | removed |
| `internal/quota/tiers.go:99` | `unused` const `sweepInterval` (+ now-unused `time` import) | removed |
| `cli/client/sso.go:148` | `QF1008` redundant embedded selector | `out.session(…)` |
| `internal/auth/store.go:22` | `QF1008` redundant embedded selector | `db.Name()` |
| `auth/condition.go:77` | `QF1002` expression switch → tagged switch on `r.Owner` | tagged switch |

### 4. Enabler for the security fixes

The F-02 remediation (a design change, tracked P1) will need a shared per-resource
authorization helper. Rather than pre-build an abstraction with one caller (a YAGNI
violation the `AGENTS.md` rules explicitly warn against), that helper is deferred to
the F-02 implementation itself, where it will have its real call sites. Recorded here
so the intent is not lost.

## Before / after metrics

| Metric | Before | After | Note |
|--------|-------:|------:|------|
| `golangci-lint` loads | **no** (v1 config rejected) | **yes** | the core repair |
| Lint issues surfaced (root/auth/quota) | 0 (nothing ran) | **0 / 0 / 0** | after fixing the 7 real ones |
| Enforcement scripts referenced by AGENTS.md that exist | 0 of 1 | **1 of 1** | `check_langchaingo_imports.sh` |
| `go test ./...` (10 modules) | green | **green** | behaviour preserved |
| Module coverage gates (90–95%) | met | **met** | auth 95.8% |
| Net Go LOC (this workstream) | — | **−9** | dead code removed, no features added |

## Matrix D — Refactor Candidate Matrix

Priority score = (Impact × 2) − Risk. Only the guardrail/lint candidates were approved;
the broad cleanup candidates were **rejected** with reasons, per the descope.

| ID | File / module | Smell | Metric before | Impact (1–5) | Risk (1–5) | Coverage % | Effort | Priority | Action | Approved? |
|----|---------------|-------|---------------|:------------:|:----------:|:----------:|:------:|:--------:|--------|:---------:|
| D1 | `.golangci.yml` | Dead config; linter never ran | v1 schema, load error | 5 | 1 | n/a | S | **9** | Migrate to v2, `default: none` + 4 linters | ✅ |
| D2 | `scripts/` | Enforcement script AGENTS.md claims, absent | 0 of 1 exist | 4 | 1 | n/a | S | **7** | Add `check_langchaingo_imports.sh` | ✅ |
| D3 | 7 lint sites (table above) | Dead code, dup import, copy-loop | 7 findings | 3 | 1 | ≥90 | S | **5** | Apply staticcheck/unused fixes | ✅ |
| D4 | `internal/platform/storage/gorm_storage.go` (874 LOC) | Largest file | 874 LOC | 2 | 4 | high | L | **0** | Split by aggregate | ❌ — green, cohesive, high-churn risk; would bury security diffs |
| D5 | `config/config.go` (784 LOC) | Large config loader | 784 LOC | 2 | 3 | good | M | **1** | Extract per-section loaders | ❌ — mostly declarative struct mapping; low readability gain |
| D6 | `internal/api/handler/quota.go` (636 LOC) | Large handler | 636 LOC | 2 | 3 | good | M | **1** | Split handler | ❌ — one cohesive resource; splitting adds indirection |
| D7 | Whole tree | Naming/comment hygiene sweep | — | 1 | 2 | — | L | **0** | Rename/reword | ❌ — code already reads well; net-negative churn |
| D8 | Whole tree | `errcheck` adoption (45 findings) | 45 unchecked errs | 3 | 3 | — | L | **3** | Enable errcheck, fix 45 | ❌ *deferred* — real value, but a baselined rollout via CI, not this branch |

**Rejected-candidate rationale (D4–D8):** every rejected item scores low on
`(Impact×2)−Risk` because the code is already clear and tested, so the readability
upside is small while the risk of churn — and of obscuring the security review — is
real. D8 (errcheck) is the one with genuine merit; it is deferred to a **baselined**
CI rollout (`CI_SECURITY_PIPELINE.md`) so existing findings do not block work on day one,
rather than a 45-site sweep bundled into a security branch.

## Verification

```
golangci-lint run ./...                 # root: 0 issues (was: config load error)
(cd auth && golangci-lint run ./...)    # 0 issues
(cd quota && golangci-lint run ./...)   # 0 issues
./scripts/check_langchaingo_imports.sh  # OK
go build ./... && go test ./...         # green, all 10 modules
```
