package software

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// The deterministic stream verifiers of Phase 41 step 2. Each settles one
// criterion from what the forge reports and returns the criterion's state, and
// the reason when it is inconclusive, in StateDelta "state" and "reason".
const (
	capStreamPR      capability.CapabilityID = "software.verify.stream_pr"
	capCIGreen       capability.CapabilityID = "software.verify.ci_green"
	capPathsDeclared capability.CapabilityID = "software.verify.paths_declared"
)

// fakeForge is a version-control adapter that answers the verifiers' reads
// from what a test hands it.
type fakeForge struct {
	prs    []versioncontrol.PRSummary
	checks []versioncontrol.CheckRun
	files  []string

	prsErr, checksErr, filesErr error

	// What the verifier asked for, so a test can tell it read the right PR.
	checkReads int
	gotPRID    string
}

func (f *fakeForge) Name() string { return "fake_forge" }
func (f *fakeForge) Active() bool { return true }

func (f *fakeForge) CreatePR(context.Context, versioncontrol.PullRequest) (string, error) {
	return "", nil
}

func (f *fakeForge) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	if f.prsErr != nil {
		return nil, f.prsErr
	}
	return f.prs, nil
}

func (f *fakeForge) GetCommits(context.Context, string, time.Time) ([]versioncontrol.Commit, error) {
	return nil, nil
}

// ListChecks returns every check it holds whatever the ref, as a forge listing
// a branch's checks does, so a check on an older commit reaches the verifier.
func (f *fakeForge) ListChecks(context.Context, string, string) ([]versioncontrol.CheckRun, error) {
	f.checkReads++
	if f.checksErr != nil {
		return nil, f.checksErr
	}
	return f.checks, nil
}

func (f *fakeForge) ChangedFiles(_ context.Context, _, prID string) ([]string, error) {
	f.gotPRID = prID
	if f.filesErr != nil {
		return nil, f.filesErr
	}
	return f.files, nil
}

// passStart is when the pass under verification began.
var passStart = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

const streamBranch = "karakuri/ux-2026-10-17"

// streamPR is the stream's pull request with its head at c2.
func streamPR(created, updated time.Time) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{
		ID: "201", Title: "feat(ux): add id hints", URL: "https://example.invalid/pull/201",
		HeadBranch: streamBranch, HeadSHA: "c2",
		CreatedAt: created, UpdatedAt: updated, OwnAuthor: true,
	}
}

func check(name, sha, status, conclusion string) versioncontrol.CheckRun {
	return versioncontrol.CheckRun{Name: name, HeadSHA: sha, Status: status, Conclusion: conclusion}
}

// run is a check run with its forge ID, for cases where which run is the
// latest matters.
func run(id int64, name, conclusion string) versioncontrol.CheckRun {
	return versioncontrol.CheckRun{ID: id, Name: name, HeadSHA: "c2", Status: "completed", Conclusion: conclusion}
}

func actVerifier(t *testing.T, vc versioncontrol.VersionControlAdapter, id capability.CapabilityID, params map[string]any) environment.ActionResult {
	t.Helper()
	res, err := (&gitEnv{id: EnvGit, vc: vc}).Act(context.Background(), environment.Action{CapabilityID: id, Params: params})
	if err != nil {
		t.Fatalf("act %s: %v", id, err)
	}
	return res
}

