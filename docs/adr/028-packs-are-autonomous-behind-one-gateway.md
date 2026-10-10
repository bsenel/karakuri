# ADR 028 — A domain pack is autonomous, and every tool call goes through one gateway

**Status:** Proposed
**Date:** 2026-10-10
**Relates to:** [ADR 003](003-git-worktrees.md) (worktrees created by the core; superseded as a core responsibility), [ADR 005](005-domain-pack-isolation.md) (domain pack isolation; not read, see "What was read"), [ADR 006](006-multi-instance-tool-adapters.md) (named instances bound per twin), [ADR 015](015-standing-objectives-and-reconciliation.md) (one gate: authority is written into the bounds), [ADR 019](019-capabilities-declare-what-they-need.md) (`NeedsWorkspace`; Decision 1 amended), [ADR 021](021-observations-carry-provenance.md) (a payload says who wrote it), [ADR 022](022-discovered-tools-are-bounded-four-ways.md) (discovered tools are bounded four ways), [ADR 026](026-an-environment-that-cannot-see-says-so.md) (blind is an error, not an empty result)

## Context

A domain pack today is a Go value compiled into the server. `domain.Pack` in
`internal/core/domain/domain.go` returns capabilities, environment factories,
agent definitions, objective templates and planner hints, and has `Init` and
`Teardown`. `domains/software/pack.go` implements it and is handed the
deployment's `tools.Registry`, so the pack's environments reach GitHub, Linear,
Slack and the observability backends through adapters the orchestrator built
and whose tokens the orchestrator holds.

The orchestrator also does part of the pack's work for it. `Capability` in
`internal/core/capability/capability.go` carries `NeedsWorkspace`, and the loop
provisions a git worktree for a capability that sets it (ADR 003, ADR 019,
AGENTS.md rule 3). A git worktree is the software pack's idea of a workspace.
A pack whose work happens in a sandbox, or which needs no workspace, has nothing
to say in that field, and the core has a `git.WorktreeManager` it cannot use for
it.

That is one pack. Every further pack written this way adds its adapters, its
tokens and its notion of a workspace to the orchestrator, in Go, in the same
process.

Two other things already point the same way. ADR 022 made Karakuri an MCP
client and server, with discovered tools in a reserved namespace. Phase 34
(planned, on the unmerged branch `karakuri/phase-34-own-tools-in-agents-hands`)
gives a coding agent Karakuri delegated to a credential scoped to that one
delegation, so that the agent calls Karakuri's tools as itself and not as the
operator.

## Decision

This is the owner's decision of 2026-10-10, given "for scalability of domain
packs": Karakuri becomes an orchestrator on top of autonomous domain packs. The
five points are recorded here as decided. This ADR works out what they require;
it does not reopen them.

1. **One MCP gateway.** Karakuri has a single MCP gateway. Every tool call goes
   through it, whether the caller is the reasoning loop or a coding agent
   Karakuri delegated to (Phase 34). The gateway is where authentication,
   authority bounds, quota, audit, cost and provenance marking happen.
2. **Each pack brings its own MCP server or servers**, holding its capabilities
   as tools: fetch logs, fetch PRs, run tests, write code, run a remediation.
   The gateway routes to them.
3. **Packs are autonomous, and the orchestrator does not know their
   internals.** (Corrected by the owner the same day: "Karakuri should not care
   about the internal details of a domain pack. The orchestrator should be
   decoupled from domain packs.") Where a pack does its work (a git worktree
   for the software pack; a sandbox, or nowhere at all, for another), which
   adapters and vendors it uses and which secrets it holds are the pack's own.
   The orchestrator has no concept of a workspace, not even as an opaque
   reference: it creates none, grants none, stores none and is told of none. It
   knows nothing of a pack's worktrees, branches, sandboxes, adapters, vendors
   or secrets. What it knows about a pack is listed exhaustively under "The
   whole coupling surface" below.
4. **What stays in the orchestrator**, never in a pack or an MCP server:
   objectives, the reasoning loop, reconciliation of standing objectives,
   checkpoints and approvals, authority (who is asking and what they may do),
   quota, the audit log, cost, memory, and the verdict that an objective
   converged. A pack runs a verifier; the orchestrator decides which verifiers
   it trusts and whether the criteria are met.
5. **What MCP does not carry goes in a manifest.** Each pack ships a small
   manifest outside MCP: which of its tools may be used as criterion verifiers,
   the name of its snapshot tool for drift detection, its objective, agent and
   stream templates, and its routing hints.

