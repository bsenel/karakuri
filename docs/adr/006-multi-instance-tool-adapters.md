# ADR 006: Multi-Instance Tool Adapters with Twin Bindings

## Status

Accepted

## Context

Phase 6 shipped real adapters for GitHub, Linear, Slack, Figma, Playwright, Google Calendar, and a multi-provider email slot (Gmail, Outlook, SMTP, Apple Mail). Each slot held exactly **one** adapter instance per server: one GitHub repo, one Slack workspace, one email account.

This precludes the multi-tenant use case: a single Karakuri instance serving multiple organizations, teams, or individuals — where Acme uses GitHub + Outlook, Beta uses GitLab + Gmail, and an individual uses a personal SMTP + Apple Mail account. Each tenant needs its own provider instance, and the loop must dispatch to the right one based on which twin owns the running objective.

The Phase 6 config also carried an asymmetry: most slots were flat top-level sections (`tools.github`, `tools.linear`) with implicit "first enabled wins" dispatch, while `email` already used a parent-slot-with-provider-selector shape (`tools.email.provider: gmail | outlook | smtp | apple_mail`). Converging on one shape was overdue regardless of multi-tenancy.

## Decision

1. **Slot is the abstraction; provider is the implementation.** Each adapter category — `versioncontrol`, `projectmgmt`, `messaging`, `design`, `testing`, `calendar`, `email` — is a *slot*: a stable name the engine, capabilities, and environments depend on. Providers (GitHub, Linear, Gmail, Outlook, …) are implementations of that slot.

2. **Pattern B universally.** Every slot uses the same parent-slot shape:

    ```yaml
    tools:
      <slot>:
        default: <instance_name>          # resolved when a twin has no binding for this slot
        instances:
          <instance_name>:
            type: <provider>              # github | linear | slack | gmail | …
            …provider-specific fields…
    ```

3. **Multi-instance co-existence.** A slot may hold any number of named instances simultaneously. The registry resolves an instance by name on demand; there is no implicit "first wins" rule.

4. **Twin-keyed routing.** `DigitalTwin.AdapterBindings map[string]string` maps slot → instance name. When a loop runs for a twin, environments capture the right adapter at construction time (factories now take `environment.BuildContext{TwinID, AdapterBindings}`). Twins without a binding for a slot fall back to that slot's `default`.

5. **Env-var credential references.** Each instance config field accepts a sibling `*_env` key whose value is an environment variable name. At config load, `resolveEnvRefs` copies the env value into the bare key (`token_env: ACME_GITHUB_TOKEN` → `token: <env value>`). Inline values stay supported for local development convenience.

6. **Adapter interfaces unchanged.** `VersionControlAdapter`, `EmailAdapter`, etc., keep their current shape. Only registry construction and environment factories changed.

## Consequences

- One Karakuri server can host arbitrarily many provider instances per slot. Multi-tenant deployments land without further infrastructure.
- Capability code and environment dispatch remain slot-keyed, never provider-keyed — domain pack isolation holds. `gitEnv.Act()` calls `e.vc.CreatePR(...)`; it never knows whether `e.vc` is GitHub or GitLab.
- Config files are hard-cut incompatible with the Phase 6 flat shape. Migration is mechanical (rename top-level sections, wrap in `instances:`); a sample multi-tenant config is documented in `config/default.yaml`.
- `/health` enumerates one row per (slot, instance) pair instead of one row per slot, so operators see exactly which providers are wired.
- Environment factories see the assigned twin's bindings once per loop run — no per-action lookup overhead. The factory signature changed from `Build(map[string]any)` to `Build(environment.BuildContext)`.
- Operators set bindings via `krk twin bindings <id> --set versioncontrol=acme_github --set email=acme_outlook` or `PUT /twins/:id/bindings`.

## Out of scope (deferred)

- Per-objective overrides beyond twin bindings.
- External secret backends (Vault, AWS Secrets Manager, sealed-secrets). `*_env` references suffice for v1.
- Hot reload of registry on config change. Server restart picks up new instances.
- Cross-instance failover. If `acme_github` is down there is no automatic switch to `acme_github_mirror`.

## Example

```yaml
tools:
  versioncontrol:
    default: acme_github
    instances:
      acme_github:     { type: github, repo: acme/api,      token_env: ACME_GITHUB_TOKEN }
      beta_gitlab:     { type: github, repo: beta/api,      token_env: BETA_GITLAB_TOKEN }
      personal_github: { type: github, repo: bsenel/sideproj, token_env: BSENEL_GH_TOKEN }
  email:
    default: acme_outlook
    instances:
      acme_outlook:   { type: outlook, from_address: bot@acme.com, oauth_token_env: ACME_MS_TOKEN }
      personal_gmail: { type: gmail,   from_address: bsenel@gmail.com, oauth_token_env: BSENEL_GOOGLE_TOKEN }
      bsenel_smtp:    { type: smtp,    host: smtp.fastmail.com, port: 587, username: bsenel@fastmail.com, password_env: BSENEL_SMTP_PASS }
```

```bash
krk twin create --name acme-eng --kind team
krk twin bindings <acme-id>    --set versioncontrol=acme_github     --set email=acme_outlook
krk twin create --name bsenel  --kind person
krk twin bindings <bsenel-id>  --set versioncontrol=personal_github --set email=bsenel_smtp
```

Two loops running concurrently — one per twin — produce PRs on the correct repos and notifications via the correct email provider.