// assertVerdict checks the state and reason a verifier settled. An empty
// wantReason on an inconclusive state accepts any reason but none, because an
// inconclusive state always says why. Success is the criterion being met and
// nothing else, so an inconclusive result never reads as a pass.
func assertVerdict(t *testing.T, res environment.ActionResult, id capability.CapabilityID, wantState objective.CriterionState, wantReason string) {
	t.Helper()
	if got := fmt.Sprint(res.StateDelta["verifier"]); got != string(id) {
		t.Errorf("verifier = %q, want %s", got, id)
	}
	state := fmt.Sprint(res.StateDelta["state"])
	if state != string(wantState) {
		t.Errorf("state = %q, want %q (error %q)", state, wantState, res.Error)
	}
	if res.Success != (wantState == objective.CriterionMet) {
		t.Errorf("Success = %v for state %q", res.Success, wantState)
	}
	gotReason := ""
	if reason := res.StateDelta["reason"]; reason != nil {
		gotReason = fmt.Sprint(reason)
	}
	switch {
	case wantState != objective.CriterionInconclusive:
		if gotReason != "" {
			t.Errorf("reason = %q on a %q state; only inconclusive carries one", gotReason, wantState)
		}
	case wantReason == "":
		if gotReason == "" {
			t.Error("an inconclusive state carries no reason")
		}
	case gotReason != wantReason:
		t.Errorf("reason = %q, want %q", gotReason, wantReason)
	}
}

// A pass that opened or updated the stream's pull request did the work the
// stream asks for. No pull request is no data, never a pass and never a
// failure, so a discovery pass that found nothing does not trip the breaker.
func TestStreamPRVerifier(t *testing.T) {
	other := streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))
	other.HeadBranch = "karakuri/discovery-2026-10-17"

	for _, tc := range []struct {
		name       string
		prs        []versioncontrol.PRSummary
		wantState  objective.CriterionState
		wantReason string
	}{
		{"opened in this pass", []versioncontrol.PRSummary{streamPR(passStart.Add(10*time.Minute), passStart.Add(10*time.Minute))},
			objective.CriterionMet, ""},
		{"updated in this pass", []versioncontrol.PRSummary{streamPR(passStart.Add(-48*time.Hour), passStart.Add(5*time.Minute))},
			objective.CriterionMet, ""},
		{"absent", nil, objective.CriterionInconclusive, "no_data"},
		{"only on another branch", []versioncontrol.PRSummary{other}, objective.CriterionInconclusive, "no_data"},
		{"not touched in this pass", []versioncontrol.PRSummary{streamPR(passStart.Add(-48*time.Hour), passStart.Add(-time.Hour))},
			objective.CriterionInconclusive, "no_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := actVerifier(t, &fakeForge{prs: tc.prs}, capStreamPR, map[string]any{
				"branch": streamBranch, "since": passStart.Format(time.RFC3339),
			})
			assertVerdict(t, res, capStreamPR, tc.wantState, tc.wantReason)
		})
	}
}

