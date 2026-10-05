# ADR 027 — A tool that asks for input is an error the planner sees

**Status:** Proposed
**Date:** 2026-10-05
**Relates to:** [ADR 021](021-observations-carry-provenance.md) (trust is set from what the payload holds), [ADR 022](022-discovered-tools-are-bounded-four-ways.md) (everything an MCP server returns is somebody else's writing), [ADR 026](026-an-environment-that-cannot-see-says-so.md) (an error, never an empty result)

## Context

Phase 33 of the roadmap gives Karakuri's MCP client a second protocol path for
revision 2026-07-28. The roadmap describes that revision from the
specification's "Key Changes" page
(https://modelcontextprotocol.io/specification/2026-07-28/changelog, read
2026-10-04 by the discovery pass, not re-read for this ADR): every result
carries a required `resultType`, and a server that wants something from the
client mid-call no longer sends a request of its own. The discovery report
(`docs/research/discovery-2026-10-04.md`, "Trends") records it as a server
returning `resultType: "input_required"` and the client retrying with
`inputResponses`, in place of `roots/list`, `sampling/createMessage` and
`elicitation/create`.

What is in the repository, read for this ADR:

- `internal/platform/tools/mcp/client.go` reads `resultType` in one place,
  `checkResultType`, on the 2026-07-28 path only. The handshake path reads
  none.
- The client declares no capabilities on either path and never implemented
  sampling or elicitation. It has nothing to answer a request for input with.
- Roadmap Phase 33, step 3, leaves the meaning open between two readings, "an
  error returned to the planner or a checkpoint", and notes that the second
  reaches `internal/feature/`. It says that until decided the result is "an
  error that names the reason, never an empty result".
- The roadmap states that no Karakuri user has asked for Phase 33 at all; the
  demand it cites is other products' issue titles.

## Decision 1 — `input_required` is an error, with a type of its own

A `tools/call` result whose `resultType` is `input_required` becomes
`*mcp.InputRequiredError`, returned from `Client.CallTool` beside a zero
`ToolResult`. It carries the tool's name and the server's request. Its text
names the result type, says that this client does not answer requests for
input, and quotes what the server asked.

`Instance.Call` returns it unwrapped enough that `errors.As` still matches.
`Environment.Act` matches it and reports an `ActionResult` with
`Success: false` and the error's text. It is never a successful result and
never one with no error text.

The type exists so that the one caller that has to tell this failure from a
transport failure, `Environment.Act`, does it by type and not by reading a
sentence.

## Decision 2 — the client does not retry it and does not answer it

One request is sent. The result arrived whole, so sending the call again asks
the server the same question, and the client has no `inputResponses` to send
with it. Step 4 of the phase (re-issuing a call whose response stream broke)
is a different case and does not apply to a result that was received.

## Decision 3 — the server's request is third-party text, and it is capped

The request is written by the server. `Environment.Act` marks the result
`environment.TrustThirdParty`, as it marks every result from this environment
([ADR 022](022-discovered-tools-are-bounded-four-ways.md), decision 3), so a
plan built on it escalates under the existing provenance rule
([ADR 021](021-observations-carry-provenance.md)). Nothing in the loop infers
this.

The request is cut to `maxInputRequestText` bytes in `client.go` before it is
put in the error. An error's text reaches a planner prompt, and the only other
bound on it is the 8 MiB the HTTP transport will read. The package had no cap
for foreign text in an error before this; the value is a choice made here and
is not derived from a measurement.

## Consequences

- **A tool that needs input mid-call cannot be used through Karakuri.** Every
  call to it fails the same way until this ADR is revisited. This is inferred
  from the code; it was not observed against a real server.
- **The planner sees why.** The failure names the tool, the result type and
  what the server asked for, so a planner can pass the value as an argument if
  the tool takes one, choose another tool, or stop. Whether a planner does any
  of these was not tested: the tests assert what `Act` returns, not what a
  model makes of it.
- **Nothing under `internal/feature/` or `internal/core/` changes.** The loop
  receives a failed action, which it already handles.
- **A server's question can reach a prompt.** It is capped and marked
  third-party, which are the same two protections a tool result has. A server
  that asks for a secret gets no answer from this client, but the planner does
  read the question.

## What is not known

- **No real server that returns `input_required` was run.** The tests use an
  `httptest` fake written in this repository. Phase 33 step 1 ran real SDK
  servers and records none of them returning this result type.
- **The field that carries the server's request was not checked against the
  specification text.** The discovery report names the client's half
  (`inputResponses`) and not the server's. `inputRequests`, and the
  elicitation-like shape below it that the client reads a `message` from, are
  assumed; they are declared once in `protocol.go` with that caveat. When the
  shape is not the assumed one the client falls back to carrying the field's
  raw JSON, capped; when the field is absent the error says the server gave no
  request. In all three cases the result is still the named error.
- **Whether `input_required` can be returned for methods other than
  `tools/call` is not known.** If it is, `checkResultType` returns the same
  error type with no tool name, wrapped with the method.

## Alternatives considered

**A checkpoint a human answers.** The call would pause, a person would supply
the input, and the client would retry with it. It was rejected for this pass
on three grounds. It reaches `internal/feature/`, where checkpoints live, and
the client is in `internal/platform/`. It needs a way to carry the answer back
into a retried call, which does not exist, and the retry's wire shape is one
of the things not checked above. And no user has asked for it (YAGNI). What
would reopen it: a Karakuri deployment binding a server whose tools return
`input_required` in ordinary use, shown by this error appearing in its tool
events, or a user asking for such a tool to work.

**An empty or partial result.** Decoding the result as a `ToolResult` gives no
content and `isError: false`, which reads as a tool that ran and returned
nothing. [ADR 026](026-an-environment-that-cannot-see-says-so.md) rejects an
empty result for "could not" on the ground that it is indistinguishable from
"looked and found nothing"; the same reasoning applies here.

**Retrying without an answer.** It asks the same question again and may run
the tool's side effects again on a server that performed some before asking.
Whether any server does that is not known.

**A plain `fmt.Errorf` and no type.** This is what the branch had before this
ADR. It was enough for the planner, but `Environment.Act` could not tell the
failure apart without matching on a sentence.
