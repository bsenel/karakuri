# Karakuri

An autonomous decision-making platform built on four primitives: **Capabilities**, **Environments**, **Objectives**, and **Agents**. The engine runs a continuous observe→reason→decide→act→verify→learn loop, accumulates cross-run memory, and escalates to humans only when confidence or authority bounds require it.

## Quick start

```bash
# Build
make build

# Start the server.
# Auth: set ANTHROPIC_API_KEY (or GOOGLE_API_KEY for Gemini). If unset and
# the matching CLI (`claude` / `gemini`) is installed and authenticated,
# Karakuri routes through the CLI as a transparent fallback — operators
# with `claude /login` already done can skip the env var entirely.
./bin/server

# Create a twin and objective, then run the loop
krk twin create --name "dev-team" --kind team --domain software
krk objective create --twin <twin-id> --title "implement JWT auth" --domain software
krk loop start <objective-id> --twin <twin-id>
krk loop status <loop-id>

# Approve a checkpoint when the loop escalates
krk loop resume <loop-id> --decision approve

# Continuous watch mode (creates watcher twin and streams SSE until Ctrl+C)
krk auto --domain software
```

## Architecture

Three-layer Go monolith + thin CLI:

```
cmd/              → server and krk binaries
internal/core/    → domain types and interfaces (zero vendor imports)
internal/feature/ → business logic (loop, memory, checkpoint, artifact, …)
internal/platform/→ vendor bindings (LangChain Go, GORM, go-git, OTel)
internal/api/     → HTTP handlers; delegates to feature services
domains/          → pluggable domain packs
cli/              → krk commands; thin HTTP client
```

See [docs/architecture.md](docs/architecture.md) for the full design and loop step breakdown.

## The Reasoning Loop

```
OBSERVE → REASON → DECIDE → ACT → VERIFY → LEARN
   ↑                                          │
   └──────────────────────────────────────────┘
```

- **Observe** — fan-out across all registered environments; recall episodic + semantic memory
- **Reason** — call the agent with world state + memory; parse JSON plan. Strategy is per-agent: ChainOfThought (default), ReAct, or **Reflexion** (Phase 13 — two-pass critique + revise; never regresses below the draft if the revision is unparseable)
- **Decide** — check authority bounds; bias confidence from procedural memory history; emit checkpoint if escalating. Every escalation writes a `tool_events` row tagged `kind=escalation, bounds_violation=true` for the audit log
- **Act** — execute actions; create isolated git worktrees for code-writing capabilities
- **Verify** — evaluate weighted success criteria via agent or environment results; emits a `per_domain_score` payload when criteria carry `Domain` tags
- **Learn** — write episodic + procedural memory; consolidate to semantic tier

Synthetic Reflexion-vs-ChainOfThought comparison lives in [`docs/benchmarks.md`](docs/benchmarks.md); regenerate with `go run ./cmd/krk-bench`.

## Standing Objectives

An objective converges once and stops. A **standing** objective is a desired
state Karakuri holds: it senses cheaply on a cadence, reconciles when the world
has drifted or a clock says to look anyway, and escalates whatever exceeds the
autonomy it has earned.

```bash
# Watch a repository and propose fixes, never acting on its own.
krk objective standing obj_123 --sense 15m --autonomy propose

# A weekday morning review that may act, having earned its way up to it.
krk objective standing obj_123 \
    --cron "0 8 * * 1-5" --timezone Europe/Istanbul \
    --sense 1h --resync 24h \
    --autonomy propose --ceiling act_with_notice --promote-after 5 \
    --quiet 22:00-07:00

krk objective reconcile-status obj_123   # what it has been doing
krk objective reconcile obj_123          # now, rather than on the cadence
krk objective pause obj_123 --reason "investigating a noisy adapter"
krk objective resume obj_123
```

**Two tiers, and the cheap one is why this is affordable.** Sensing calls
`Snapshot` on each environment and compares a composite hash against the one
taken when the objective last converged — a handful of adapter calls, no model
call, no tokens. The full loop runs only when that hash moved, a schedule came
due, or `--resync` expired. An objective can be checked every fifteen minutes
all year and spend money only on the days something happened.

An environment that returns no hash (a calendar, an inbox) is reported as
*blind* rather than as unchanged; those objectives are driven by their schedule
instead.

