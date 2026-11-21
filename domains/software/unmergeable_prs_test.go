package software

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 6a: the git observation also lists each stream's open pull
// requests that cannot be merged as they stand, in state["unmergeable_prs"]
// with planner lines in state["unmergeable_prs_lines"]. A pull request is
// listed when the forge reports a conflict with main, when a check failed at
// its head, or when no check was reported at its head noChecksAfter after its
// last update. Every stream's template tells its planner to repair those first,
// before its branches without a pull request (step 6). A forge that cannot say
// whether a pull request can be merged yields "not known", never clean or
// conflicting, and one that cannot list the pull requests is an error on the
// observation, never an empty list (AGENTS.md rule 10).

// unmergeableForge is step 6's branchForge (ListPRs with a zero since returns
// every open pull request, prsErr fails it) with what this step reads on top:
// each pull request's merge state and the checks at each head SHA.
type unmergeableForge struct {
	branchForge
	// mergeStates is what MergeState returns for each pull request ID; an ID
	// missing from it is "clean".
	mergeStates map[string]string
	// mergeErr, when set, is what MergeState returns for every pull request.
	mergeErr error
	// checks is what ListChecks returns for each ref; a ref missing from it
	// has no check.
	checks map[string][]versioncontrol.CheckRun
}

func (f *unmergeableForge) MergeState(_ context.Context, _, prID string) (string, error) {
	if f.mergeErr != nil {
		return "", f.mergeErr
	}
	if s, ok := f.mergeStates[prID]; ok {
		return s, nil
	}
	return "clean", nil
}

func (f *unmergeableForge) ListChecks(_ context.Context, _, ref string) ([]versioncontrol.CheckRun, error) {
	return f.checks[ref], nil
}

// greenCheck is one completed, successful check at sha.
func greenCheck(sha string) []versioncontrol.CheckRun {
	return []versioncontrol.CheckRun{{ID: 1, Name: "test", HeadSHA: sha, Status: "completed", Conclusion: "success"}}
}

// openPR is an open pull request of the deployment's own from branch at sha,
// green unless the test changes it, last updated updated ago.
func openPR(number int, branch, sha string, updated time.Duration) versioncontrol.PRSummary {
	now := time.Now().UTC()
	return versioncontrol.PRSummary{
		ID: fmt.Sprint(number), Number: number, Title: "work on " + branch,
		HeadBranch: branch, HeadSHA: sha, BaseBranch: "main", OwnAuthor: true,
		CheckState: "success",
		CreatedAt:  now.Add(-updated - time.Hour), UpdatedAt: now.Add(-updated),
	}
}

// unmergeablePR is one entry of state["unmergeable_prs"], read through JSON so
// the test does not fix the Go type the implementation chooses. PR may be the
// number or the ID as text; prID reads either.
type unmergeablePR struct {
	PR     any    `json:"pr"`
	Branch string `json:"branch"`
	Stream string `json:"stream"`
	Reason string `json:"reason"`
}

func (u unmergeablePR) prID() string { return fmt.Sprint(u.PR) }

func unmergeablePRs(t *testing.T, state map[string]any) []unmergeablePR {
	t.Helper()
	raw, ok := state["unmergeable_prs"]
	if !ok {
		t.Fatalf("observation has no unmergeable_prs; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal unmergeable_prs: %v", err)
	}
	var out []unmergeablePR
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmergeable_prs is not a list of {pr, branch, stream, reason}: %v (%s)", err, b)
	}
	return out
}

func unmergeableLines(t *testing.T, state map[string]any) []string {
	t.Helper()
	raw, ok := state["unmergeable_prs_lines"]
	if !ok {
		t.Fatalf("observation has no unmergeable_prs_lines; state keys: %v", stateKeys(state))
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal unmergeable_prs_lines: %v", err)
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmergeable_prs_lines is not a list of strings: %v (%s)", err, b)
	}
	return out
}

func observeUnmergeable(forge *unmergeableForge) map[string]any {
	state := map[string]any{}
	observeUnmergeablePRs(context.Background(), forge, "owner/repo", state)
	return state
}

// findUnmergeable is the entry for the pull request with this ID, or false.
func findUnmergeable(list []unmergeablePR, id string) (unmergeablePR, bool) {
	for _, u := range list {
		if u.prID() == id {
			return u, true
		}
	}
	return unmergeablePR{}, false
}

// lineNaming is the first planner line that holds every one of parts.
func lineNaming(lines []string, parts ...string) (string, bool) {
	for _, l := range lines {
		all := true
		for _, p := range parts {
			if !strings.Contains(l, p) {
				all = false
				break
			}
		}
		if all {
			return l, true
		}
	}
	return "", false
}

