package software

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 44 step 5: a pull request that changes a verifier meets this forge's
// own data before it is marked ready. mark_ready runs the changed verifier's
// rule, read-only, over the check runs of the repository's open and most
// recently merged pull requests and compares each verdict with what the forge
// itself reports. A disagreement is a finding and leaves the pull request a
// draft; each verdict is written beside the forge's status into the body.
//
// The recorded responses are #187's story (the Why of Phase 44): this
// repository's CI skips "SBOM (Syft)" on every pull request, and #189's head
// carries a cancelled "PR title" run beside the green one, yet the forge shows
// both pull requests green.

// verifierFile is a file that implements a forge verifier.
const verifierFile = "domains/software/stream_verifiers.go"

// openGreen is #189, open, which the forge shows green although its head
// carries a cancelled "PR title" run that a green one superseded.
func openGreen() versioncontrol.PRSummary {
	return versioncontrol.PRSummary{ID: "189", Number: 189, URL: "https://example.invalid/pull/189",
		HeadBranch: "karakuri/phase-41-step-09", HeadSHA: "h189", CheckState: "success"}
}

// mergedGreen is #187, merged, which the forge shows green with its SBOM
// check skipped.
func mergedGreen() versioncontrol.PRSummary {
	return versioncontrol.PRSummary{ID: "187", Number: 187, URL: "https://example.invalid/pull/187",
		HeadBranch: "karakuri/phase-41", HeadSHA: "h187", CheckState: "success"}
}

func runAt(id int64, sha, name, conclusion string) versioncontrol.CheckRun {
	return versioncontrol.CheckRun{ID: id, Name: name, HeadSHA: sha, Status: "completed", Conclusion: conclusion}
}

// recordedChecks are the check runs the forge reports for the pull request
// under review (head c2) and for #189 and #187. Every head has "SBOM (Syft)"
// skipped and everything else green.
func recordedChecks() []versioncontrol.CheckRun {
	return append(greenChecks(),
		runAt(30, "h189", "PR title", "cancelled"),
		runAt(31, "h189", "PR title", "success"),
		runAt(32, "h189", "build", "success"),
		runAt(33, "h189", "SBOM (Syft)", "skipped"),
		runAt(40, "h187", "PR title", "success"),
		runAt(41, "h187", "build", "success"),
		runAt(42, "h187", "SBOM (Syft)", "skipped"),
	)
}

// recordedForge is the forge as recorded: the draft under review on
// streamBranch, ticked and green by the deployed rules, whose diff touches
// files; #189 open; #187 merged.
func recordedForge(files ...string) *fakeForge {
	return &fakeForge{
		prs:    []versioncontrol.PRSummary{draftPR(groupBody), openGreen()},
		merged: []versioncontrol.PRSummary{mergedGreen()},
		checks: recordedChecks(),
		files:  files,
	}
}

// skippedIsNotRun is #187's first wrong rule: a skipped check did not run, so
// a head that skips one is inconclusive.
func skippedIsNotRun(checks []versioncontrol.CheckRun) objective.CriterionState {
	for _, c := range latestRuns(checks) {
		switch c.Conclusion {
		case "success":
		case "skipped":
			return objective.CriterionInconclusive
		default:
			return objective.CriterionNotMet
		}
	}
	return objective.CriterionMet
}

// everyRunCounts is #187's second wrong rule: every run of a check speaks for
// it, so a cancelled run superseded by a green one is not met.
func everyRunCounts(checks []versioncontrol.CheckRun) objective.CriterionState {
	for _, c := range checks {
		if c.Conclusion != "success" && c.Conclusion != "skipped" {
			return objective.CriterionNotMet
		}
	}
	return objective.CriterionMet
}

// latestRunsGreen agrees with the forge: the latest run of each check counts,
// and a skipped one blocks nothing.
func latestRunsGreen(checks []versioncontrol.CheckRun) objective.CriterionState {
	failing, pending, notRun, _, green := sortChecks(latestRuns(checks))
	switch {
	case len(failing) > 0:
		return objective.CriterionNotMet
	case len(pending) > 0, len(notRun) > 0, len(green) == 0:
		return objective.CriterionInconclusive
	}
	return objective.CriterionMet
}