**Autonomy is a ladder, and the ceiling is yours.**

| Level | What it may do |
|-------|----------------|
| `sense` | Never reconciles. Watches, and raises a checkpoint when something moves. Costs no tokens. |
| `propose` | Plans, then escalates every action to a checkpoint. The default. |
| `act_with_notice` | The agent's own authority bounds apply; every autonomous action appears in the next digest. |
| `act` | The agent's bounds apply; only exceptions surface. |

`--promote-after N` earns one rung after N clean reconciles and never passes
`--ceiling`. One rejected checkpoint demotes immediately — a reviewer saying no
is a stronger signal than any number of runs nobody objected to. Both movements
are written to the audit log as `kind=promotion` / `kind=demotion`.

**Guardrails, on by default.** A database lease so two replicas never reconcile
the same objective (and never send the same report twice); `max_concurrent` on
simultaneous reconciles; a circuit breaker at three consecutive failures and a
stall detector at three reconciles that fail to move the score — both pause the
objective *and* raise a checkpoint, because an objective that goes quiet with no
explanation looks exactly like one that is content. `--quiet` and
`--min-interval` hold back the expensive tier only; sensing runs through the
night, which is how the morning reconcile knows what happened.

Configure the ceilings in `reconcile:` (see [config/default.yaml](config/default.yaml));
`reconcile.enabled: false` is the kill switch. Design rationale in
[ADR 015](docs/adr/015-standing-objectives-and-reconciliation.md).

> `krk loop start --watch` still works and means what it always did. It is now a
> standing objective at `sense` autonomy, which behaves the same and survives a
> server restart.

### Digests

A standing objective that works unsupervised should report unsupervised. A
digest covers a **twin** — one message a day, not one per objective — and ends
with the decisions it needs from you.

```bash
# A weekday morning brief to Slack.
krk report create --twin twin_1 --daily-at 08:00 --timezone Europe/Istanbul \
    --channel messaging --target '#eng-standup'

# See exactly what tomorrow's will say, without sending it.
krk report preview --twin twin_1 --window 24h
```

A digest is assembled from records that already exist — reconcile outcomes, the
audit log, pending checkpoints, the cost ledger — so the same window produces
the same report tomorrow, and a preview is not an approximation of tonight's
delivery but a copy of it.

An agent writes the narrative and nothing else: it never decides what is in the
report or what counts as a decision. When no model is available the structured
rendering is delivered on its own, and when one is, that rendering is appended
beneath the prose.

Nothing is sent for a window in which nothing happened (`--send-when-empty`
opts in) — a daily mail that says "nothing happened" is a mail people stop
reading, and the cost is paid by the one that matters. Delivery goes through the
twin's bound adapter and is recorded in the audit log like any other action.
Enable the sender in `reports:`; it is off by default. Rationale in
[ADR 016](docs/adr/016-earned-autonomy-and-digests.md).

## Domain Packs

Domain packs encapsulate all field-specific knowledge. The core engine imports none of it.

| Pack | Status | Capabilities | Agents | Templates |
|------|--------|-------------|--------|-----------|
| `software` | Active (v1) | 20 | 7 | 7 |
| `agriculture` | Active (v1) | 8 | 2 | 2 |
| `healthcare` | Active (Phase 10) — strict authority bounds, compliance-aware verifiers | — | — | — |
| `legal`, `mechanical`, `consulting` | Stub | — | — | — |

Cross-domain objectives (Phase 13): set `additional_domains` on an Objective to have the loop runner recruit agents and environment factories across multiple packs in a single iteration; `Criterion.Domain` tags per-domain verify scores on the `loop.step_completed` event.

Validate any pack with:
```bash
krk domain test software
krk domain test agriculture
```

See [docs/domain-packs.md](docs/domain-packs.md) to author your own pack.

## Memory

Four tiers, all persisted across loop runs:

| Tier | Storage | Purpose |
|------|---------|---------|
| Working | in-process map | In-flight state within a loop run |
| Episodic | SQLite or Postgres | Iteration traces; recalled at observe step |
| Semantic | SQLite (keyword) or **pgvector** (Phase 8 — cosine-distance vector recall) | Consolidated facts; promoted from episodic |
| Procedural | SQLite or Postgres | Per-capability success rates; biases plan confidence at decide step |