### The whole coupling surface

These are the only things the orchestrator knows about a pack. The list is
exhaustive.

- (a) **Its registered MCP endpoint.**
- (b) **Its tools' names, descriptions and input schemas.**
- (c) **Its manifest:** which tools may serve as criterion verifiers; its
  objective, agent and stream templates; its routing hints; and the name of its
  snapshot tool.
- (d) **A snapshot value for drift detection**, which the orchestrator only
  compares for equality with the previous one and never interprets.
- (e) **Results, their provenance mark, and the explicit "cannot see" error.**

**Rule: anything not on this list is a pack internal, and a change to the
orchestrator that needs to know one is a design error.** Workspaces, worktrees,
branches, sandboxes, adapters, vendors and secrets are not on the list.

In the other direction, a pack knows the orchestrator only by what the call
contract sends it (section b): the orchestrator's identifiers, the authority
bounds, a credential, and the lifecycle signals.

The sections below are what those five points need in order to be buildable.

### a. Trust: registered is first-party, discovered is not

There are two classes of MCP server behind the gateway, and the class is decided
by where the operator configured the server, never by anything the server says.

- **A pack's server is first-party because an operator registered it as a pack**
  in the deployment's configuration, together with the pack's manifest. Its
  tools are registered under the pack's own ID (`software.…`), may set what a
  pack's capability may set today, and are subject to pack conformance.
- **A tool discovered on anyone else's server** is configured as an MCP
  instance, as ADR 022 has it, and is registered as `mcp.<instance>.<tool>`.
  ADR 022's bounds apply to it: never a criterion verifier, approval by
  default, outside pack conformance. Its first bound, "never gets a workspace",
  is reworded by this ADR (see Consequences): the orchestrator grants
  workspaces to nobody, so there is nothing to withhold. Everything it returns
  is `TrustThirdParty`.

The two are told apart the way ADR 022 already tells them apart: by the
capability ID's namespace, asked through `capability.IsMCPCapability` at each
place that would otherwise trust the ID. The gateway assigns the namespace from
the configuration entry that named the server. A server's name, its
`serverInfo`, its tool names, its tool descriptions and its manifest are all
inputs from the server, and none of them is read when the class is decided. A
discovered server that describes itself as a pack is still a discovered server;
a pack whose manifest or tool list names something under `mcp.` fails
registration, which is the direction `checkReservedNamespace` already checks
according to ADR 022.

The manifest does not grant anything either. It nominates verifiers; point 4
says the orchestrator decides which it trusts.

### b. The call contract between gateway and pack

**The gateway sends, with every call:**

- the orchestrator's own identifiers for the call: tenant, twin, objective,
  loop and pass, and action;
- the `agent.AuthorityBounds` in force for the run;
- a signed, limited credential: scoped to that twin, valid no longer than the
  action's timeout, revoked when the action ends.

The permission decision is made in the orchestrator before the call is sent.
ADR 015's single gate stays single. The bounds travel so that a pack can keep
what it does on the orchestrator's behalf inside them, for example when it
starts a coding agent of its own; a pack that refuses a call has failed an
action, not withheld an approval.

**Correlation is by the orchestrator's identifiers.** A pack that needs
continuity between the actions of one pass (write code, then run the tests on
it) keys whatever it keeps on the identifiers it was sent. Nothing pack-defined
is stored by the orchestrator or echoed back to the pack on a later call.

The likely carrier of the credential is Phase 34's delegation credential.
`internal/feature/delegation/issuer.go` on that branch mints a token signed with
the API's own keyring for a throwaway service principal holding a read-only
viewer binding scoped to one twin, expiring at the action's timeout, audited
without the token, and revoked afterwards. It does not today carry a tenant, an
objective or authority bounds, and it is issued to authenticate *to* Karakuri's
API, not to a pack. Whether those travel as claims or beside the token, and how
a pack verifies the signature, is for the phase to settle; the keyring was not
read for this ADR.

**The pack returns:**

- the result;
- the trust of the text in it, set from what the payload holds (ADR 021,
  AGENTS.md rule 9): `TrustThirdParty` when it carries a PR title, a log line,
  a chat message or anything else somebody outside the deployment wrote;
- **an explicit "cannot see" error, never an empty result**, when the pack could
  not look: no backend bound, backend down, a signal the backend does not have
  (ADR 026, AGENTS.md rule 10). An empty result means the pack looked and found
  nothing.