func fixtureVerifier(rule func([]versioncontrol.CheckRun) objective.CriterionState) []verifierRule {
	return []verifierRule{{id: string(capCIGreen), files: []string{verifierFile}, rule: rule}}
}

func actMarkReadyWith(t *testing.T, forge *fakeForge, verifiers []verifierRule) environment.ActionResult {
	t.Helper()
	env := &gitEnv{id: EnvGit, vc: forge, verifiers: verifiers}
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: capMarkReady,
		Params: map[string]any{"branch": streamBranch, "repo": "acme/widgets"}})
	if err != nil {
		t.Fatalf("act %s: %v", capMarkReady, err)
	}
	return res
}

// verdictLine finds the line of body that names pull request #id; empty when
// none does.
func verdictLine(body, id string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "#"+id) {
			return line
		}
	}
	return ""
}

// onlyBodyEdit asserts that the comparison wrote nothing to the forge but one
// edit of the reviewed pull request's body, which keeps its checklist.
func onlyBodyEdit(t *testing.T, forge *fakeForge) bodyEdit {
	t.Helper()
	if len(forge.created) != 0 {
		t.Errorf("CreatePR was called %d times; the comparison is read-only", len(forge.created))
	}
	if len(forge.bodyEdits) != 1 {
		t.Fatalf("EditPRBody calls = %+v, want one edit of pull request 201's body", forge.bodyEdits)
	}
	edit := forge.bodyEdits[0]
	if edit.prID != "201" || edit.repo != "acme/widgets" {
		t.Errorf("EditPRBody edited %s#%s, want acme/widgets#201, the pull request under review", edit.repo, edit.prID)
	}
	if !strings.Contains(edit.body, groupBody) {
		t.Errorf("the edited body lost the group's checklist:\n%s", edit.body)
	}
	return edit
}

// A verifier whose rule disagrees with a recorded forge response is a finding:
// the pull request stays a draft and the result names the finding.
func TestVerifierMeetsDataFindsADisagreement(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule func([]versioncontrol.CheckRun) objective.CriterionState
		// disagree are the pull requests the verifier misreads, and verdict
		// what it calls them, where the forge shows them green.
		disagree []string
		verdict  objective.CriterionState
	}{
		// (a) Every recorded head skips "SBOM (Syft)".
		{"a skipped check counted as not run", skippedIsNotRun, []string{"187", "189"}, objective.CriterionInconclusive},
		// (b) #189's head carries a cancelled "PR title" run superseded by a
		// green one.
		{"a superseded cancelled run counted", everyRunCounts, []string{"189"}, objective.CriterionNotMet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := recordedForge(verifierFile, "domains/software/stream_verifiers_test.go")
			res := actMarkReadyWith(t, forge, fixtureVerifier(tc.rule))

			if len(forge.markedReady) != 0 {
				t.Errorf("MarkReady was called %v; a verifier that misreads this forge is not ready", forge.markedReady)
			}
			if res.Success {
				t.Errorf("Success = true, want a finding (state_delta %v)", res.StateDelta)
			}
			if forge.mergedReads == 0 {
				t.Error("the merged pull requests were never read")
			}
			for _, want := range append([]string{string(capCIGreen), string(tc.verdict)}, tc.disagree...) {
				if !strings.Contains(res.Error, want) {
					t.Errorf("error %q does not name %q", res.Error, want)
				}
			}
			findings, _ := res.StateDelta["findings"].([]string)
			if len(findings) != len(tc.disagree) {
				t.Errorf("state_delta findings = %v, want one per pull request in %v", res.StateDelta["findings"], tc.disagree)
			}
			for _, id := range tc.disagree {
				named := false
				for _, f := range findings {
					named = named || strings.Contains(f, "#"+id)
				}
				if !named {
					t.Errorf("no finding names #%s: %v", id, findings)
				}
			}

			// The reviewer sees the comparison even when it fails.
			edit := onlyBodyEdit(t, forge)
			for _, id := range tc.disagree {
				line := verdictLine(edit.body, id)
				if !strings.Contains(line, string(tc.verdict)) || !strings.Contains(line, "success") {
					t.Errorf("body line for #%s = %q, want the verdict %q beside the forge's success", id, line, tc.verdict)
				}
			}
		})
	}
}