**Retention scheduler** (Phase 13, disabled by default): a goroutine ticker calls `MemoryService.RunRetention(ctx, ...)` every `memory.retention.interval_minutes`. Per-tier TTLs (`working_ttl_minutes`, `episodic_ttl_days`, `semantic_ttl_days`) and a `semantic_min_score` confidence floor drop stale entries; enable it after measuring growth.

## CLI Commands

```
# Twins
krk twin create --name <name> --kind <person|team|org> --domain <id>
krk twin list
krk twin get <id>

# Organisations, teams and projects (Phase 17) — names resolve to IDs, which is
# what a binding stores, so renaming a container rewrites no policy
krk org create --name <name>
krk team create --org <org> --name <name>
krk team move <name> --org <from> --to-org <to>
krk project create --name <name>
krk project add-twin --project <name> --twin <id>
krk org list / krk team list [--org <org>] / krk project list

# Objectives
krk objective create --twin <id> --title <title> --domain <id> \
                     [--description <text>] [--max-iter N] [--priority N] \
                     [--template <id>]
krk objective list [--twin <id>] [--status <pending|active|completed|failed>]
krk objective get <id>

# Standing objectives (Phase 20) — an objective Karakuri holds rather than
# finishes. Sensing is free; reconciling is what costs.
krk objective standing <id> --sense 15m --every 1h --autonomy propose
krk objective reconcile-status <id>
krk objective reconcile <id>
krk objective pause <id> --reason "..."
krk objective resume <id>
krk objective unstanding <id>

# Digests (Phase 21) — per twin, ending with the decisions you owe
krk report create --twin <id> --daily-at 08:00 --channel messaging --target '#eng'
krk report preview --twin <id> --window 24h
krk report list [--twin <id>] / krk report send <id> / krk report delete <id>

# Loop (start returns JSON `{"loop_id":"..."}` with --output json; jq
# selector is `.loop_id`, not `.id`)
krk loop start <objective-id> --twin <id> [--max-iter N] [--watch]
krk loop status <loop-id>
krk loop resume <loop-id> --decision <approve|reject|modify> \
                          [--note <text>] [--approver <id>]

# Checkpoints — --note + --approver populate the Phase 13 audit row
krk checkpoint list [--twin <id>]
krk checkpoint get <id>
krk checkpoint resolve <id> --decision <approve|reject|modify> \
                            [--note <text>] [--approver <id>]

# Memory
krk memory store --agent <id> --tier episodic --content "..."
krk memory recall --query "..." [--tier episodic]
krk memory forget --before <date>

# Artifacts
krk artifact list [--objective <id>]
krk artifact get <sha>
krk artifact write --objective <id> --agent <id> --content "..."

# Domains
krk domain list
krk domain capabilities [--domain <id>]
krk domain test <domain-id>

# Research
krk research --topic "..." [--depth shallow|deep]

# Watch mode
krk auto [--domain <id>]

# Quotas (Phase 15)
krk quota config
krk quota show --twin <id>
krk quota reset --twin <id> [--capability <cap>]

# The limits themselves (Phase 19) — stored in the database; the config file seeds them
krk quota tiers
krk quota set --tier llm-tokens --cap 5000000 --reason "team grew to twelve"
krk quota unset llm-tokens

# Self-service limits and spend (Phase 18)
krk quota request --tier llm-tokens --twin <id> --cap 5000000 --reason "launch week"
krk quota requests list [--status pending] [--mine]
krk quota requests approve <request-id> [--note "..."]
krk quota requests reject <request-id> --note "..."
krk cost report [--since 720h] [--twin <id>] [--org <name> --team <name>] \
                [--provider <p>] [--group-by day,provider,model,label] [--limit N]

# Audit log (Phase 13)
krk audit [--kind execute|escalation|approval] [--objective <id>] \
          [--agent <id>] [--violations-only] [--since <RFC3339>] [--limit N]
```

## Web interface

The React SPA at `/` covers what `krk` does, for the work an operator does daily:
users and their bindings, the org/team/project tree, the role→permission matrix,
quota usage and the self-service queue, the limits themselves, spend, and the
audit log.

