package software

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// deliveredPRs is what the forge lists in the Phase 44 step 3 fixture: a
// delivery pull request whose checklist ticks steps 4 and 5, one whose body
// has no task list, and one whose items carry no step that can be read.
func deliveredPRs() []versioncontrol.PRSummary {
	return []versioncontrol.PRSummary{
		{ID: "41", Number: 41, Title: "feat(software): x (phase 44 steps 4 to 6)",
			Body: "Summary of the group.\n\n" +
				"- [x] Step 4 first (abc1234)\n" +
				"- [X] Step 5 second, abcdef1\r\n" +
				"- [ ] Step 6 third\n\n" +
				"Opened by Karakuri.",
			Author: "stranger"},
		{ID: "42", Number: 42, Title: "fix(cli): no list (phase 44 step 2)",
			Body: "A plain description with no task list at all.", Author: "stranger"},
		{ID: "43", Number: 43, Title: "chore(web): tidy (phase 7 step 3)",
			Body: "- [x] 3. numbered item\n" +
				"- [ ] Steps 8 and 9 together\n" +
				"- [ ] Not about a step",
			Author: "stranger"},
		{ID: "44", Number: 44, Title: "chore: no phase in this title",
			Body: "- [x] Step 2 done", Author: "stranger"},
	}
}

// observedPRs observes the git environment over prs and reads its pull
// request entries back the way the planner and the world-state record see
// them, as JSON, so the test holds the shape rather than the Go type that
// carries it.
func observedPRs(t *testing.T, prs []versioncontrol.PRSummary) (environment.Observation, []map[string]any) {
	t.Helper()
	env := &gitEnv{id: EnvGit, vc: &vcStub{prs: prs}}
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	raw, err := json.Marshal(obs.State["prs"])
	if err != nil {
		t.Fatalf("marshal prs: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("the observation's prs are not a list of objects: %v (%s)", err, raw)
	}
	if len(entries) != len(prs) {
		t.Fatalf("observation carries %d pull requests, want %d", len(entries), len(prs))
	}
	return obs, entries
}

// wantItem is one checklist item as the observation should carry it.
type wantItem struct {
	text   string
	ticked bool
	shas   []string
	step   string
}

// checklistOf reads one pull request entry's checklist back as wantItems,
// failing when the entry has none.
func checklistOf(t *testing.T, entry map[string]any) []wantItem {
	t.Helper()
	list, ok := entry["checklist"].([]any)
	if !ok {
		t.Fatalf("pull request entry has no checklist list: %#v", entry["checklist"])
	}
	got := make([]wantItem, 0, len(list))
	for i, v := range list {
		item, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("checklist item %d is not an object: %#v", i, v)
		}
		text, _ := item["text"].(string)
		ticked, _ := item["ticked"].(bool)
		step, _ := item["step"].(string)
		var shas []string
		if raw, ok := item["shas"].([]any); ok {
			for _, s := range raw {
				sha, _ := s.(string)
				shas = append(shas, sha)
			}
		}
		got = append(got, wantItem{text: text, ticked: ticked, shas: shas, step: step})
	}
	return got
}