The gateway marks provenance from what the pack returned and can only lower
trust, never raise it: a first-party pack saying "third party" is believed, and
a result with no trust stated is treated as third party.

The pack returns nothing about where or how it did the work. A pack may put
what it likes in its result; to the orchestrator that is data, never parsed for
meaning.

**Lifecycle signals.** The orchestrator tells every pack that was called during
a pass, in domain-neutral terms and with the same identifiers:

- **a pass ended** (tenant, twin, objective, pass);
- **an objective ended or was cancelled** (tenant, twin, objective).

What a pack does on a signal is its own business: clean up where it worked,
drop a cache, nothing. The orchestrator does not know, and decides nothing on
the outcome. Two rules make a missed signal harmless:

- **A signal is safe to repeat.** The orchestrator may send one more than once,
  for example after a restart, and a pack treats a signal for something it no
  longer holds as done.
- **A pack bounds its own leftovers.** Delivery is best effort: a pack that was
  down, or an orchestrator that crashed, means a signal is missed. A pack
  therefore expires on its own what it keeps for a pass or an objective (by
  age, by count, or however suits it). The signal makes cleanup prompt; the
  pack's own bound makes it certain.

How the signals are carried (a reserved tool every pack serves, or an MCP
notification) is for the phase to settle.

**The audit log** records the call, the caller, the authority it ran under and
the result as the pack returned it. It has no workspace field and no other
pack-defined field.

### c. The pack contract, and how it is checked

A pack promises:

- **Per-tenant secret resolution.** The secret used for a call is the one that
  belongs to the tenant named in the call.
- **No secret in a result or a log.** Not in tool output and not in an error.
- **Continuity and isolation by identifier.** Actions of the same pass see each
  other's work where the pack's tools imply it; actions of different passes,
  objectives, twins or tenants never do. How the pack achieves that is not part
  of the contract.
- **Lifecycle signals are safe to repeat, and leftovers are bounded** whether
  or not a signal arrives.
- **Snapshots for drift.** The snapshot tool the manifest names returns a value
  that is equal when nothing changed and different when something did, and the
  "cannot see" error, not a value, when the pack cannot look (ADR 026). The
  orchestrator compares values for equality and reads nothing in them.
- **Timeouts.** A call returns or fails within the timeout the gateway gave it.

The orchestrator can no longer inspect a pack, so the contract is checked from
outside: **a conformance suite that runs against the pack's server through the
call contract**, as a client would. This replaces reading the pack's Go values.
The existing in-process conformance package was not read for this ADR, so which
of its checks carry over as they are is not claimed here.

Some of these cannot be proved from outside. A suite can plant a secret and
search the results for it; it cannot show that a pack never logs one somewhere
the suite does not look. The suite says which promises it tested and which it
only asked the pack to declare.

### d. Where MCP is not used

- **The verdict.** Whether an objective converged is decided in the
  orchestrator. A party that could say "met" over a socket could end an
  objective.
- **Permission.** Checkpoints, approvals and the authority decision. An approval
  that is a tool is an approval a tool caller can give itself; the served
  surface already withholds it (`mcpWithheld` in `internal/api/handler/mcp.go`).
- **Drift detection.** Reconcile compares snapshot values for equality on its
  own schedule. The manifest names the snapshot tool; deciding that the world
  moved is reconcile's (ADR 015, AGENTS.md rule 8).
- **The loop's own state and memory.** They are the orchestrator's data, read
  and written in-process. Routing them through a tool boundary would let tool
  output address them.
- **Calls between the orchestrator's own layers.** `api → feature → core` stays
  Go calls. The gateway is the boundary to tools, not a bus.

### e. The first step stays in one process

The software pack's MCP server first lives inside the Karakuri binary, behind
the gateway. The call contract, the manifest and the outside-in conformance
suite are built and exercised there. Nothing is deployed separately.

It is split out when one of these is true:

- a second pack exists that is not compiled into the binary;
- a pack is written in a language other than Go.

In one process, "the orchestrator does not hold a pack's secrets" is true of the
code and not of the address space. That is accepted for the first step and is
one more thing the split removes.

## Consequences

**What is superseded, amended or untouched.** This ADR states the effect; it
edits none of these. The effect lands with the migration, not with this file.