**The navigation shows only what you can open**, and where you land after signing
in depends on what you hold — an auditor with no objective access should not meet
a 403 as their first impression of a system behaving correctly. Hiding a link is
a courtesy rather than a control: the server refuses either way, and nothing
secret relies on a hidden menu item.

The cost dashboard follows a live stream (`GET /api/v1/events`) that is filtered
per subscriber against the same bindings that decide which twins you may list. An
event the server cannot classify is withheld rather than broadcast — see
[ADR 012](docs/adr/012-limits-as-resolved-state.md).

## Configuration

Copy `config/default.yaml` and set `ANTHROPIC_API_KEY`. Key options:

```yaml
server:
  addr: ":8080"
database:
  driver: sqlite
  dsn: karakuri.db
providers:
  default: claude
auth:
  jwt:
    keys: []       # or set KARAKURI_AUTH_JWT_SECRET; the server refuses to
                   # start without a signing key
quota:
  backend: memory  # per replica — see Rate limits and quotas below
memory:
  semantic_top_k: 5
```

## Deployment

Karakuri ships five interchangeable ways to run locally. All five share one Docker image (`karakuri:latest`) and one canonical runtime config (`deploy/karakuri.yaml`), so switching between them never requires re-templating values.

| Variant | Best for | Up | Down |
|---|---|---|---|
| **Docker Compose** | Simplest single-machine dev | `make docker-up` | `make docker-down` |
| **Helm (direct)** | Any existing Kubernetes cluster | `make helm-up` | `make helm-down` |
| **Minikube** | Local single-node K8s with a built-in image registry | `make minikube-up` | `make minikube-down` |
| **k3s** | Lightweight K8s (edge / VMs / Raspberry Pi) | `make k3s-up` | `make k3s-down` |
| **ArgoCD** | GitOps continuous sync from this repo's `deploy/` | `make argocd-up` | `make argocd-down` |

Every variant reaches the API at `localhost:8080` (for Helm/k3s, after `kubectl port-forward svc/karakuri 8080:8080 -n karakuri`).

### Required env vars

```bash
export ANTHROPIC_API_KEY=sk-ant-...

# Required. The server refuses to start without a signing key — there is no
# default, because a predictable one would be a backdoor.
export KARAKURI_AUTH_JWT_SECRET="$(openssl rand -base64 32)"

# Required on first boot only. A database with no principals needs this to
# create the first administrator; the server refuses to start without it
# rather than generating a password and writing it to the log stream.
export KARAKURI_AUTH_BOOTSTRAP_PASSWORD="choose-something"
```

The Makefile injects these as a Kubernetes Secret (`karakuri-secrets`) for the K8s variants and as Compose environment for Docker.

### Authentication

Every `/api/v1` route except `/health` and the login endpoints requires a
short-lived access token, and every route is gated by a permission (see
[ADR 007](docs/adr/007-standalone-auth-module.md)).

```bash
# On a database with no principals, the server creates `admin` with the
# password from KARAKURI_AUTH_BOOTSTRAP_PASSWORD. Exchange it for a token pair:
curl -s -XPOST localhost:8080/api/v1/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"id":"admin","password":"..."}'
# → {"access_token": "...", "refresh_token": "...", "expires_in": 900}

# Access tokens last 15 minutes. Refresh tokens rotate on every use: the one
# you present is spent, and replaying it revokes every session in its family.
curl -s -XPOST localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' -d '{"refresh_token":"..."}'
```

Four built-in roles ship with the server: `viewer` (read-only), `auditor`
(viewer plus the audit log), `operator` (drives twins, objectives and loops),
and `admin` (everything, including the auth model). Role bindings carry a
**scope**, so a role can be granted over a single twin rather than globally:

```bash
curl -s -XPOST localhost:8080/api/v1/auth/users -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"id":"olive","roles":["operator"],"scope":"twin:abc","password":"..."}'

# Why was something refused? The decision trace answers it directly.
curl -s -XPOST localhost:8080/api/v1/auth/check -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"principal":"olive","action":"audit:read","resource":"*"}'
# → {"allowed": false, "reason": "no policy grants audit:read on audit:* (default deny)", ...}
```

Refused requests are recorded in the same audit log as authority-bounds
escalations, so `krk audit --kind authz_denied` shows who was turned away
alongside who approved what.

