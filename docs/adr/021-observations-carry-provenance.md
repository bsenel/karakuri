# ADR 021 — A payload says who wrote it, and a plan drafted from a stranger's writing escalates

**Status:** Accepted
**Date:** 2026-08-22
**Relates to:** [ADR 015](015-standing-objectives-and-reconciliation.md) (authority is expressed by writing bounds into the request), [ADR 016](016-earned-autonomy-and-digests.md) (earned autonomy), [ADR 019](019-capabilities-declare-what-they-need.md) (capabilities declare what they need), [ADR 020](020-a-declaration-is-verified-by-running-it.md) (a declaration is verified by running it)

## Context

Everything from Phase 1 to Phase 26 was about what Karakuri may **do**. Nothing
was about what it may be **told**.

`stepObserve` fanned out across every environment and merged the results into
one flat `WorldState` with a composite SHA. Nothing on `environment.Observation`
said where a payload came from, so a pull-request title somebody typed and a
commit SHA an operator's GitHub returned were the same kind of fact to the
planner that read them next. The same was true of `environment.ActionResult`.

Karakuri opens pull requests and sends mail on somebody's behalf. The exposure
is real, and OWASP's 2026 list for agentic applications puts goal hijack first
— with the distinction that applies here: the risk is not what a model *says*,
it is what an actor with credentials *does next*. The mitigation the field
converges on is not better prompting; it is approval for consequential actions,
which is a mechanism this system has had since Phase 1 and had never pointed at
this.

**What actually reaches the planner is narrower than it looks, and the
narrowing is itself the finding.** Read from the code rather than from the
threat model:

| Path | Carries somebody else's writing? |
|---|---|
| `gitEnv.Observe` | Yes — `PRSummary.Title`. Bodies are not carried, and the version-control adapter has no issue operation at all. |
| `ticketEnv.Observe` | Yes, when a `ticket_id` filter is supplied — title and body. |
| `commsEnv.Observe` | *Could*, and never does. It reads `q.Filter["channel"]`; `stepObserve` calls `Observe` with `ObservationQuery{Limit: 20}` and no filter, so `GetMessages` is unreachable. |
| `researchEnv.Observe` | No, deliberately — research has no ambient state. It reports whether its adapter is wired. |
| `researchEnv.Act` | **Yes — scraped titles and summaries.** The widest surface in the tree. |
| the email slot | Never reaches the planner. No pack registers an email environment; `Registry.Email` is consumed only by digest delivery. |

So the untrusted content with the widest surface arrives through **action
results, not observations**. A change that marked only `Observation` would have
left the larger hole open — which is the reverse of what the field's framing
suggests, and the reason this was done from the code.

**A second defect, in the same function.** An environment whose `Observe`
returned an error was skipped with a bare `continue`, so an environment that
had gone blind was indistinguishable from one that looked and found the world
unchanged. Phase 20 refused exactly that conflation on the outer loop — a blind
environment is named in the outcome and its objective is driven by its schedule
instead, because a calendar saying "I don't know" read as "unchanged" goes quiet
precisely when it should not. The inner loop never learned it.

## Decision 1 — the payload declares its trust, and the environment is what declares it

`environment.Trust` is a field on both `Observation` and `ActionResult`. An
environment knows whether what it is handing back is infrastructure an operator
configured or prose a third party wrote; the loop does not.

Inferring it from an `EnvID` would be the name-suffix mistake ADR 019 exists to
record, and the two obvious guesses are both wrong in this tree:
`software.env.communication` reads like prose and returns an adapter name,
while `software.env.git` reads like infrastructure and carries titles a stranger
typed.

It is on the **payload** rather than on the `Factory`, unlike `Serves`. A
factory-level declaration would be a property of the environment, and the
property that matters is a property of what came back: `gitEnv` observing a
repository with no open pull requests has carried nobody's writing, and a
deployment where every plan escalates because the repository *might one day*
have a pull request is a deployment whose operator switches the mechanism off.
The shipped environments compute it from what they put in the map.

**The zero value is trusted, and that is a real cost.** ADR 019 argues the
opposite default for `AuthorityBounds` — bounds nobody filled in permit nothing,
so forgetting fails toward asking. The trade is different here: untrusted-by-
default would escalate every plan in every deployment until each of the shipped
environments had been labelled, and a gate that fires on everything is a gate
nobody reads. A payload that carries somebody's prose and forgets to say so is
a pack bug, and it is one no check outside the pack can find — see Decision 4.

## Decision 2 — the policy grows an input, not a second gate

`AuthorityBounds.Decide(confidence, threshold, plannedCapabilities, ev Evidence)`.

The signature change is the change; the branch is the small part. The bounds saw
no observations and no per-action provenance at all — everything the policy knew
was about the plan, and nothing about what the plan was drafted from — so this
is worth stating as a new input rather than describing it as one more condition.

It stays the **only** gate. ADR 015 is explicit that authority for a run is
expressed by writing `agent.AuthorityBounds` into the request, never by adding a
second check beside the one that exists, and a provenance gate bolted onto
`stepDecide` would have been exactly that. `Evidence` carries source *names*,
never the material: the policy does not read prose and does not rank it by how
suspicious it looks.

**Provenance is checked first**, ahead of confidence. The existing ordering
comment says a more specific reason wins because it is the one an operator can
act on; "a stranger's writing was in front of the planner, here is which
source" is a fact about the world a reviewer can go and check, where confidence
is the model's report on itself.