| | Effect |
|---|---|
| [ADR 003](003-git-worktrees.md) | **Superseded outright.** The orchestrator has no `WorktreeManager` and no notion of a worktree. Whether the software pack keeps a worktree per pass is that pack's internal, no longer an architectural decision of Karakuri. |
| AGENTS.md rule 3 | **Superseded outright**; the phase removes it. |
| [ADR 019](019-capabilities-declare-what-they-need.md) | **Superseded outright**, with `NeedsWorkspace` and `GrantsWorkspace`: the fields leave the core type and nothing replaces them. Decisions 2 to 4 were read as headings only; whatever they declare that is not on the coupling surface goes the same way, and the phase must check each. |
| The in-process pack interface (`domain.Pack`) | **Superseded as the pack boundary** by a pack's MCP server plus its manifest. In the first step it remains as how the in-binary software pack is implemented. |
| [ADR 022](022-discovered-tools-are-bounded-four-ways.md) | **First bound reworded, the other three untouched.** "A discovered tool never gets a workspace" assumed an orchestrator that grants them. It grants none to anyone, so the bound becomes: a discovered tool gets the call contract and no more, and is sent no lifecycle signals. The ADR's title also says Karakuri serves MCP read-only; that part was not read, and a gateway that carries a pack's acting tools to a delegated agent bears on it. The phase must say how. |
| AGENTS.md rules 8, 9 and 10; ADR 015, 021, 026 | **Untouched.** Rules 9 and 10 now also bind a pack across the call contract. |
| ADR 006 | **Untouched** for discovered servers. Whether a pack's own backends keep using its slots is the pack's business. |
| ADR 005 | Not read. It concerns pack isolation and is likely affected; this ADR does not say how. |

**What it costs.**

- **A process boundary and serialisation on every call**, once a pack is split
  out. In the first step the cost is the serialisation and the gateway hop
  without the process boundary.
- **Each pack repeats what packs have in common inside them**, such as secret
  handling and keeping state per pass. The answer is an optional shared library
  a pack may use. It is not a core responsibility and the orchestrator does not
  depend on it.
- **Cleanup is no longer guaranteed by the core.** A missed lifecycle signal
  leaves a pack's leftovers until the pack's own bound removes them; the
  orchestrator cannot see them.
- **The audit log says less about where work happened.** It holds only what the
  pack chose to put in its result.
- **A pack that holds secrets is a larger thing to trust.** Today a pack is
  code handed adapters. After this it holds the tokens, and the orchestrator
  can check its behaviour only from outside.
- **The one existing pack has to be migrated.** `domains/software` has
  environments built on the orchestrator's `tools.Registry`, and worktree
  provisioning that `internal/feature/loop/service.go`, `internal/app/bootstrap.go`,
  `internal/api/server.go` and `internal/api/handler/health.go` all reference
  (found by search, not read). That is the bulk of the phase.

**What it buys.** A second pack adds a registration and a server. It adds no
adapters, tokens or domain nouns to the core, and need not be written in Go.
The test of the decoupling, an acceptance criterion of Phase 39: the
orchestrator's packages (`cmd`, `internal/api`, `internal/feature`,
`internal/core`) import no pack package and contain no software-domain noun
(worktree, branch, repository, pull request) outside tests; and a second,
trivial pack with no workspace and no secrets at all can be registered and
driven end to end without any change to the orchestrator.

## What was read

Read: `AGENTS.md`; `internal/core/capability/capability.go` and `mcp.go`;
`internal/core/domain/domain.go`; `domains/software/pack.go`; ADR 003 in full;
ADR 022 through Decision 4; ADR 026 through Decision 2;
`internal/feature/delegation/issuer.go` on
`origin/karakuri/phase-34-own-tools-in-agents-hands` through `Issue`; in
`docs/roadmap.md` the opening of Phase 33 and Phase 34 and "Domain Pack System"
(registration and isolation).

Skimmed: `domains/software/environments.go` and `capabilities.go` (function
list and where trust, snapshots and `NeedsWorkspace` are set);
`internal/api/handler/mcp.go` (comments only); ADR 019 (context and headings);
ADR 006, 015 and 021 (headings only, plus what AGENTS.md rules 8 and 9 say of
them).

Not read: ADR 005, 017, 018 and 020; ADR 027, which exists only on pull request
#159 and is not on main; `internal/core/domain/registry.go`; the conformance
package; `git.WorktreeManager` and the loop's use of it; the auth keyring; the
steps of Phases 33 and 34; the rest of ADR 022 and ADR 026.