// (1) A clean pull request with a green check at its head can be merged, so it
// is not listed, and the list is empty rather than missing.
func TestUnmergeablePRsLeaveOutACleanGreenPR(t *testing.T) {
	const branch = "karakuri/ux-2026-11-16"
	forge := &unmergeableForge{
		branchForge: branchForge{vcStub: vcStub{prs: []versioncontrol.PRSummary{openPR(401, branch, "sha401", time.Hour)}}},
		checks:      map[string][]versioncontrol.CheckRun{"sha401": greenCheck("sha401")},
	}

	state := observeUnmergeable(forge)

	if got := unmergeablePRs(t, state); len(got) != 0 {
		t.Fatalf("unmergeable_prs = %+v, want empty for a clean, green pull request", got)
	}
	if _, ok := state["unmergeable_prs_error"]; ok {
		t.Errorf("unmergeable_prs_error set on a forge that was read: %v", state["unmergeable_prs_error"])
	}
	for _, l := range unmergeableLines(t, state) {
		if strings.Contains(l, branch) {
			t.Errorf("planner line names %s, which can be merged: %q", branch, l)
		}
	}
}

// (2) A pull request the forge reports as conflicting with main is listed with
// its stream and the reason "conflicts with main", in state and in a planner
// line. It was last updated long before the observation window, which a read
// bounded to that window would miss.
func TestUnmergeablePRsListAConflictWithItsStream(t *testing.T) {
	cases := map[string]string{
		"karakuri/discovery-2026-11-10":            marketDiscoveryTemplateID,
		"karakuri/tech-backlog-2026-11-10":         engineeringBacklogTemplateID,
		"karakuri/ux-2026-11-10":                   uxImprovementTemplateID,
		"karakuri/phase-34-step-02-later-group":    roadmapDeliveryTemplateID,
		"karakuri/eb-012-rate-limit-resume-repair": roadmapDeliveryTemplateID,
	}
	forge := &unmergeableForge{mergeStates: map[string]string{}, checks: map[string][]versioncontrol.CheckRun{}}
	ids := map[string]string{}
	n := 410
	for branch := range cases {
		n++
		sha := fmt.Sprintf("sha%d", n)
		forge.prs = append(forge.prs, openPR(n, branch, sha, 30*24*time.Hour))
		forge.mergeStates[fmt.Sprint(n)] = "conflicting"
		forge.checks[sha] = greenCheck(sha)
		ids[branch] = fmt.Sprint(n)
	}

	state := observeUnmergeable(forge)

	got := unmergeablePRs(t, state)
	if len(got) != len(cases) {
		t.Fatalf("listed %d pull requests, want %d: %+v", len(got), len(cases), got)
	}
	lines := unmergeableLines(t, state)
	for branch, stream := range cases {
		u, ok := findUnmergeable(got, ids[branch])
		if !ok {
			t.Errorf("pull request %s on %s is not listed; list: %+v", ids[branch], branch, got)
			continue
		}
		if u.Branch != branch {
			t.Errorf("pull request %s: branch = %q, want %q", ids[branch], u.Branch, branch)
		}
		if u.Stream != stream {
			t.Errorf("pull request %s: stream = %q, want %q", ids[branch], u.Stream, stream)
		}
		if u.Reason != "conflicts with main" {
			t.Errorf("pull request %s: reason = %q, want %q", ids[branch], u.Reason, "conflicts with main")
		}
		if _, ok := lineNaming(lines, branch, stream, "conflicts with main"); !ok {
			t.Errorf("no planner line names %s, its stream %s and that it conflicts with main; lines: %q", branch, stream, lines)
		}
	}
}

// (3) A pull request whose head has a failed check is listed, and the reason
// names the checks that failed.
func TestUnmergeablePRsListAFailedCheckByName(t *testing.T) {
	const branch = "karakuri/phase-40-step-06a-unmergeable-prs"
	pr := openPR(420, branch, "sha420", time.Hour)
	pr.CheckState = "failure"
	pr.FailingChecks = []string{"go test", "lint"}
	forge := &unmergeableForge{
		branchForge: branchForge{vcStub: vcStub{prs: []versioncontrol.PRSummary{pr}}},
		checks: map[string][]versioncontrol.CheckRun{"sha420": {
			{ID: 1, Name: "go test", HeadSHA: "sha420", Status: "completed", Conclusion: "failure"},
			{ID: 2, Name: "lint", HeadSHA: "sha420", Status: "completed", Conclusion: "failure"},
		}},
	}

	state := observeUnmergeable(forge)

	got := unmergeablePRs(t, state)
	u, ok := findUnmergeable(got, "420")
	if !ok || len(got) != 1 {
		t.Fatalf("unmergeable_prs = %+v, want only pull request 420", got)
	}
	if u.Stream != roadmapDeliveryTemplateID {
		t.Errorf("stream = %q, want %q", u.Stream, roadmapDeliveryTemplateID)
	}
	if !strings.HasPrefix(u.Reason, "check failed: ") {
		t.Errorf("reason = %q, want it to start with %q", u.Reason, "check failed: ")
	}
	for _, name := range pr.FailingChecks {
		if !strings.Contains(u.Reason, name) {
			t.Errorf("reason = %q does not name the failed check %q", u.Reason, name)
		}
	}
	if l, ok := lineNaming(unmergeableLines(t, state), branch, "check failed"); !ok {
		t.Errorf("no planner line names %s and its failed check", branch)
	} else if !strings.Contains(l, "go test") || !strings.Contains(l, "lint") {
		t.Errorf("planner line does not name the failed checks: %q", l)
	}
}

