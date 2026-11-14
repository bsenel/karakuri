package software

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 44 step 1: a delivery pull request is marked ready when its group is
// done, which is every item of its body's checklist ticked and every check at
// its head green, by ci_green's rules and without its wait.
const (
	capMarkReady capability.CapabilityID = "software.act.mark_ready"
	capCreatePR  capability.CapabilityID = "software.act.create_pr"
)

// groupBody is a delivery pull request's body whose checklist names its
// group's steps. The X of the second item is upper case, which ticks it too.
const groupBody = `Phase 44, group 1.

- [x] Step 1a: read a pull request's body, base and draft (3f1a245)
- [X] Step 1b: mark one ready

Tests: go test ./domains/software/...`

// draftPR is the stream's pull request, a draft with its head at c2.
func draftPR(body string) versioncontrol.PRSummary {
	pr := streamPR(passStart.Add(time.Minute), passStart.Add(time.Minute))
	pr.Number, pr.Body, pr.BaseBranch, pr.Draft, pr.NodeID = 201, body, "main", true, "PR_kwDO201"
	return pr
}

// greenChecks are green by ci_green's rules: a skipped check blocks nothing,
// and a cancelled run that a later green run of the same check superseded does
// not speak for the check.
func greenChecks() []versioncontrol.CheckRun {
	return []versioncontrol.CheckRun{
		run(20, "PR title", "cancelled"),
		run(21, "PR title", "success"),
		run(22, "build", "success"),
		run(23, "test", "success"),
		run(24, "SBOM (Syft)", "skipped"),
	}
}

func actMarkReady(t *testing.T, forge *fakeForge, params map[string]any) (environment.ActionResult, time.Duration) {
	t.Helper()
	start := time.Now()
	res, err := (&gitEnv{id: EnvGit, vc: forge}).Act(context.Background(), environment.Action{CapabilityID: capMarkReady, Params: params})
	if err != nil {
		t.Fatalf("act %s: %v", capMarkReady, err)
	}
	return res, time.Since(start)
}

// A ticked, green group is marked ready in the same pass, once, on the pull
// request the branch heads, in the repository the params name.
func TestMarkReadyMarksATickedGreenGroupReady(t *testing.T) {
	forge := &fakeForge{prs: []versioncontrol.PRSummary{draftPR(groupBody)}, checks: greenChecks()}
	res, _ := actMarkReady(t, forge, map[string]any{"branch": streamBranch, "repo": "acme/widgets"})

	if !res.Success {
		t.Fatalf("Success = false (error %q), want a ticked, green group marked ready", res.Error)
	}
	if want := []string{"acme/widgets#201"}; !reflect.DeepEqual(forge.markedReady, want) {
		t.Errorf("MarkReady calls = %v, want %v", forge.markedReady, want)
	}
	if got := fmt.Sprint(res.StateDelta["pr"]); got != "201" {
		t.Errorf("state_delta pr = %q, want 201", got)
	}
	if got, _ := res.StateDelta["marked_ready"].(bool); !got {
		t.Errorf("state_delta marked_ready = %v, want true", res.StateDelta["marked_ready"])
	}
}

