# ADR 022 — A tool discovered from an MCP server is bounded four ways, and Karakuri serves MCP read-only

**Status:** Proposed
**Date:** 2026-09-26
**Relates to:** [ADR 006](006-multi-instance-tool-adapters.md) (named instances, a default, twin bindings), [ADR 010](010-scope-sets.md) (scope sets), [ADR 015](015-standing-objectives-and-reconciliation.md) (authority is written into the bounds, never a second gate), [ADR 019](019-capabilities-declare-what-they-need.md) (capabilities declare what they need), [ADR 021](021-observations-carry-provenance.md) (a payload says who wrote it)

## Context

Phase 6 shipped ten adapters across seven slots, and every tool Karakuri could
use was one somebody in this repository wrote. MCP is the protocol the field
settled on for tool access, and ADR 006 is already the shape of an MCP client
configuration: a slot with a `default` and named `instances`, resolved per twin
through `AdapterBindings`. The slot was the easy part.

What does not fit is the capability model. A pack declares its capabilities when
it is written, and conformance checks them. An MCP server's tools are read off
the server at boot, from a server this repository did not write, and change when
that server changes. Registering them as ordinary capabilities would give a tool
that appeared this morning everything a pack's capability gets: it could grade a
criterion, get a git worktree, run without approval, and take part in pack
conformance.

## Decision 1 — discovered tools live in a reserved namespace

A discovered tool is registered as `mcp.<instance>.<tool>` in domain `mcp`
(`capability.MCPCapabilityID`, `capability.MCPDomain`).

The instance is part of the ID because two servers can both export `read_file`,
and the capability ID is what routing, quota, telemetry and the audit log all
key on. An ID that named only the tool would make one tenant's filesystem
server indistinguishable from another's everywhere downstream. The tool name is
cut once from the left (`capability.MCPToolOf`), so a tool whose name contains
dots reaches the server as the server named it.

The namespace is what tells a discovered tool from a declared one. Each of the
four bounds below asks `capability.IsMCPCapability` at the place that would
otherwise trust the ID, rather than relying on a flag on the registry entry. The
entry is a plain struct, and anything that builds or rewrites one could set a
field.

## Decision 2 — the four bounds

1. **Never `NeedsWorkspace`.** `Capability.GrantsWorkspace` returns false in the
   reserved namespace whatever the entry declares, and the loop's
   `needsWorkspace` asks it rather than reading the field. A server's
   description saying a tool edits files is not a reason to cut a branch.

2. **Never valid as a `Criterion.Verifier`.** A criterion is how this deployment
   decides its own work is done. A verifier a third party controls would let
   that party declare an objective satisfied. `Criterion.VerifierIsReserved`
   is checked in two places:
   - in conformance (`checkCriterionVerifierRefs`), before the cross-pack
     escape, so `Domain: "mcp"` does not read as a reference to another pack;
   - in `stepVerify`, because boot runs only the dangling-verifier audit, and
     that only warns. A reserved verifier is **not met**. It is not handed to
     the agent either, because the agent's judgement would be read off that
     same tool's output.

3. **Approval by default.** `AuthorityBounds.Decide` escalates any plan that
   names a tool in the reserved namespace, even when `RequiresApprovalFor` is
   empty. `RequiresApprovalFor` is written when an agent is written, and these
   IDs are read off a server at boot: an operator who wanted them gated had no
   name to list. This is still the single gate ADR 015 requires. The default is
   part of the policy, not a second check beside it. Every MCP action escalates
   on a fresh deployment.

4. **Outside pack conformance.** A discovered tool is never in
   `Pack.Capabilities()` or `Pack.EnvironmentFactories()`, so the per-pack suite
   never sees one. `krk domain test software` behaves the same with or without
   MCP instances configured. The other direction is checked:
   `checkReservedNamespace` fails a pack that declares a capability in the
   namespace, an environment in domain `mcp`, or a `Serves` entry naming an
   `mcp.` ID. Any of those would hand a pack the exemptions or the routes that
   belong to discovery.

## Decision 3 — untrusted by construction

Every `ActionResult` the MCP environment returns carries
`environment.TrustThirdParty`, including refusals the environment writes itself.
ADR 021 decides trust per payload because other environments have a trusted half
to separate out, such as a commit SHA versus a PR title. This environment has
none: everything a server returns was written by the server.

An `Observation` that lists advertised tools is also third party, because a tool
**description** reaches the planner's catalog. That is ADR 021's surface arriving
from a new direction. An instance advertising nothing (unreachable, or
allowlisted to nothing) carries nobody's prose and stays trusted. That is the
same rule `researchEnv` applies to an empty search.