// (4) A pull request with no check at its head is listed once noChecksAfter
// has passed since its last update, not before: checks take a while to start.
func TestUnmergeablePRsListMissingChecksOnlyAfterTheLimit(t *testing.T) {
	const (
		late  = "karakuri/tech-backlog-2026-11-15"
		early = "karakuri/discovery-2026-11-16"
	)
	latePR := openPR(430, late, "sha430", noChecksAfter+5*time.Minute)
	earlyPR := openPR(431, early, "sha431", noChecksAfter-5*time.Minute)
	latePR.CheckState, earlyPR.CheckState = "", ""
	forge := &unmergeableForge{
		branchForge: branchForge{vcStub: vcStub{prs: []versioncontrol.PRSummary{latePR, earlyPR}}},
		checks:      map[string][]versioncontrol.CheckRun{},
	}

	state := observeUnmergeable(forge)

	got := unmergeablePRs(t, state)
	u, ok := findUnmergeable(got, "430")
	if !ok {
		t.Fatalf("pull request 430, with no check %v after its update, is not listed: %+v", noChecksAfter+5*time.Minute, got)
	}
	if u.Reason != "no check reported" {
		t.Errorf("reason = %q, want %q", u.Reason, "no check reported")
	}
	if u.Stream != engineeringBacklogTemplateID {
		t.Errorf("stream = %q, want %q", u.Stream, engineeringBacklogTemplateID)
	}
	if _, ok := findUnmergeable(got, "431"); ok {
		t.Errorf("pull request 431, updated %v ago, is listed before the %v limit: %+v", noChecksAfter-5*time.Minute, noChecksAfter, got)
	}
	lines := unmergeableLines(t, state)
	if _, ok := lineNaming(lines, late, "no check reported"); !ok {
		t.Errorf("no planner line names %s and that no check was reported; lines: %q", late, lines)
	}
	for _, l := range lines {
		if strings.Contains(l, early) {
			t.Errorf("planner line names %s before the limit: %q", early, l)
		}
	}
}

// (5) A pull request from a branch no stream owns is somebody else's work, so
// it is not listed even when it conflicts and its check failed.
func TestUnmergeablePRsLeaveOutBranchesNoStreamOwns(t *testing.T) {
	var prs []versioncontrol.PRSummary
	for i, branch := range []string{"feature/someone-elses-work", "karakuri/salvage/phase-44-step-02", "main"} {
		pr := openPR(440+i, branch, fmt.Sprintf("sha%d", 440+i), 30*24*time.Hour)
		pr.CheckState, pr.FailingChecks = "failure", []string{"go test"}
		prs = append(prs, pr)
	}
	forge := &unmergeableForge{
		branchForge: branchForge{vcStub: vcStub{prs: prs}},
		mergeStates: map[string]string{"440": "conflicting", "441": "conflicting", "442": "conflicting"},
	}

	state := observeUnmergeable(forge)

	if got := unmergeablePRs(t, state); len(got) != 0 {
		t.Fatalf("unmergeable_prs = %+v, want empty: no stream owns these branches", got)
	}
	// main is left out of this check: a line may well mention main.
	for _, l := range unmergeableLines(t, state) {
		for _, branch := range []string{"feature/someone-elses-work", "karakuri/salvage/phase-44-step-02"} {
			if strings.Contains(l, branch) {
				t.Errorf("planner line names %s, which no stream owns: %q", branch, l)
			}
		}
	}
}

