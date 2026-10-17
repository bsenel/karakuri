<!-- Title: `<type>(<scope>): <summary>`, e.g. `fix(cli): shift audit export window to the requested hours (EB-003)`; `!` before the colon if breaking.
     It becomes the squash commit on main, and the version is derived from it: feat is minor, fix and perf are patch.
     Checked by `PR title` (.github/workflows/pr-title.yml); try one with `go run ./cmd/prtitle '<title>'`. -->

## Summary

<!-- 1-3 bullets describing what changed and why. Link the issue or roadmap phase. -->

## Test plan

<!-- Mark each item as you confirm it before requesting review. -->

- [ ] `go build ./...` clean
- [ ] `go test ./... -count=1` passes
- [ ] Affected feature manually exercised (`./bin/server` + `krk` or browser)
- [ ] New behavior covered by a test, or rationale noted for why not
- [ ] Roadmap / docs updated if the change is user-visible

## Risk

<!-- One sentence on blast radius. Schema migration? Public API change? Adapter behavior change? Just docs? -->

## Notes for reviewer

<!-- Anything non-obvious from the diff: design tradeoffs, places to look first, follow-ups deferred. -->