The per-instance allowlist is part of the same decision. **An empty
`allowed_tools` allows nothing**, because a server can add a tool between one
boot and the next, and "empty means everything" is a default an operator
discovers in the audit log. What the allowlist refused is reported in `/health`
as `filtered`, not dropped silently. The allowlist is checked again at call
time, because a capability ID can outlive the list that admitted it, for
example in a stored plan or a pending checkpoint.

## Decision 4 — discovery completes before registration

`environment.Factory.Serves` is an exact list, reverse-indexed once at
`Register`, with no prefix matching and no re-registration. Either the registry
learns a namespace route, or discovery finishes before registration. This ADR
chooses the second.

`tools.NewRegistryFromConfig` dials every instance, runs the handshake and one
`tools/list`, and only then does bootstrap register the capabilities and one
factory per instance (`mcp.env.<instance>`) whose `Serves` is exactly what was
discovered and allowed. An MCP action then routes through the index like any
other capability. The `EnvID` fallback is left for what the index cannot
answer, and ambiguity is still never resolved by picking.

The consequences are accepted:

- **A server that is down at boot serves nothing until the next boot.** It
  appears in `/health` as `unreachable` with its error. The alternative would
  register a promise the router sends actions to. There is no reconnect, for
  the same reason: a client that quietly re-established a session would make
  `/health` describe a server it is no longer talking to.
- **A server's new tools need a restart.** `listChanged` is not followed.
- **One `tools/list` page.** `nextCursor` is accepted and ignored instead of
  half-handled.
- **Boot does I/O.** The MCP slot builder is the only one that talks to anything.
  A down server does not fail the boot: `mcp.NewInstance` never returns an
  error.

Tool sources belong to no domain an objective declares, so two readers are
reached differently:

- `BuildEnvironments` builds every factory in domain `mcp` whatever the
  objective's domains are. The factory refuses to build for a twin bound to a
  different instance, or for an unbound twin when it is not the slot default.
- The reason step's catalog asks the environments actually built for this twin
  (`environment.ToolSource`). That limits a twin's planner to its own instance's
  tools.

Calls to one server are serialised on its `Client`. The stdio transport is one
pipe each way and skips replies whose ID it did not send, so two calls in flight
would each discard the other's reply.

## Decision 5 — Karakuri as an MCP server is read-only, with no permission model of its own

`POST /api/v1/mcp` serves streamable HTTP inside the authenticated group. The
bearer token is the same token `krk` sends, and there is no MCP credential. It
serves one JSON body per POST, with no SSE, because every tool is a bounded
read. It sends no session header, because the token already identifies the
caller on every request.

There is no MCP permission model either. Each tool demands the action that the
REST route answering the same question demands, against a resource reference
built the same way (`objective:read` for objectives and reconcile status,
`report:read` for a digest, `audit:read` for telemetry). Listings use the same
`ScopedCollectionRef` / `ListFor` rules as the REST list routes. The request-free
forms of the scope helpers were extracted from `internal/auth` so there is one
rule, not two. `tools/list` shows only what the principal may call. A refused
call is written to the same audit hook as a refused REST request, named with the
tool.

Nothing exposed starts work, changes an objective, or resolves a checkpoint.
`checkpoint_resolve`, `checkpoint_approve` and `checkpoint_reject` are refused
by name, with the reason. A runtime will eventually try to clear the checkpoint
that blocks it, and "unknown tool" would read as an oversight to work around
rather than a decision. Resolving a checkpoint is a decision about who may
approve, and it stays with a person.

## Consequences

- **Every MCP action on a fresh deployment waits for a person.** That is the
  intended default, and it costs reviewer attention. No per-tool exemption dial
  was added.
- **A criterion naming an MCP tool can never be met.** A template carrying one
  fails `krk domain test`. One that reaches a deployment anyway scores zero on
  that criterion rather than being settled by the tool.
- **The planner sees a server's descriptions unedited.** Rewriting them would
  put words in a third party's mouth. They arrive marked as third-party writing
  instead, and any plan built on them escalates.
- **`schemaOf` is lossy by design.** It narrows the server's JSON Schema to what
  `capability.Schema` models, for the planner only. Nothing validates a call
  against it; the server does that.

## Alternatives considered

**Register discovered tools as ordinary capabilities under their own domain.**
They would receive every property a pack's capability has, which is the problem
this ADR exists to solve.

**A namespace route in the environment registry** (prefix matching in
`ServedBy`). It would allow late discovery and reconnects. It was rejected
because it widens the index for one caller and makes `/health`, `Serves` and the
router able to disagree about what an instance offers.

**An `approval_exempt` list per instance.** It was deferred rather than built.
The first thing it would be used for is switching off the case the default
exists for.

**Exposing checkpoint resolution to MCP clients behind `checkpoint:resolve`.**
It was rejected for this phase. A foreign runtime holding a token with that
action is a different question from a person holding it, and the phase is read
and propose only.
