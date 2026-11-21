## What this does not show

- The coding agent's session-log return-time reading (the second half of step 9a) is not in this group.
- The production reader has not been run against a real forge. Its tests use a recording fake adapter and the no-op adapter; the GitHub adapter's `CompareCommits` is exercised by its own tests only.
- The branch comes from the cut-off loop's planned write actions (`params.branch`, base `params.base_branch`, default `main`, repo `params.repo`). A cut-off pass that planned no write action naming a branch gets `CommitsUnreadable`, never an empty list.
- The acceptance test uses a scripted planner that obeys the brief when it is shown. It proves the brief reaches the planner and that a planner that follows it finishes the work on one branch with one pull request. It cannot prove a real model complies; nothing in the product forces a planner to.
- Nothing stops a planner from choosing a second branch or a second pull request; the notice only tells it not to.
- Full-package runs of `internal/feature/reconcile` and `internal/feature/loop` hit my 9 minute timeout when run together on this machine (two stop-all tests also failed under that load and pass alone). Targeted runs are below.

## Summary

Phase 40 step 9a: the pass that follows a `waiting_for_quota` deferral (step 8) or an `interrupted_by_restart` ending (step 4) is given, by reconcile in its request, the commits already on the stream's branch and the success criteria the cut-off pass had not settled. The loop renders that into the planner's task, so it plans only what is left.

Stacked on #265 (step 9), which carries steps 7 (#261) and 8 (#264).

## Evidence

```
go build ./...                                           -> ok
go vet ./internal/feature/reconcile/ ./internal/feature/loop/ ./internal/core/loop/   -> ok
go vet ./internal/api/ ./internal/feature/reconcile/ ./test/integration/   -> ok
gofmt -l internal/api internal/feature/reconcile   -> nothing listed
go test -count=1 -run 'TestBranchCommitsReader|TestThePassAfterACutOffPass|TestAnUnreadableBranch|TestNoPassRunsBefore' ./internal/feature/reconcile/   -> ok (42.4s)
go test -count=1 -run 'TestReconcileServiceHasABranchCommitsReader|TestOpenAPI' ./test/integration/   -> ok (9.7s)
go test -count=1 -run 'TestStopAllThatChangesNothing|TestStopAllWithANarrowScope' ./internal/feature/reconcile/ -> ok (earlier run)
go test -count=1 -run TestThePassAfterAQuotaCutOff ./internal/feature/loop/                                -> ok (earlier run)
go test -count=1 ./internal/core/...                                                                      -> ok (earlier run)
```

New tests: `internal/feature/reconcile/branch_commits_test.go` (branch and base from planned writes, base defaults to `main`, no branch in any plan is an error, no plan is an error, forge error is an error, no-op adapter wraps `ErrUnsupported`) and `test/integration/branch_commits_wired_test.go` (`api.NewApp` sets `branchCommits`).

`gofmt -l .` lists only files this change does not touch (auth/, domains/, test/integration/postgres_test.go).

## What changed

- `internal/feature/reconcile/run.go`: `resumeBrief` builds `coreloop.Resume` from the stream's latest outcome when it was deferred `waiting_for_quota` or ended `interrupted_by_restart`; unsettled criteria come from the cut-off loop's row (states other than `met`); an unreadable branch sets `CommitsUnreadable`. Outcomes that share a start time are searched for the cut-off one, since a restart's cut-off outcome is recorded at the instant the supervisor finds it.
- `internal/feature/loop/{runner,reason}.go`: the request's `Resume` reaches the planner's task through `buildResumeNotice`; empty (prompt unchanged) when there is none.
- `internal/feature/reconcile/branch_commits.go`: `NewBranchCommitsReader` reads the branch from the latest planned write action of the objective's saved learn iterations and asks the bound version-control adapter for `CompareCommits(repo, base, branch)`; every failure is an error.
- `internal/api/server.go`: wires it with `WithBranchCommits`, resolving the adapter from the twin's `versioncontrol` binding (no-op when unbound).
- `internal/feature/reconcile/resume_brief_test.go`: slice A's tests, plus the fake-clock test (no pass before the return time, the first one after it gets the brief).
- `internal/feature/loop/resume_acceptance_test.go`: fake forge, coding agent out of quota on the third of four actions (after two commits); the second pass is shown the two commits and the unsettled criterion, plans only actions 3 and 4, pushes to the same branch, and exactly one pull request exists.

## Checklist

- [x] Request carries commits and unsettled criteria after quota deferral and after restart
- [x] None carried after an ordinary ending
- [x] An unreadable branch says so and never carries an empty list as if there were none
- [x] Acceptance test: fake coding agent out of quota after the second of four actions, fake clock, one pull request at the end (split across the reconcile clock test and the loop test; see above for what the scripted planner does and does not prove)
- [x] `branchCommits` wired to the version-control adapter in production
- [ ] Session-log return-time reading (not in this group)

Opened by Karakuri's standing objective delivery; a human reviews and merges.