#### Organisations, teams and projects

A scope can name a **container** as well as a single resource, which is how a
multi-tenant deployment isolates (see
[ADR 010](docs/adr/010-scope-sets.md)). A resource carries the containers it
belongs to as a set of labels, and a binding on any of them reaches it:

```bash
krk org create --name acme
krk team create --org acme --name eng          # → {"id":"t_7f2a","kind":"team",...}
krk auth bindings add --principal alice --role operator --org acme --team eng
```

Alice now reads acme's twins and objectives, and `krk twin list` returns hers
alone. **Names are resolved to IDs before anything is sent**, and never reach a
policy: two organisations may each have a team called "eng", so a grant matching
on the word would cover both. Names are unique among siblings and nowhere else,
and renaming an organisation rewrites no binding.

Because the labels are a *set* rather than a path, a resource can be in more than
one place at once — which is how sharing across organisations works, with no
second construct:

```bash
krk project create --name delta
krk project add-twin --project delta --twin <acme-twin-id>   # needs a grant over that twin
krk auth bindings add --principal oidc:bob --role viewer --project delta
```

Bob reaches that one twin through the project and nothing else of acme's. The
tree also governs changes to itself: creating a team inside an organisation
requires a grant covering it, moving one requires a grant at **both** ends, and
you can only grant a scope you already hold.

A resource in no container carries no labels, so a deployment that never creates
an organisation behaves exactly as it did before.

#### Browser sessions

The web UI never receives a token. It posts `{"cookie": true}` to
`/auth/token` and the server replies with `Set-Cookie` instead — httpOnly, so
injected script cannot read the session; `SameSite=Strict`, which is what makes
cookies safe here without a separate CSRF token; and `Secure`, always.

That last one is the only thing you can turn off, with
`auth.cookies.insecure_allow_http: true` (or `KARAKURI_AUTH_COOKIES_INSECURE=1`).
It exists for plain-HTTP development where non-browser clients drop Secure
cookies and a login appears to succeed while doing nothing. Browsers do not need
it — `http://localhost` is a trustworthy origin and accepts Secure cookies — so
if you find yourself setting it in front of real users, terminate TLS instead.

#### Single sign-on (OIDC and SAML)

Set `auth.provider` to `oidc` or `saml` and Karakuri authenticates against
Keycloak, Okta, Auth0, Azure AD or ADFS, mapping the groups they assert onto its
own roles (see [ADR 009](docs/adr/009-federated-identity-jit-provisioning.md)):

```yaml
auth:
  provider: oidc
  frontend:
    public_url: https://karakuri.example.com   # required; never inferred
  oidc:
    issuer_url: https://sso.example.com/realms/corp
    client_id: karakuri
    client_secret_env: KARAKURI_AUTH_OIDC_CLIENT_SECRET
    groups_claim: groups        # Keycloak often nests: realm_access.roles
  role_map:
    groups:
      karakuri-admins: [admin]
      karakuri-operators: [operator]
    default: []                 # see below
```

A user who logs in becomes an ordinary principal — `oidc:<subject>` — with role
bindings reconciled from their groups on every login. After that everything
works as it does for a local account: ownership conditions, quota keys, the
audit log, `krk auth users list`.

Three things worth knowing before turning it on:

- **`public_url` is required**, and never inferred from the request. Behind a
  proxy the `Host` header is attacker-controlled, and deriving a redirect target
  from one is how open redirects start.
- **`role_map.default` is empty on purpose.** Everybody in a corporate directory
  can authenticate against a corporate identity provider, so a default role here
  grants the whole company access. A user matching no group signs in and can do
  nothing, which is the intended shape.
- **Password login keeps working.** It is the break-glass path when the identity
  provider is unreachable, which is why Phase 16 did not re-introduce a static
  shared token.

Logging in from a terminal opens a browser and catches the credential on a
loopback port:

```bash
krk auth login --sso              # or --no-browser to paste the URL elsewhere
krk auth whoami                   # oidc:<subject>, with the roles the groups mapped to
```

Nothing usable passes through the browser: the code it carries is bound to a
secret that never leaves the CLI process, and it is a spent refresh token the
moment it is redeemed.