// Every check at the pull request's head must be green, counting the latest
// run of each check. Pending, absent, cancelled and awaiting-approval checks
// settle nothing, and neither does a green check on a commit the head has
// moved past. A skipped or neutral check blocks nothing, as on the forge's own
// merge gate, but checks that were all skipped are no data.
func TestCIGreenVerifier(t *testing.T) {
	pr := []versioncontrol.PRSummary{streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))}

	for _, tc := range []struct {
		name       string
		prs        []versioncontrol.PRSummary
		checks     []versioncontrol.CheckRun
		wantState  objective.CriterionState
		wantReason string
	}{
		{"green", pr, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
			check("test", "c2", "completed", "success"),
		}, objective.CriterionMet, ""},
		{"red", pr, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
			check("test", "c2", "completed", "failure"),
		}, objective.CriterionNotMet, ""},
		{"pending", pr, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
			check("test", "c2", "in_progress", ""),
		}, objective.CriterionInconclusive, ""},
		{"absent", pr, nil, objective.CriterionInconclusive, ""},
		// This repository's CI skips "SBOM (Syft)" on every pull request.
		{"skipped beside green", pr, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
			check("deploy-preview", "c2", "completed", "skipped"),
		}, objective.CriterionMet, ""},
		{"neutral beside green", pr, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
			check("lint", "c2", "completed", "neutral"),
		}, objective.CriterionMet, ""},
		{"only skipped", pr, []versioncontrol.CheckRun{
			check("deploy-preview", "c2", "completed", "skipped"),
		}, objective.CriterionInconclusive, "no_data"},
		// An edited pull request re-runs a check and cancels the first run, as
		// "PR title" was on #189.
		{"cancelled run superseded by a green one", pr, []versioncontrol.CheckRun{
			run(20, "PR title", "cancelled"),
			run(21, "PR title", "success"),
			run(22, "build", "success"),
		}, objective.CriterionMet, ""},
		{"green run superseded by a red one", pr, []versioncontrol.CheckRun{
			run(31, "test", "failure"),
			run(30, "test", "success"),
		}, objective.CriterionNotMet, ""},
		{"latest run cancelled", pr, []versioncontrol.CheckRun{
			run(40, "build", "success"),
			run(41, "test", "success"),
			run(42, "test", "cancelled"),
		}, objective.CriterionInconclusive, "not_run"},
		{"awaiting approval", pr, []versioncontrol.CheckRun{
			run(50, "build", "success"),
			run(51, "deploy", "action_required"),
		}, objective.CriterionInconclusive, "not_run"},
		{"green on an earlier commit than the head", pr, []versioncontrol.CheckRun{
			check("build", "c1", "completed", "success"),
			check("test", "c1", "completed", "success"),
		}, objective.CriterionInconclusive, ""},
		{"no pull request", nil, []versioncontrol.CheckRun{
			check("build", "c2", "completed", "success"),
		}, objective.CriterionInconclusive, "no_data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := actVerifier(t, &fakeForge{prs: tc.prs, checks: tc.checks}, capCIGreen, map[string]any{
				"branch": streamBranch, "wait_sec": 0,
			})
			assertVerdict(t, res, capCIGreen, tc.wantState, tc.wantReason)
		})
	}
}

// A check still pending when the bounded wait ends did not run as far as the
// verifier can tell, and the wait does end.
func TestCIGreenVerifierStillPendingWhenTheWaitEnds(t *testing.T) {
	forge := &fakeForge{
		prs:    []versioncontrol.PRSummary{streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))},
		checks: []versioncontrol.CheckRun{check("test", "c2", "queued", "")},
	}
	start := time.Now()
	res := actVerifier(t, forge, capCIGreen, map[string]any{"branch": streamBranch, "wait_sec": 1})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("a one-second wait took %v", elapsed)
	}
	assertVerdict(t, res, capCIGreen, objective.CriterionInconclusive, "not_run")
	if forge.checkReads == 0 {
		t.Error("settled without reading the checks")
	}
}

// The diff touches only the declared path globs. An empty diff is no data:
// "every one of zero paths is inside" must not settle a criterion.
func TestPathsDeclaredVerifier(t *testing.T) {
	pr := []versioncontrol.PRSummary{streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))}

	for _, tc := range []struct {
		name       string
		prs        []versioncontrol.PRSummary
		files      []string
		paths      any
		wantState  objective.CriterionState
		wantReason string
		// outside is a path the error must name.
		outside string
	}{
		{"every path inside one glob", pr,
			[]string{"docs/research/a.md", "docs/research/2026/b.md"}, []string{"docs/research/**"},
			objective.CriterionMet, "", ""},
		{"every path inside one of two globs", pr,
			[]string{"cli/cmd/objective.go", "web/src/app.ts"}, "cli/**, web/**",
			objective.CriterionMet, "", ""},
		{"one file named exactly", pr,
			[]string{"docs/roadmap.md"}, []any{"docs/roadmap.md"},
			objective.CriterionMet, "", ""},
		{"a path outside the globs", pr,
			[]string{"docs/research/a.md", "internal/feature/loop/verify.go"}, []string{"docs/research/**"},
			objective.CriterionNotMet, "", "internal/feature/loop/verify.go"},
		{"a sibling of the named file", pr,
			[]string{"docs/roadmap.md", "docs/research/a.md"}, []string{"docs/roadmap.md"},
			objective.CriterionNotMet, "", "docs/research/a.md"},
		{"empty diff", pr, nil, []string{"docs/**"}, objective.CriterionInconclusive, "no_data", ""},
		{"no pull request", nil, []string{"docs/a.md"}, []string{"docs/**"}, objective.CriterionInconclusive, "no_data", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := &fakeForge{prs: tc.prs, files: tc.files}
			res := actVerifier(t, forge, capPathsDeclared, map[string]any{
				"branch": streamBranch, "paths": tc.paths,
			})
			assertVerdict(t, res, capPathsDeclared, tc.wantState, tc.wantReason)
			if tc.prs != nil && forge.gotPRID != "201" {
				t.Errorf("changed files read for PR %q, want 201", forge.gotPRID)
			}
			if tc.outside != "" && !strings.Contains(res.Error, tc.outside) {
				t.Errorf("error %q does not name %s", res.Error, tc.outside)
			}
		})
	}
}

