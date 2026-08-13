# ADR-013 — Versioned migrations replace runtime AutoMigrate

**Status:** Proposed (audit finding E-05 / SECURITY_AUDIT F-12) · **Date:** 2026-08-13

## Context

The server runs GORM `AutoMigrate` over 13 models at startup
(`internal/app/bootstrap.go:60` → `internal/platform/db/migrate.go:8-23`), and each
standalone store runs its own `CREATE TABLE IF NOT EXISTS`. A set of hand-written,
numbered SQL migrations exists (`migrations/000001..000006.{up,down}.sql`) but **has no
runner** — no golang-migrate driver, no code reads the directory. `internal/quota/tierstore.go`
even describes its SQL file as "a mirror of" the Go `Migrate` method.

Consequences of the status quo:
- The **live schema is whatever GORM infers**, which can drift from the SQL of record
  (indexes, constraints, column types are not guaranteed identical).
- The application's DB role must hold **DDL privileges permanently**, violating least
  privilege — a compromised app process can alter the schema.
- There is **no reviewable, reversible migration history** and no way to test a migration
  in isolation.

## Decision

Adopt a versioned migration runner (golang-migrate, or GORM's versioned migrator) driven by
`migrations/`, and **remove AutoMigrate from the production start path**:

- Migrations run as an explicit step (a `krk migrate` subcommand and/or an init container
  in the Helm chart), not implicitly on every server boot.
- The runtime DB role is scoped to **DML only**; a separate, migration-time role holds DDL.
- AutoMigrate may remain behind a `--dev` flag for local SQLite convenience.

## Consequences

- **+** Schema is reviewed, versioned, reversible, and identical across environments.
- **+** Least-privilege runtime DB role.
- **−** A migration step must run before/with each deploy (an init container or CI step).
- **−** One-time effort to reconcile the current AutoMigrate-produced schema into an
  authoritative baseline migration.

## References
[golang-migrate](https://github.com/golang-migrate/migrate) · [CWE-665](https://cwe.mitre.org/data/definitions/665.html) · [NIST SSDF PW.9](https://csrc.nist.gov/pubs/sp/800/218/final)