For SAML, publish `/api/v1/auth/saml/metadata` to the identity provider and
point `auth.saml.idp_metadata_url` back at theirs. SAML is browser-only —
assertions are one-time login artifacts, not per-request credentials — so a
machine-to-machine client wants OIDC.

### Rate limits and quotas

Four limits ship enabled (see [ADR 008](docs/adr/008-standalone-quota-module.md)),
generous enough that ordinary use never reaches them:

| Tier | Default | Keyed on |
|------|---------|----------|
| Request rate | 60/min, bursting to 20 | the calling principal |
| Capability | 1000/day | twin + capability |
| LLM tokens | 1,000,000/day | twin |
| Adapter | 5000/day | twin + adapter |

```bash
krk quota config                 # what this server is actually enforcing
krk quota show --twin <id>       # usage per tier, and when it resets
krk quota reset --twin <id>      # operator override; needs quota:admin
```

A refused request gets a 429 with `Retry-After` and the `X-RateLimit-*` headers,
which are also on successful responses so a client can slow down before being
refused. At 80% of any tier a `quota_pressure` event goes to the same stream the
SPA already watches.

**Counters are per replica by default.** `quota.backend: memory` means a limit of
60/min across three replicas admits 180. Behind a load balancer, set
`backend: valkey` — it is the only backend consistent across replicas. `sql`
shares the application database and suits daily quotas more than per-request
rates.