// Every open pull request whose body has a task list carries it in the git
// observation: each item's text, whether it is ticked, the SHAs it names and
// the step it is about, read from the title's phase and the item's step.
func TestGitObservationCarriesEachOpenPullRequestsChecklist(t *testing.T) {
	_, entries := observedPRs(t, deliveredPRs())

	want := []wantItem{
		{text: "Step 4 first (abc1234)", ticked: true, shas: []string{"abc1234"}, step: "phase 44 step 4"},
		{text: "Step 5 second, abcdef1", ticked: true, shas: []string{"abcdef1"}, step: "phase 44 step 5"},
		{text: "Step 6 third", ticked: false, step: "phase 44 step 6"},
	}
	if got := checklistOf(t, entries[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("checklist of pull request 41 =\n  %#v\nwant\n  %#v", got, want)
	}

	// 'MM.' and 'Steps MM' name a step as well as 'Step MM' does; an item
	// that starts with neither is about no step that can be read.
	want = []wantItem{
		{text: "3. numbered item", ticked: true, step: "phase 7 step 3"},
		{text: "Steps 8 and 9 together", ticked: false, step: "phase 7 step 8"},
		{text: "Not about a step", ticked: false},
	}
	if got := checklistOf(t, entries[2]); !reflect.DeepEqual(got, want) {
		t.Errorf("checklist of pull request 43 =\n  %#v\nwant\n  %#v", got, want)
	}

	// A title with no '(phase NN step ...)' suffix gives no phase, so no item
	// of its checklist names a step, even one that starts 'Step 2'.
	want = []wantItem{{text: "Step 2 done", ticked: true}}
	if got := checklistOf(t, entries[3]); !reflect.DeepEqual(got, want) {
		t.Errorf("checklist of pull request 44 =\n  %#v\nwant\n  %#v", got, want)
	}
}

// A pull request whose body has no task list gets no checklist field at all,
// not an empty or null one: "no checklist" and "nothing ticked" differ.
func TestGitObservationGivesNoChecklistToAPullRequestWithoutATaskList(t *testing.T) {
	_, entries := observedPRs(t, deliveredPRs())
	if v, ok := entries[1]["checklist"]; ok {
		t.Errorf("pull request 42 has no task list but carries checklist %#v", v)
	}
}

// The planner reads the checklist as text, one line per item: the box, the
// step when it can be read, the item's text and the SHAs it names.
func TestGitObservationWritesTheChecklistAsLinesForThePlanner(t *testing.T) {
	obs, _ := observedPRs(t, deliveredPRs())

	raw, ok := obs.State["checklist_lines"].([]string)
	if !ok {
		t.Fatalf("observation has no checklist_lines []string: %#v", obs.State["checklist_lines"])
	}
	for _, tc := range []struct {
		prefix, text, suffix string
	}{
		{"[x] phase 44 step 4: ", "Step 4 first (abc1234)", "(abc1234)"},
		{"[x] phase 44 step 5: ", "Step 5 second, abcdef1", "(abcdef1)"},
		{"[ ] phase 44 step 6: ", "Step 6 third", "Step 6 third"},
		{"[x] phase 7 step 3: ", "3. numbered item", "3. numbered item"},
		{"[ ] phase 7 step 8: ", "Steps 8 and 9 together", "Steps 8 and 9 together"},
		// An item with no readable step is still a line, without a step.
		{"[ ] ", "Not about a step", "Not about a step"},
		{"[x] ", "Step 2 done", "Step 2 done"},
	} {
		found := false
		for _, line := range raw {
			if strings.HasPrefix(line, tc.prefix) && strings.Contains(line, tc.text) &&
				strings.HasSuffix(line, tc.suffix) && !strings.Contains(strings.TrimPrefix(line, tc.prefix), "phase ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no line %q ... %q ... %q in %q", tc.prefix, tc.text, tc.suffix, raw)
		}
	}
	if len(raw) != 7 {
		t.Errorf("checklist_lines has %d lines, want one per item (7): %q", len(raw), raw)
	}
}

// Carrying the checklist does not change who the observation says wrote it:
// a body is the writing of whoever opened the pull request (AGENTS.md rule 9).
func TestGitObservationChecklistKeepsThePullRequestsProvenance(t *testing.T) {
	obs, _ := observedPRs(t, deliveredPRs())
	if !obs.Trust.IsThirdParty() {
		t.Errorf("an observation carrying a stranger's checklist declared itself %v", obs.Trust)
	}

	own := deliveredPRs()
	for i := range own {
		own[i].OwnAuthor = true
	}
	obs, _ = observedPRs(t, own)
	if obs.Trust.IsThirdParty() {
		t.Errorf("an observation carrying only the deployment's own checklists declared itself third party")
	}
}
