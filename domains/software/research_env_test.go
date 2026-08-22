package software

import (
	"context"
	"errors"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
)

type stubResearch struct {
	active    bool
	findings  []research.Finding
	err       error
	lastTopic string
}

func (s *stubResearch) Active() bool { return s.active }

func (s *stubResearch) Search(_ context.Context, topic string, _ []string, _ string) ([]research.Finding, error) {
	s.lastTopic = topic
	return s.findings, s.err
}

// Findings arrive ranked, whatever order the adapter returned them in.
//
// Pre-ranked for the same reason the telemetry environment pre-ranks
// bottlenecks: a model asked to order them itself orders them slightly
// differently every run, and a proposal's evidence should not depend on which
// ordering it drew.
func TestResearchFindingsAreRanked(t *testing.T) {
	adapter := &stubResearch{active: true, findings: []research.Finding{
		{Title: "middling", Confidence: 0.5},
		{Title: "strong", Confidence: 0.9},
		{Title: "weak", Confidence: 0.1},
	}}
	env := newResearchEnv("software.env.research", adapter)

	res, err := env.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "reconciliation loops"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("research refused: %s", res.Error)
	}
	if adapter.lastTopic != "reconciliation loops" {
		t.Errorf("adapter was asked about %q", adapter.lastTopic)
	}

	ranked, _ := res.StateDelta["findings"].([]map[string]any)
	if len(ranked) != 3 {
		t.Fatalf("got %d findings, want 3", len(ranked))
	}
	want := []string{"strong", "middling", "weak"}
	for i, w := range want {
		if ranked[i]["title"] != w {
			t.Errorf("finding %d is %v, want %q", i, ranked[i]["title"], w)
		}
	}
	if res.StateDelta["evidence"] != EvidenceAdequate {
		t.Errorf("three findings graded %v, want adequate", res.StateDelta["evidence"])
	}
}

// A search that found nothing is a real answer and is not evidence. Grading it
// otherwise lets a proposal cite a search that returned an empty list.
func TestAnEmptySearchIsNotEvidence(t *testing.T) {
	env := newResearchEnv("software.env.research", &stubResearch{active: true})
	res, _ := env.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "something nobody has written about"},
	})
	if !res.Success {
		t.Fatalf("an empty search was recorded as a failure: %s", res.Error)
	}
	if res.StateDelta["evidence"] != EvidenceNone {
		t.Errorf("an empty search graded %v, want none", res.StateDelta["evidence"])
	}
}

// Empty input is refused rather than succeeding quietly: a capability that
// succeeds on nothing feeds a perfect success rate into procedural memory and
// biases the next plan's confidence up for having produced nothing.
func TestResearchRefusesAnEmptyTopic(t *testing.T) {
	env := newResearchEnv("software.env.research", &stubResearch{active: true})
	res, _ := env.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{},
	})
	if res.Success {
		t.Error("research reported success with no topic")
	}
}

// An unwired adapter degrades the way the rest of the pack does, and an
// adapter error is reported rather than swallowed.
func TestResearchDegradesHonestly(t *testing.T) {
	unwired := newResearchEnv("software.env.research", &stubResearch{active: false})
	res, _ := unwired.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "anything"},
	})
	if res.Success {
		t.Error("an unwired adapter reported success")
	}

	broken := newResearchEnv("software.env.research", &stubResearch{active: true, err: errors.New("upstream down")})
	res, _ = broken.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "anything"},
	})
	if res.Success || res.Error != "upstream down" {
		t.Errorf("an adapter error was not reported: success=%v err=%q", res.Success, res.Error)
	}
}

// Research has no state that drifts. An environment returning an empty SHA
// contributes nothing to the composite fingerprint, which is correct: a
// standing objective must not reconcile because a search engine's results
// moved.
func TestResearchContributesNoDrift(t *testing.T) {
	env := newResearchEnv("software.env.research", &stubResearch{active: true})
	snap, err := env.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.SHA != "" {
		t.Errorf("research reported a drift SHA %q; a moving search result is not deployment drift", snap.SHA)
	}
}