**It escalates whatever autonomy the agent has earned.** An agent with
`UnlimitedActions` is precisely the one this is for. Earned autonomy (ADR 016)
is earned against the operator's own infrastructure; nothing about a track
record over commits says anything about a plan drafted from a comment somebody
left this morning.

Evidence **accumulates across the run** rather than resetting each iteration.
Material a planner has already read cannot be un-read, and the act path in
particular delivers its payload one step *after* the decision that would have
gated it — so the plan a scraped page justifies is the next one.

## Decision 3 — blind environments are named

`loop.WorldState.Blind` lists the environments whose `Observe` returned an
error, the same name `reconcile.Fingerprint.Blind` uses for the same reason.
"Saw nothing" and "could not see" are now distinguishable from outside, in the
iteration record and on the `loop_step_completed{step: observe}` event.

## Decision 4 — conformance checks that the label changes the outcome, and says what it cannot check

`checkProvenanceEscalates` runs each pack's declared bounds through the real
`Decide` twice — the same plan, the same confidence, once with a synthetic
third-party source and once without — and asserts the two answers differ in
exactly that dimension. Reverting the branch in `Decide` fails it in every pack
that declares an agent.

The other half of the ladder is asserted too, so "escalate everything" cannot
pass: the trusted verdict must not blame a source, and an agent that is neither
bounded to zero actions nor holding an approval list must not escalate at all.

What it deliberately does **not** check is whether a pack has labelled its own
environments honestly. That is not decidable from outside the pack: an
environment returning `TrustOperator` over a chat transcript is indistinguishable
from one returning it over a metric, and no amount of running the pack reveals
which. A suite can check that the label it is given changes the outcome. It
cannot check that the label is true.

## Decision 5 — the planner is told, in the prompt, because nothing else reaches it

`stepReason` writes a provenance notice into the task text naming the untrusted
sources and the blind environments.

This is not the defence — telling a model some of its input is untrusted does
not stop the input from steering it, which is the whole reason the escalation in
Decision 2 is what holds. It is in the **prompt** rather than left to the
`Trust` field on `WorldState` because `buildUserPrompt` does not serialise
`WorldState` at all: it renders the objective, the task, and a count of memory
entries. A field the model never sees would be a declaration nobody reads, which
is the defect class ADR 020 exists for.

## Consequences

- **A deployment watching a repository with open pull requests will escalate
  more.** That is the mechanism working, and it is a real cost in reviewer
  attention rather than a free improvement. It is bounded by being computed per
  payload: no pull requests, no escalation.

- **No dial was added.** An operator who wants a source exempted has no switch
  for it, deliberately: the first thing a dial would be used for is switching
  off the case it was built for.

- **`gitEnv`, `ticketEnv`, `commsEnv` and `researchEnv` declare; every other
  shipped environment takes the zero value.** `commsEnv` labels the branch that
  would carry message bodies even though `stepObserve` cannot currently reach
  it, so that wiring a channel filter later is a one-line change that is already
  governed rather than a hole that opens quietly.

- **Within one iteration, an untrusted action result reaches `stepVerify`
  before it reaches a `Decide`.** The step order is observe → reason → decide →
  act → verify, so material a search brings back is scored by the judge in the
  same pass that fetched it and gated on the next one. That is the accumulation
  design working as intended rather than a gap being papered over: verify scores
  and does not act, and the first plan the material could justify is the next
  one. It is worth knowing that the judge sees it ungated.

- **Evidence is per-process.** A loop resumed after a server restart
  (`ResumeStoredLoops`) starts with an empty `Evidence` and rebuilds it from the
  next observation. The escalation that was raised before the restart persists —
  the sources are in the audit row — but a restart between an untrusted
  observation and the decision it would have gated loses that link. Persisting
  it would mean a column and a migration for a window measured in one iteration,
  and the re-observation closes it.

- **This does not detect injected instructions**, and declines to rank prose by
  how suspicious it reads. It marks where text came from and makes a human look
  before Karakuri acts on it, which is a property that holds against attacks
  nobody has thought of yet — where a classifier is only as good as its last
  training set.

## Alternatives considered

**A `Factory.Trust` declaration, symmetrical with `Serves`.** Rejected in
Decision 1: it makes the label a property of the environment when the property
that matters belongs to the payload, and it escalates on repositories that have
carried nobody's writing.

**Both — a factory declaration for conformance to read, and a payload field for
the loop.** Two declarations that can disagree, which is the shape ADR 019
records as a defect: `selfimprove.go` had a hand-written `servedBy` map beside
the environments, correct only while somebody remembered to update it.

**Untrusted as the zero value.** Correct by the ADR 019 argument and rejected on
the trade in Decision 1. Worth revisiting if the escalation rate ever turns out
to be lower than expected.

**Ranking or classifying the prose.** A model asked whether text looks like an
injection is a model that can be talked out of its answer, and it fails against
the attacks that matter — the ones nobody has written a detector for.

**A second gate in `stepDecide`, beside `Decide`.** Rejected on ADR 015: one
gate, and authority is expressed by what is written into the bounds.

**Trusting the planner to report which evidence it relied on.** The plan's
`reasoning` field is written by the same model the untrusted material is
steering. Asking the suspect.
