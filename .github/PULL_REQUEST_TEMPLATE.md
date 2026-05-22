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