// (6) A forge that cannot say whether a pull request can be merged yields a
// line giving its merge state as "not known"; the pull request is neither
// listed as conflicting nor described as clean (AGENTS.md rule 10).
func TestUnmergeablePRsSayMergeStateNotKnownWhenUnsupported(t *testing.T) {
	const branch = "karakuri/ux-2026-11-16"
	forge := &unmergeableForge{
		branchForge: branchForge{vcStub: vcStub{prs: []versioncontrol.PRSummary{openPR(450, branch, "sha450", time.Hour)}}},
		mergeErr:    fmt.Errorf("noop: merge state: %w", versioncontrol.ErrUnsupported),
		checks:      map[string][]versioncontrol.CheckRun{"sha450": greenCheck("sha450")},
	}

	state := observeUnmergeable(forge)

	for _, u := range unmergeablePRs(t, state) {
		if u.prID() == "450" && u.Reason == "conflicts with main" {
			t.Errorf("pull request 450 is listed as conflicting on a forge that cannot say: %+v", u)
		}
	}
	lines := unmergeableLines(t, state)
	l, ok := lineNaming(lines, branch, "not known")
	if !ok {
		t.Fatalf("no planner line names %s and gives its merge state as not known; lines: %q", branch, lines)
	}
	if strings.Contains(strings.ToLower(l), "clean") || strings.Contains(l, "conflicts with main") {
		t.Errorf("the not-known line calls the pull request clean or conflicting: %q", l)
	}
}

// (7) When the open pull requests cannot be listed, the observation does not
// say "none are broken": it carries the error, has no list, and tells the
// planner it could not see them (AGENTS.md rule 10).
func TestUnmergeablePRsSayTheyCouldNotSeeWhenListPRsFails(t *testing.T) {
	unreachable := errors.New("github: dial tcp: connection refused")
	forge := &unmergeableForge{branchForge: branchForge{prsErr: unreachable}}

	state := observeUnmergeable(forge)

	if raw, ok := state["unmergeable_prs"]; ok {
		t.Errorf("unmergeable_prs = %v on a forge that could not be read; want no list", raw)
	}
	msg, _ := state["unmergeable_prs_error"].(string)
	if msg == "" {
		t.Fatalf("unmergeable_prs_error missing; state keys: %v", stateKeys(state))
	}
	if !strings.Contains(msg, unreachable.Error()) {
		t.Errorf("unmergeable_prs_error = %q, want it to carry %q", msg, unreachable.Error())
	}
	lines := unmergeableLines(t, state)
	saidSo := false
	for _, l := range lines {
		lower := strings.ToLower(l)
		if strings.Contains(lower, "could not") || strings.Contains(lower, "not seen") {
			saidSo = true
		}
	}
	if !saidSo {
		t.Errorf("no planner line says the pull requests could not be seen; lines: %q", lines)
	}
}

// Every stream's template carries the rule that repairs its unmergeable pull
// requests, placed before the rule that opens its branches without a pull
// request (step 6), so the planner reads them in the order the roadmap gives:
// broken pull requests first, then branches with none.
func TestStreamTemplatesRepairUnmergeablePRsFirst(t *testing.T) {
	for _, id := range []string{marketDiscoveryTemplateID, engineeringBacklogTemplateID, uxImprovementTemplateID, roadmapDeliveryTemplateID} {
		t.Run(id, func(t *testing.T) {
			unmergeable, branches := -1, -1
			var rule string
			for i, c := range streamTemplate(t, id).Constraints {
				if c.ID == branchesWithoutPRConstraint.ID {
					branches = i
				}
				if unmergeable < 0 && strings.Contains(c.Description, "unmergeable_prs") {
					unmergeable, rule = i, c.Description
				}
			}
			if unmergeable < 0 {
				t.Fatalf("no constraint of %s names unmergeable_prs: the planner is never told to repair the stream's pull requests that cannot be merged", id)
			}
			if branches < 0 {
				t.Fatalf("%s has no %s constraint", id, branchesWithoutPRConstraint.ID)
			}
			if unmergeable > branches {
				t.Errorf("the unmergeable_prs rule is constraint %d of %s, after %s at %d; repairing comes first", unmergeable, id, branchesWithoutPRConstraint.ID, branches)
			}
			for _, want := range []struct{ phrase, why string }{
				{"first", "that repairing them comes first"},
				{"merge main into the branch", "that a conflict is repaired by merging main into the branch"},
				{"keeping both sides", "that the merge keeps both sides"},
				{"never rebase", "that the branch is never rebased"},
				{"force-push", "that the branch is never force-pushed"},
				{"rerun the tests of what the merge touched", "that the tests of what the merge touched are run again"},
				{"cannot keep both sides", "that a conflict that cannot keep both sides is a blocker"},
				{"failed check", "that a failed check is a blocker"},
				{"checks still missing after a clean merge", "that checks still missing after a clean merge are a blocker"},
				{"blocker, not a guess", "that those are reported as blockers, not guessed at"},
			} {
				if !strings.Contains(rule, want.phrase) {
					t.Errorf("the unmergeable_prs rule does not say %s (missing %q):\n%s", want.why, want.phrase, rule)
				}
			}
		})
	}
}