// (c) A verifier that agrees with every recorded response is marked ready, and
// the body carries each verdict beside the forge's status.
func TestVerifierMeetsDataAgreesAndIsMarkedReady(t *testing.T) {
	forge := recordedForge(verifierFile)
	res := actMarkReadyWith(t, forge, fixtureVerifier(latestRunsGreen))

	if !res.Success {
		t.Fatalf("Success = false (error %q), want a verifier that agrees with this forge marked ready", res.Error)
	}
	if want := []string{"acme/widgets#201"}; !reflect.DeepEqual(forge.markedReady, want) {
		t.Errorf("MarkReady calls = %v, want %v", forge.markedReady, want)
	}
	if findings, _ := res.StateDelta["findings"].([]string); len(findings) != 0 {
		t.Errorf("state_delta findings = %v, want none", findings)
	}
	edit := onlyBodyEdit(t, forge)
	for _, id := range []string{"187", "189"} {
		line := verdictLine(edit.body, id)
		if line == "" {
			t.Errorf("the body names no verdict for #%s:\n%s", id, edit.body)
			continue
		}
		for _, want := range []string{string(capCIGreen), string(objective.CriterionMet), "success"} {
			if !strings.Contains(line, want) {
				t.Errorf("body line for #%s = %q, want it to carry %q", id, line, want)
			}
		}
		if strings.Contains(line, string(objective.CriterionNotMet)) {
			t.Errorf("body line for #%s = %q calls an agreeing verdict not met", id, line)
		}
	}
}

// (d) A pull request that changes no verifier is marked ready exactly as
// before: no merged pull request read, no other head's checks read, no body
// edit.
func TestVerifierMeetsDataRunsNoComparisonWithoutAVerifierChange(t *testing.T) {
	forge := recordedForge("docs/roadmap.md", "domains/software/pr_title.go")
	res := actMarkReadyWith(t, forge, fixtureVerifier(skippedIsNotRun))

	if !res.Success {
		t.Fatalf("Success = false (error %q), want the group marked ready as before", res.Error)
	}
	if want := []string{"acme/widgets#201"}; !reflect.DeepEqual(forge.markedReady, want) {
		t.Errorf("MarkReady calls = %v, want %v", forge.markedReady, want)
	}
	if forge.mergedReads != 0 {
		t.Errorf("ListMergedPRs was called %d times with no verifier changed", forge.mergedReads)
	}
	if forge.checkReads != 1 {
		t.Errorf("ListChecks was called %d times, want once, for the head under review", forge.checkReads)
	}
	if len(forge.bodyEdits) != 0 {
		t.Errorf("EditPRBody was called %+v with no verifier changed", forge.bodyEdits)
	}
	if _, ok := res.StateDelta["findings"]; ok {
		t.Errorf("state_delta carries findings %v with no comparison run", res.StateDelta["findings"])
	}
}

// (e) The comparison is read-only: whatever it finds, the only writes to the
// forge are the body edit and, when every verdict agrees, the mark-ready call
// on the pull request under review.
func TestVerifierMeetsDataWritesOnlyTheBodyAndTheMarkReady(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rule      func([]versioncontrol.CheckRun) objective.CriterionState
		wantReady []string
	}{
		{"a finding", everyRunCounts, nil},
		{"every verdict agrees", latestRunsGreen, []string{"acme/widgets#201"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forge := recordedForge(verifierFile)
			actMarkReadyWith(t, forge, fixtureVerifier(tc.rule))

			onlyBodyEdit(t, forge)
			if fmt.Sprint(forge.markedReady) != fmt.Sprint(tc.wantReady) {
				t.Errorf("MarkReady calls = %v, want %v", forge.markedReady, tc.wantReady)
			}
		})
	}
}
