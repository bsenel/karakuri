package software

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
)

// CapResearch is declared in Phase 2 as "research a topic across configured
// sources" and the ResearchAdapter it needs was built in Phase 6. Nothing
// connected them: the capability sat on the strategist's and the architect's
// lists, was planned, and reached noopEnv.
//
// Phase 25 step 4 asked for a research environment in the self-improvement
// pack. There was no need for a second one — the capability and the adapter
// were both already here, unintroduced.
const CapResearch = "software.reason.research"

// researchEnv answers "what is the field doing about this", ranked.
//
// Pre-ranked for the same reason the telemetry environment pre-ranks
// bottlenecks and the codebase environment pre-ranks TODO density: a model
// asked to order findings itself will order them slightly differently on every
// run, and a proposal's evidence should not depend on which ordering it drew.
type researchEnv struct {
	id       environment.EnvironmentID
	adapter  research.ResearchAdapter
	maxDepth string
}

func newResearchEnv(id environment.EnvironmentID, adapter research.ResearchAdapter) *researchEnv {
	return &researchEnv{id: id, adapter: adapter, maxDepth: "standard"}
}

func (e *researchEnv) ID() environment.EnvironmentID { return e.id }
func (e *researchEnv) Domain() string                { return "software" }

func (e *researchEnv) Observe(_ context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	// Research has no ambient state to observe: there is no "current value" of
	// the field, only answers to questions somebody asked. Reporting whether
	// the adapter is wired is the honest observation, and it is what a planner
	// needs to decide whether to plan a research action at all.
	state := map[string]any{
		"available": e.adapter != nil && e.adapter.Active(),
	}
	if !state["available"].(bool) {
		state["reason"] = "no research adapter is wired into this deployment"
	}
	return environment.Observation{
		EnvID: e.id, State: state, Version: stateVersion(state), Timestamp: time.Now().UTC(),
	}, nil
}

func (e *researchEnv) Act(ctx context.Context, a environment.Action) (environment.ActionResult, error) {
	if a.CapabilityID != CapResearch {
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("%s researches; %s cannot be executed here", e.id, a.CapabilityID),
		}, nil
	}
	if e.adapter == nil || !e.adapter.Active() {
		return noopAct(a), nil
	}

	topic := asString(a.Params, "topic")
	if topic == "" {
		// A capability that succeeds on empty input feeds a perfect success
		// rate into procedural memory, biasing the next plan's confidence up
		// for having produced nothing.
		return environment.ActionResult{
			Success: false,
			Error:   fmt.Sprintf("%s needs a topic: pass params.topic", CapResearch),
		}, nil
	}

	var sources []string
	if raw, ok := a.Params["sources"].([]any); ok {
		for _, s := range raw {
			if str, ok := s.(string); ok && str != "" {
				sources = append(sources, str)
			}
		}
	}
	depth := asString(a.Params, "depth")
	if depth == "" {
		depth = e.maxDepth
	}

	findings, err := e.adapter.Search(ctx, topic, sources, depth)
	if err != nil {
		return environment.ActionResult{Success: false, Error: err.Error()}, nil
	}

	// Confidence descending, then title, so the order is total and stable
	// rather than whatever the adapter happened to return.
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Confidence != findings[j].Confidence {
			return findings[i].Confidence > findings[j].Confidence
		}
		return findings[i].Title < findings[j].Title
	})

	ranked := make([]map[string]any, 0, len(findings))
	for _, f := range findings {
		ranked = append(ranked, map[string]any{
			"title": f.Title, "summary": f.Summary,
			"confidence": f.Confidence, "source": f.Source,
		})
	}

	// An empty result is a real answer — "nobody has written about this" is
	// worth knowing — but it is not evidence, and saying so keeps a proposal
	// from citing a search that found nothing.
	level := EvidenceNone
	if len(ranked) >= minPattern {
		level = EvidenceAdequate
	} else if len(ranked) > 0 {
		level = EvidenceThin
	}

	return environment.ActionResult{Success: true, StateDelta: map[string]any{
		"capability": CapResearch,
		"topic":      topic,
		"findings":   ranked,
		"evidence":   level,
	}}, nil
}

func (e *researchEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

func (e *researchEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, _ := e.Observe(ctx, environment.ObservationQuery{})
	// Deliberately no SHA. Research has no state that drifts, and an
	// environment returning an empty SHA contributes nothing to the composite
	// fingerprint — which is correct: a standing objective must not reconcile
	// because a search engine's results moved.
	return environment.EnvironmentSnapshot{
		EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp,
	}, nil
}