// Anything short of a ticked, green group leaves the pull request a draft and
// is not a success, and the result says what stands in the way.
func TestMarkReadyLeavesAnUnfinishedGroupADraft(t *testing.T) {
	unticked := groupBody + "\n- [ ] Step 1c: open the next group stacked on this one"
	pendingChecks := append(greenChecks(), check("e2e", "c2", "in_progress", ""))
	failedChecks := append(greenChecks(), run(25, "lint", "failure"))
	elsewhere := draftPR(groupBody)
	elsewhere.HeadBranch = "karakuri/discovery-2026-10-17"

	for _, tc := range []struct {
		name   string
		prs    []versioncontrol.PRSummary
		checks []versioncontrol.CheckRun
		// names is what the error must name.
		names string
	}{
		{"an unticked item", []versioncontrol.PRSummary{draftPR(unticked)}, greenChecks(), "Step 1c: open the next group stacked on this one"},
		{"a failed check", []versioncontrol.PRSummary{draftPR(groupBody)}, failedChecks, "lint"},
		{"a pending check", []versioncontrol.PRSummary{draftPR(groupBody)}, pendingChecks, "e2e"},
		{"no check at the head", []versioncontrol.PRSummary{draftPR(groupBody)}, nil, ""},
		// "Every one of zero items is ticked" must not mark a pull request ready.
		{"no checklist", []versioncontrol.PRSummary{draftPR("Phase 44, group 1. No steps listed.")}, greenChecks(), ""},
		{"no open pull request on the branch", nil, greenChecks(), streamBranch},
		{"an open pull request on another branch only", []versioncontrol.PRSummary{elsewhere}, greenChecks(), streamBranch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := &fakeForge{prs: tc.prs, checks: tc.checks}
			res, elapsed := actMarkReady(t, forge, map[string]any{"branch": streamBranch})
			if len(forge.markedReady) != 0 {
				t.Errorf("MarkReady was called %v", forge.markedReady)
			}
			if res.Success {
				t.Errorf("Success = true, want an unfinished group left a draft (state_delta %v)", res.StateDelta)
			}
			if res.Error == "" {
				t.Error("the result does not say why the pull request was not marked ready")
			}
			if tc.names != "" && !strings.Contains(res.Error, tc.names) {
				t.Errorf("error %q does not name %q", res.Error, tc.names)
			}
			// ci_green's rules without its wait: a pending check is read once.
			if elapsed > 10*time.Second {
				t.Errorf("took %v; mark_ready does not wait for pending checks", elapsed)
			}
		})
	}
}

// A pull request already out of draft is left alone, and that is a success:
// the group's pull request is ready, which is what was asked.
func TestMarkReadyLeavesAReadyPullRequestAlone(t *testing.T) {
	pr := draftPR(groupBody)
	pr.Draft = false
	forge := &fakeForge{prs: []versioncontrol.PRSummary{pr}, checks: greenChecks()}
	res, _ := actMarkReady(t, forge, map[string]any{"branch": streamBranch})

	if len(forge.markedReady) != 0 {
		t.Errorf("MarkReady was called %v on a pull request already ready", forge.markedReady)
	}
	if !res.Success {
		t.Errorf("Success = false (error %q), want success for a pull request already ready", res.Error)
	}
	if got, _ := res.StateDelta["already_ready"].(bool); !got {
		t.Errorf("state_delta already_ready = %v, want true", res.StateDelta["already_ready"])
	}
}

// A forge that cannot mark a pull request ready says so (AGENTS.md rule 10),
// and that is not a success.
func TestMarkReadyReportsAnUnsupportedForge(t *testing.T) {
	forge := &fakeForge{
		prs:          []versioncontrol.PRSummary{draftPR(groupBody)},
		checks:       greenChecks(),
		markReadyErr: fmt.Errorf("fake_forge: mark ready: %w", versioncontrol.ErrUnsupported),
	}
	res, _ := actMarkReady(t, forge, map[string]any{"branch": streamBranch})

	if res.Success {
		t.Error("Success = true when the forge cannot mark a pull request ready")
	}
	if !strings.Contains(strings.ToLower(res.Error), "unsupported") {
		t.Errorf("error %q does not say unsupported", res.Error)
	}
}

// The unticked item is text the pull request's author wrote, so a result that
// names it carries the pull request's provenance (AGENTS.md rule 9).
func TestMarkReadyCarriesThePullRequestsProvenance(t *testing.T) {
	pr := draftPR(groupBody + "\n- [ ] Step 1c: open the next group")
	pr.OwnAuthor = false
	forge := &fakeForge{prs: []versioncontrol.PRSummary{pr}, checks: greenChecks()}
	res, _ := actMarkReady(t, forge, map[string]any{"branch": streamBranch})

	if res.Trust != environment.TrustThirdParty {
		t.Errorf("Trust = %q, want %q for a checklist somebody else wrote", res.Trust, environment.TrustThirdParty)
	}
}

