package software

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13, discovery half: the reading that says whether delivery has
// work also says whether a discovery cycle starts. No model, a fake clock.

var discoveryNow = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

func discoveryPR(n int) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{ID: "5", Number: n, Title: "docs(research): market report", HeadBranch: "karakuri/discovery-2026-03-01", OwnAuthor: true}
}

func deliveryPR(title string) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{ID: "9", Number: 9, Title: title, HeadBranch: "karakuri/phase-40-step-1-x", OwnAuthor: true}
}

func TestOpenDiscoveryProposalsAreFoundByBranchPattern(t *testing.T) {
	prs := []versioncontrol.PRSummary{discoveryPR(5), deliveryPR("feat(x): y (phase 40 step 1)")}
	got := OpenDiscoveryProposals(prs)
	if len(got) != 1 || got[0].Number != 5 {
		t.Fatalf("got %+v, want only the pull request on karakuri/discovery-*", got)
	}
	if got := OpenDiscoveryProposals(prs[1:]); len(got) != 0 {
		t.Errorf("a delivery branch counted as a discovery proposal: %+v", got)
	}
}

func TestComputeDiscoveryTrigger(t *testing.T) {
	never := time.Time{}
	cases := []struct {
		name    string
		roadmap string
		prs     []versioncontrol.PRSummary
		last    time.Time
		want    bool
		reason  string
	}{
		{name: "nothing planned, no proposal, no earlier cycle", roadmap: roadmapNothingPlanned, last: never, want: true},
		{name: "one open proposal", roadmap: roadmapNothingPlanned, prs: []versioncontrol.PRSummary{discoveryPR(5)}, last: never, want: false, reason: "open"},
		{name: "delivered item waiting for merge, nothing else planned",
			roadmap: roadmapPlannedPhase, prs: []versioncontrol.PRSummary{deliveryPR("feat(x): y (phase 40 step 13)")}, last: never, want: true},
		{name: "planned work remains", roadmap: roadmapPlannedPhase, last: never, want: false, reason: "40"},
		{name: "closed proposal, inside the minimum time", roadmap: roadmapNothingPlanned, last: discoveryNow.Add(-DiscoveryMinGap + time.Minute), want: false, reason: "minimum"},
		{name: "closed proposal, after the minimum time", roadmap: roadmapNothingPlanned, last: discoveryNow.Add(-DiscoveryMinGap), want: true},
		{name: "open proposal outweighs an elapsed minimum time", roadmap: roadmapNothingPlanned,
			prs: []versioncontrol.PRSummary{discoveryPR(5)}, last: discoveryNow.Add(-48 * time.Hour), want: false, reason: "open"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work, err := ComputeDeliveryWork(tc.roadmap, nil, tc.prs, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := ComputeDiscoveryTrigger(work, tc.prs, tc.last, discoveryNow, DiscoveryMinGap)
			if got.Start != tc.want {
				t.Errorf("Start = %v, want %v (reasons %v)", got.Start, tc.want, got.Reasons)
			}
			if len(got.Reasons) == 0 {
				t.Errorf("a verdict must say why")
			}
			if tc.reason != "" && !strings.Contains(strings.ToLower(strings.Join(got.Reasons, "|")), tc.reason) {
				t.Errorf("reasons %v do not mention %q", got.Reasons, tc.reason)
			}
		})
	}
}

// After a merge that adds Planned work, delivery starts and the empty trigger is silent.
func TestAMergeThatAddsPlannedWorkStartsDeliveryAndSilencesDiscovery(t *testing.T) {
	work, err := ComputeDeliveryWork(roadmapPlannedPhase, nil, nil, nil) // the proposal is merged: no open pull request
	if err != nil {
		t.Fatal(err)
	}
	if !work.HasWork {
		t.Fatal("delivery has no work although main gained a Planned phase")
	}
	if got := ComputeDiscoveryTrigger(work, nil, discoveryNow.Add(-72*time.Hour), discoveryNow, DiscoveryMinGap); got.Start {
		t.Errorf("the empty-pipeline trigger fired with Planned work on main: %v", got.Reasons)
	}
}