// The forge is the source, so a forge that cannot answer settles nothing: an
// unsupported read is inconclusive: unsupported, any other error is
// inconclusive: verifier_error, never a pass and never a failure.
func TestStreamVerifiersAreInconclusiveWhenTheForgeCannotAnswer(t *testing.T) {
	boom := errors.New("github: 502 bad gateway")
	unsupported := fmt.Errorf("fake_forge: checks: %w", versioncontrol.ErrUnsupported)
	pr := []versioncontrol.PRSummary{streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))}
	green := []versioncontrol.CheckRun{check("build", "c2", "completed", "success")}
	inside := []string{"docs/a.md"}

	for _, tc := range []struct {
		name       string
		id         capability.CapabilityID
		forge      *fakeForge
		wantReason string
	}{
		{"stream_pr list error", capStreamPR, &fakeForge{prsErr: boom}, "verifier_error"},
		{"ci_green list error", capCIGreen, &fakeForge{prsErr: boom, checks: green}, "verifier_error"},
		{"ci_green checks error", capCIGreen, &fakeForge{prs: pr, checksErr: boom}, "verifier_error"},
		{"ci_green checks unsupported", capCIGreen, &fakeForge{prs: pr, checksErr: unsupported}, "unsupported"},
		{"paths_declared files error", capPathsDeclared, &fakeForge{prs: pr, filesErr: boom}, "verifier_error"},
		{"paths_declared files unsupported", capPathsDeclared,
			&fakeForge{prs: pr, filesErr: fmt.Errorf("fake_forge: changed files: %w", versioncontrol.ErrUnsupported)}, "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.forge.files == nil {
				tc.forge.files = inside
			}
			res := actVerifier(t, tc.forge, tc.id, map[string]any{
				"branch": streamBranch, "since": passStart.Format(time.RFC3339),
				"wait_sec": 0, "paths": []string{"docs/**"},
			})
			assertVerdict(t, res, tc.id, objective.CriterionInconclusive, tc.wantReason)
		})
	}

	// No version control bound: there is no forge to ask.
	for _, id := range []capability.CapabilityID{capStreamPR, capCIGreen, capPathsDeclared} {
		t.Run(string(id)+" unbound", func(t *testing.T) {
			res := actVerifier(t, versioncontrol.NewNoOp(), id, map[string]any{
				"branch": streamBranch, "wait_sec": 0, "paths": []string{"docs/**"},
			})
			assertVerdict(t, res, id, objective.CriterionInconclusive, "")
		})
	}
}

// The verifiers read the forge, so they are served where the version control
// adapter is bound.
func TestStreamVerifiersRouteToGit(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	for _, id := range []capability.CapabilityID{capStreamPR, capCIGreen, capPathsDeclared} {
		if got := reg.ServedBy(id); !reflect.DeepEqual(got, []environment.EnvironmentID{EnvGit}) {
			t.Errorf("%s is served by %v, want only %s", id, got, EnvGit)
		}
	}
}