// mark_ready is served where create_pr is, by the git environment that holds
// the version control adapter.
func TestMarkReadyRoutesToGit(t *testing.T) {
	reg := environment.NewRegistry()
	for _, f := range New().EnvironmentFactories() {
		if err := reg.Register(f); err != nil {
			t.Fatalf("register %s: %v", f.EnvID, err)
		}
	}
	for _, id := range []capability.CapabilityID{capMarkReady, capCreatePR} {
		if got := reg.ServedBy(id); !reflect.DeepEqual(got, []environment.EnvironmentID{EnvGit}) {
			t.Errorf("%s is served by %v, want only %s", id, got, EnvGit)
		}
	}
}

// Marking a pull request ready asks a person to review it, so it sits behind
// the same bounds as opening one: declared alike, held and gated by the same
// agents, and in the set the maintainer must hold none of.
func TestMarkReadyIsBoundedLikeCreatePR(t *testing.T) {
	pack := New()
	declared := map[capability.CapabilityID]capability.Capability{}
	for _, c := range pack.Capabilities() {
		declared[c.ID] = c
	}
	markReady, ok := declared[capMarkReady]
	if !ok {
		t.Fatalf("%s is not declared", capMarkReady)
	}
	createPR := declared[capCreatePR]
	if markReady.Verifiable != createPR.Verifiable {
		t.Errorf("Verifiable = %v, create_pr's is %v", markReady.Verifiable, createPR.Verifiable)
	}
	if !slices.Contains(markReady.InputSchema.Required, "branch") {
		t.Errorf("Required = %v, want branch", markReady.InputSchema.Required)
	}
	if _, ok := markReady.InputSchema.Properties["repo"]; !ok || slices.Contains(markReady.InputSchema.Required, "repo") {
		t.Errorf("repo is not an optional input (properties %v, required %v)",
			markReady.InputSchema.Properties, markReady.InputSchema.Required)
	}

	for _, def := range pack.AgentDefinitions() {
		if a, b := slices.Contains(def.Capabilities, capMarkReady), slices.Contains(def.Capabilities, capCreatePR); a != b {
			t.Errorf("%s holds mark_ready %v and create_pr %v", def.ID, a, b)
		}
		gated := def.Authority.RequiresApprovalFor
		if a, b := slices.Contains(gated, capMarkReady), slices.Contains(gated, capCreatePR); a != b {
			t.Errorf("%s gates mark_ready %v and create_pr %v", def.ID, a, b)
		}
	}

	mutating := mutatingCapabilities()
	if !slices.Contains(mutating, capMarkReady) || !slices.Contains(mutating, capCreatePR) {
		t.Errorf("mutatingCapabilities() = %v, want both create_pr and mark_ready", mutating)
	}
}

// A later group of a phase is opened stacked on the earlier group's head:
// create_pr hands params.base_branch to the forge as the base.
func TestCreatePROpensALaterGroupStackedOnTheEarlierHead(t *testing.T) {
	const earlierHead = "karakuri/phase-44-mark-ready"
	forge := &fakeForge{}
	res, err := (&gitEnv{id: EnvGit, vc: forge}).Act(context.Background(), environment.Action{
		CapabilityID: capCreatePR,
		Params: map[string]any{
			"title":       "feat(software): keep what a killed action wrote (phase 44 step 2)",
			"body":        "- [ ] Step 2: salvage branch",
			"branch":      "karakuri/phase-44-salvage",
			"base_branch": earlierHead,
		},
	})
	if err != nil {
		t.Fatalf("act %s: %v", capCreatePR, err)
	}
	if !res.Success {
		t.Fatalf("Success = false (error %q)", res.Error)
	}
	if len(forge.created) != 1 {
		t.Fatalf("CreatePR was called %d times, want 1", len(forge.created))
	}
	got := forge.created[0]
	if got.BaseBranch != earlierHead {
		t.Errorf("base = %q, want the earlier group's head %q", got.BaseBranch, earlierHead)
	}
	if got.HeadBranch != "karakuri/phase-44-salvage" {
		t.Errorf("head = %q, want karakuri/phase-44-salvage", got.HeadBranch)
	}
}
