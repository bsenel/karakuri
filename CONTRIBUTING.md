# Contributing to Karakuri

By contributing to this repository, you agree that
your contribution will be distributed under:

- Apache License 2.0
- Karakuri Human Augmentation License Addendum (HALA)

Please review HALA.md before contributing.

## CI on documentation-only pull requests

A pull request whose every changed path is documentation — anything under
`docs/`, or a `*.md` file outside `.github/` — skips the build, test and scan
matrix. The required checks report **skipped**, not green, and skipped satisfies
branch protection. A pull request touching anything else runs exactly what it
always ran, and so does every push to `main` and both weekly scans.

The classification is an allowlist and lives in
[.github/actions/changed-paths](.github/actions/changed-paths/action.yml): an
unrecognised path runs the full matrix, and `.github/**` always does, so a
change to CI or to the classifier is never treated as prose. That is a guard
against mistakes rather than a security boundary — a `pull_request` workflow
runs the branch's own copy of the workflow and the action, so reviewing a diff
that touches `.github/` remains the real control, exactly as it was before this
existed. Secret scanning is never skipped: a credential can be committed in
Markdown as easily as in Go.

**This does not touch the Dependabot review below.** Those pull requests change
`go.mod`, `go.sum` or `package-lock.json`, so they are never documentation-only
and their checks run in full.

## Reviewing Dependabot pull requests

Every Dependabot PR gets a human eyes-on review before merge — no exceptions, no "trivial" bypass, no auto-approval. Patch bumps, group bumps, lockfile-only updates: all reviewed the same way.

**What to check on every Dependabot PR, in this order:**

1. **The diff.** Open `gh pr diff <N>` and read it. For `go.mod`/`go.sum` or `package-lock.json`-only changes, confirm nothing unexpected slipped in (a phantom direct→indirect flip, a new transitive package, an unrelated file edit). For source changes triggered by the bump, walk the call sites.
2. **The upstream release notes / CHANGELOG.** Open the bumped package's release page on GitHub or pkg.go.dev for the version range. Skim for `BREAKING`, `Removed`, `Deprecated`, `SECURITY`. A patch bump can still contain a behavioural change worth knowing.
3. **CI status.** All required checks (Frontend, Build, Vet, Test) must be green on the rebased branch. Informational checks (CodeQL Analyze) shouldn't be regressing either.
4. **Coverage gap.** If the bumped package is on a code path CI doesn't exercise (e.g. Postgres-backed pgvector, real network adapters), call it out in the approval comment and consider a manual smoke before merge.

**Merge workflow:**

```bash
gh pr diff <N>
gh pr view <N> --web   # release notes, CI runs
gh pr review <N> --approve --body "<one-line rationale>"
gh pr merge <N> --squash --delete-branch
```

Do **not** use `--admin` to bypass review for routine Dependabot bumps. `--admin` is reserved for genuine emergencies (security hotfix that can't wait for a second pair of eyes, repo-config bootstrap, etc.) and every use should be noted in the PR comment.

**What Dependabot won't open:**

Major-version bumps are excluded from Dependabot in `.github/dependabot.yml`. They land via manually-opened PRs after compat testing — the maintainer owns the migration work (peer-dep sweeps, deprecated-API audits, integration smoke). Dependabot still surfaces majors with security advisories via the Security tab; act on those manually.