func discoverySources(main *mainFile, last *time.Time, comment *string, now *time.Time) DiscoveryTriggerSources {
	return DiscoveryTriggerSources{
		ReadMain:       main.read,
		LastCycleEnd:   func(context.Context) (time.Time, error) { return *last, nil },
		ClosingComment: func(context.Context) (string, error) { return *comment, nil },
		MinGap:         DiscoveryMinGap,
		Now:            func() time.Time { return *now },
	}
}

func TestDiscoveryTriggerEnvironmentReadsTheVerdictWithoutAModel(t *testing.T) {
	main := &mainFile{text: roadmapNothingPlanned}
	forge := &branchForge{}
	last, comment, now := time.Time{}, "", discoveryNow
	env := NewDiscoveryTriggerEnvironment("software.env.discovery_trigger", forge, discoverySources(main, &last, &comment, &now))

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if v, _ := obs.State["has_work"].(bool); !v {
		t.Errorf("has_work = %v with nothing Planned and no proposal, want true: %v", obs.State["has_work"], obs.State)
	}
	empty := snapshotOf(t, env)
	if empty == "" {
		t.Fatal("empty SHA although main and the pull requests were readable")
	}

	forge.prs = []versioncontrol.PRSummary{discoveryPR(5)}
	open := snapshotOf(t, env)
	if open == "" || open == empty {
		t.Errorf("snapshot did not move when a proposal opened (%q -> %q)", empty, open)
	}
	snap, _ := env.Snapshot(context.Background())
	if v, ok := snap.State["has_work"].(bool); !ok || v {
		t.Errorf("snapshot state has_work = %v with a proposal open, want false (the supervisor re-baselines on it)", snap.State["has_work"])
	}
}

// The closing comment is in what the pass reads, as text somebody outside the
// deployment wrote (AGENTS.md rule 9); with no comment the observation is not third-party.
func TestDiscoveryTriggerObservationCarriesTheClosingCommentAsThirdParty(t *testing.T) {
	main := &mainFile{text: roadmapNothingPlanned}
	last, comment, now := discoveryNow.Add(-48*time.Hour), "Closing: too broad, narrow it to the CLI", discoveryNow
	env := NewDiscoveryTriggerEnvironment("software.env.discovery_trigger", &branchForge{}, discoverySources(main, &last, &comment, &now))

	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !strings.Contains(fmt.Sprint(obs.State), "too broad") {
		t.Errorf("the closing comment is not in what the pass reads: %v", obs.State)
	}
	if obs.Trust != environment.TrustThirdParty {
		t.Errorf("Trust = %q, want TrustThirdParty: the comment is text from outside the deployment", obs.Trust)
	}

	comment = ""
	obs, _ = env.Observe(context.Background(), environment.ObservationQuery{})
	if obs.Trust == environment.TrustThirdParty {
		t.Error("an observation with no closing comment is marked third-party; provenance is set from what the payload holds")
	}
}

// Main or the pull requests unreadable: no cycle starts and the reading is blind (rule 10).
func TestDiscoveryTriggerIsBlindWhenMainOrThePullRequestsCannotBeRead(t *testing.T) {
	last, comment, now := time.Time{}, "", discoveryNow
	cases := map[string]struct {
		main  *mainFile
		forge *branchForge
	}{
		"main unreadable":          {&mainFile{err: errors.New("401 bad credentials")}, &branchForge{}},
		"pull requests unreadable": {&mainFile{text: roadmapNothingPlanned}, &branchForge{prsErr: errors.New("502 from the forge")}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := NewDiscoveryTriggerEnvironment("software.env.discovery_trigger", tc.forge, discoverySources(tc.main, &last, &comment, &now))
			if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err == nil {
				t.Error("Observe returned no error; blind must say so, never look like an empty pipeline")
			}
			if sha := snapshotOf(t, env); sha != "" {
				t.Errorf("SHA = %q while blind, want empty (AGENTS.md rule 10)", sha)
			}
		})
	}
}