**An exhausted LLM budget pauses rather than fails.** A budget is a business
limit, not a fault, so the loop raises a checkpoint (`reason=llm_budget_exhausted`)
and waits: approve to continue, reject to stop. Set
`quota.llm_budget_backend: litellm` to have a [LiteLLM](https://www.litellm.ai)
gateway count dollars instead of tokens — Karakuri stamps
`x-litellm-customer-id: twin:<id>` on every model call, so spend is attributed
per twin without provisioning anything.

### The limits themselves

Since Phase 19 the tiers live in the database, and the configuration file is
their **seed** rather than their answer:

```bash
krk quota config                              # what is enforced, beside what was configured
krk quota set --tier llm-tokens --cap 5000000 --reason "team grew to twelve"
krk quota tiers                               # what has been set, by whom, and why
krk quota unset llm-tokens                    # back to the file
```

A fresh database takes every tier from YAML. Once a tier is set it takes
precedence, survives restarts, and applies across replicas within the resolver's
30-second window — immediately on the server that handled the change.

**The cost of that is worth stating.** An operator reading `llm_tokens_per_day`
in a config file is reading the seed, not necessarily the limit. Two things
close the gap: the server logs one line per diverging tier at startup, and
`GET /quota` reports `configured` beside what is in force. Editing needs
`quota:admin` and is deliberately unscoped — approving a raise for a team you
administer is a tenant decision, and moving everybody's ceiling is not.

A deployment with no database answers `editable: false` and refuses the edit,
rather than accepting a limit that would vanish on the next restart.

### Asking for more

Raising a limit for one team used to mean editing YAML and restarting. It is now
a request somebody approves ([ADR 011](docs/adr/011-overrides-and-labelled-spend.md)):

```bash
krk quota request --tier llm-tokens --twin t_7f2a --cap 5000000 --reason "launch week"
krk quota requests list --status pending
krk quota requests approve qr_1a2b3c4d          # writes the override
krk quota show --twin t_7f2a                    # the new cap, in force
```

Approving writes a per-subject **override** that is consulted when the tier is
resolved, so an approval changes the limit that is actually enforced rather than
only a row in a table. It takes effect within the resolver's 30-second cache —
immediately in the process that approved it. `--until` makes the raise lapse on
its own, which is what "double for launch week" actually means; without it the
raise is permanent.

**Asking is not granting.** Every role down to `viewer` holds `quota:request`,
because anybody who can be throttled should be able to ask. Deciding is
`quota:approve`, and it is bounded by the same containment as handing out a
binding: you can only approve a raise for a subject you already hold, so an acme
administrator approves acme's requests and nobody else's. Rejecting needs no
such scope — somebody who may decide at all may always decline.

### Cost attribution

Every model call and every tool call is recorded with what it consumed, what it
cost, and the containers the resource sat in **when the call happened**:

```bash
krk cost report --since 24h --group-by provider
krk cost report --org acme --team eng --since 720h --group-by day
krk cost report --twin t_7f2a --group-by model
```

**Nothing is priced by default.** A shipped price table would be wrong the week
after it shipped, so with no `quota.rates` configured the units are still counted
and the money reads zero — the honest answer rather than an invented one. Set the
table in configuration to get currency out.

A report shows only what you may see. The filter comes from the same bindings
that decide which twins you can list, so a report is not a way around tenancy,
and naming another tenant's team narrows to nothing rather than widening to their
spend. Labels are copied onto the event rather than derived at query time: moving
a twin between teams does not move last month's money.

Raw events age out (`quota.cost_retention_days`) and the daily rollup does not, so
a shorter horizon costs the drill-down and not the totals. Each write publishes a
`cost_recorded` event on the same stream the SPA already watches.

Self-service and cost both need somewhere durable to write, and are wired whenever
a database is configured — whatever the counter backend. A deployment with neither
gets neither, and says so at startup.

### Helm chart

`deploy/` is the chart root (chart name `karakuri` from `Chart.yaml`). The same chart drives Helm direct, Minikube, k3s, and ArgoCD. Tunable values live in [`deploy/values.yaml`](deploy/values.yaml); k3s-specific overrides in [`deploy/values-k3s.yaml`](deploy/values-k3s.yaml).

```bash
helm template karakuri deploy                       # render manifests
helm upgrade --install karakuri deploy -n karakuri  # install/upgrade
helm package deploy                                 # produce karakuri-0.1.0.tgz
```

### Single source of truth

`deploy/karakuri.yaml` is the canonical server config (`/data/`-paths). The Dockerfile `COPY`s it into the image; the chart's ConfigMap template reads the same file via `.Files.Get`. No drift is possible. The local-dev config (`./karakuri.db` paths) remains at `config/default.yaml` for `go run`.

## Adapter Ecosystem

| Category | Adapter | Status |
|----------|---------|--------|
| LLM | Claude (claude-sonnet-4-6 default) | **Active** |
| LLM | Gemini (vertex AI) | **Active** (Phase 7) |
| LLM | Cursor + Copilot CLI agents | **Active** (Phase 7 — wraps external CLIs) |
| Storage | SQLite | **Active** |
| Storage | PostgreSQL + pgvector | **Active** (Phase 8) |
| Storage | MySQL | Interface only |
| Migration tooling | `krk migrate --from … --to …` | **Active** (Phase 8 — generic GORM row copy) |
| Git worktrees | go-git | **Active** |
| Version Control | GitHub (multi-instance, twin-bound) | **Active** (Phase 6) |
| Version Control | GitLab | Stub |
| Project Management | Linear | **Active** (Phase 6) |
| Project Management | Jira | Stub |
| Messaging | Slack | **Active** (Phase 6) |
| Design | Figma | **Active** (Phase 6) |
| Testing | Playwright, Go test runner | **Active** (Phase 6) |
| Calendar / Email | Google Calendar, Gmail / Outlook / SMTP / Apple Mail | **Active** (Phase 6) |
| OTel Exporter | Local file (NDJSON / CSV / **Parquet**) with size+age rotation | **Active** (Phase 12) |
| OTel Exporter | AWS (CloudWatch metrics + S3 NDJSON logs) | **Active** (Phase 12) |
| OTel Exporter | Datadog (`/api/v1/series` + `/api/v2/logs`) | **Active** (Phase 12) |
| OTel Exporter | NewRelic, Elasticsearch (ELK), Loki, OTLP Collector, Prometheus (scrape + pushgateway) | **Active** (Phase 12 extension) |
| OTel Exporter | RetryExporter wrapper (exponential backoff, `ErrPermanent` short-circuit) | **Active** (Phase 12 extension) |
| Executor | Local goroutines | **Active** |
| Executor | Restate (durable workflows) | **Active** (Phase 11) |
| Executor | Celery (Python workers via Redis) | **Active** (Phase 11) |
| Frontend | Embedded React SPA (Vite + React 18) served from the Go binary | **Active** (Phase 9 — via `web/embed.go`) |

## Observability

Karakuri's in-process metrics + logs fan out simultaneously to any subset of eight destinations (Phase 12 + extension), with chain isolation — one downstream outage logs at WARN but never blocks the others. Remote exporters are wrapped in `RetryExporter` (3 attempts, exponential backoff capped at 30s, `ErrPermanent` short-circuits on 401/403). Configure under `observability.exporters.{local,aws,datadog,newrelic,elasticsearch,loki,otlp,prometheus}.enabled` in [`deploy/values.yaml`](deploy/values.yaml); credentials flow through the `karakuri-secrets` Kubernetes Secret. The Prometheus exporter mounts `/metrics` outside bearer auth so scrapers don't need a token. See [`docs/roadmap.md`](docs/roadmap.md#phase-12--observability-fan-out-completed) Phase 12 for env var details.

## Development

```bash
make build          # build server and krk
make test           # run all tests including integration
go test ./test/integration/... -v   # integration tests only

# CI runs the same matrix on every PR (.github/workflows/ci.yml):
# Frontend → Build → Vet → Test, plus CodeQL static analysis (Go + JS/TS).
```

Import boundary rules are enforced by golangci-lint (see `.golangci.yml`):
- LangChain Go only in `internal/platform/`
- Domain packs only in `cmd/` and `internal/app/`

## Repository governance

`main` is protected — direct pushes are blocked, all changes land via squash-merged pull requests. Active rules:

- 1 approving review required; code-owner review required; stale reviews dismissed on new push; last-push approval required
- All four required status checks (Frontend, Build, Vet, Test) must pass and the branch must be up to date
- Linear history; signed commits required; force-pushes and deletions blocked

Additional security stack:

- **Secret scanning** + **push protection** enabled
- **CodeQL** static analysis for Go and JS/TS, on every PR + weekly schedule
- **Dependabot** alerts + security updates + version updates across `gomod`, `npm`, and `github-actions`; major-version bumps excluded — those land via maintainer-opened PRs after compat testing
- **Private vulnerability reporting** open at [`/security/advisories/new`](https://github.com/bsenel/karakuri/security/advisories/new)

See [SECURITY.md](./SECURITY.md) for vulnerability reporting, [CONTRIBUTING.md](./CONTRIBUTING.md) for the Dependabot review policy and merge workflow, and [`.github/CODEOWNERS`](./.github/CODEOWNERS) for ownership.

## Philosophy

Karakuri is built for **human augmentation**.

The project is free for organizations using AI to
empower employees and improve productivity.

Use intended primarily for workforce replacement
is restricted under the
[Karakuri Human Augmentation License Addendum (HALA)](./HALA.md).

## Roadmap

Phases 1–21 have shipped: the core engine, then cross-domain objectives, then the multi-team production layer and the interface for it, and then the supervisor that makes an objective something Karakuri holds rather than something it finishes.

- **Phase 14:** RBAC + fine-grained authorization, shipped as a standalone module `github.com/bsenel/karakuri/auth` reusable by any net/http or chi server
- **Phase 15:** API rate limiting + quota management, shipped as `github.com/bsenel/karakuri/quota` with Redis/SQL backend submodules
- **Phase 16:** Federated identity (OIDC + SAML) as `auth` submodules
- **Phase 17:** Multi-tenant hierarchy — orgs, teams and projects as scope sets ([ADR 010](docs/adr/010-scope-sets.md))
- **Phase 19:** Web interface for auth, quota, cost and audit; limits become resolved state ([ADR 012](docs/adr/012-limits-as-resolved-state.md))
- **Phase 18:** Quota self-service + cost attribution — per-subject overrides an approval writes, and labelled spend ([ADR 011](docs/adr/011-overrides-and-labelled-spend.md))
- **Phase 20:** Standing objectives — a desired state held by a supervisor that senses cheaply and spends rarely, with autonomy earned under a declared ceiling ([ADR 015](docs/adr/015-standing-objectives-and-reconciliation.md))
- **Phase 21:** Digests — what a twin's standing objectives did and what they need decided, assembled from records that already exist ([ADR 016](docs/adr/016-earned-autonomy-and-digests.md))

Full per-phase status, acceptance criteria, and architecture rationale in [docs/roadmap.md](docs/roadmap.md).

## License

Licensed under Apache 2.0.

See:

- [LICENSE](./LICENSE)
- [HALA.md](./HALA.md)
- [SECURITY.md](./SECURITY.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
