package objective

import (
	"context"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 7 part A: samples, reducer, k and min_samples are checked at
// create time, beside the rest of step 5's validation, and each refusal names
// the field (docs/roadmap.md, Phase 41 decision 3 'Repeated runs').

// sampledLatency is a valid measured criterion over five samples.
func sampledLatency(edit func(*coreobjective.Criterion)) coreobjective.Criterion {
	c := coreobjective.Criterion{
		ID: "latency-sampled", Description: "p95 latency holds over five runs", Kind: coreobjective.KindMeasured,
		Verifier: latencyVerifier, Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: target(200),
		Unit: "ms", Samples: 5, Reducer: coreobjective.ReducerMedian, MinSamples: 3,
	}
	edit(&c)
	return c
}

// A sampled criterion that says what the loop can settle is accepted and
// stored as written.
func TestCreateAcceptsASampledCriterion(t *testing.T) {
	for name, criterion := range map[string]coreobjective.Criterion{
		"median, min_samples 3 of 5": sampledLatency(func(*coreobjective.Criterion) {}),
		"k_of_n, k 1":                sampledLatency(func(c *coreobjective.Criterion) { c.Reducer, c.K = coreobjective.ReducerKOfN, 1 }),
		"k_of_n, k = samples":        sampledLatency(func(c *coreobjective.Criterion) { c.Reducer, c.K = coreobjective.ReducerKOfN, 5 }),
		"min_samples = samples":      sampledLatency(func(c *coreobjective.Criterion) { c.MinSamples = 5 }),
		"all":                        sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerAll }),
		"p95":                        sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerP95 }),
		"min":                        sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerMin }),
		"max":                        sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerMax }),
		"mean":                       sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerMean }),
		"no reducer":                 sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = "" }),
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			o, err := svc.Create(context.Background(), CreateRequest{
				Title: "plain", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{criterion},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := svc.Get(context.Background(), o.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			c := criterionByID(t, got.SuccessCriteria, criterion.ID)
			if c.Samples != criterion.Samples || c.Reducer != criterion.Reducer || c.K != criterion.K || c.MinSamples != criterion.MinSamples {
				t.Errorf("stored samples=%d reducer=%q k=%d min_samples=%d, want %d %q %d %d",
					c.Samples, c.Reducer, c.K, c.MinSamples, criterion.Samples, criterion.Reducer, criterion.K, criterion.MinSamples)
			}
		})
	}
}

// A sampled criterion the loop could never settle the way its author meant is
// refused, naming the field.
func TestCreateRejectsAnInvalidSampledCriterion(t *testing.T) {
	for name, tc := range map[string]struct {
		criterion coreobjective.Criterion
		field     string
	}{
		// There is never an "any rerun passes" reducer: rerunning until green
		// hides real bugs (docs/roadmap.md, Phase 41 decision 3).
		"reducer any": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = "any" }),
			"success_criteria[1].reducer",
		},
		"unknown reducer": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = "p99" }),
			"success_criteria[1].reducer",
		},
		"k_of_n without k": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Reducer = coreobjective.ReducerKOfN }),
			"success_criteria[1].k",
		},
		"k_of_n with a negative k": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Reducer, c.K = coreobjective.ReducerKOfN, -1 }),
			"success_criteria[1].k",
		},
		"k_of_n with k above samples": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Reducer, c.K = coreobjective.ReducerKOfN, 6 }),
			"success_criteria[1].k",
		},
		"min_samples above samples": {
			sampledLatency(func(c *coreobjective.Criterion) { c.MinSamples = 6 }),
			"success_criteria[1].min_samples",
		},
		"negative samples": {
			sampledLatency(func(c *coreobjective.Criterion) { c.Samples, c.MinSamples = -1, 0 }),
			"success_criteria[1].samples",
		},
		// Only a measured criterion has samples to reduce.
		"reducer on a verified criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: testsVerifier, Reducer: coreobjective.ReducerAll},
			"success_criteria[1].reducer",
		},
		"samples on a verified criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: testsVerifier, Samples: 3},
			"success_criteria[1].samples",
		},
		"min_samples on a verified criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindVerified, Verifier: testsVerifier, MinSamples: 3},
			"success_criteria[1].min_samples",
		},
		"reducer on a judged criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindJudged, Evidence: "the PR", Reducer: coreobjective.ReducerMedian},
			"success_criteria[1].reducer",
		},
		"samples on a judged criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindJudged, Evidence: "the PR", Samples: 3},
			"success_criteria[1].samples",
		},
		"min_samples on a judged criterion": {
			coreobjective.Criterion{ID: "x", Description: "x", Kind: coreobjective.KindJudged, Evidence: "the PR", MinSamples: 3},
			"success_criteria[1].min_samples",
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newCriteriaService(t)
			// A valid criterion first, so the index names the criterion's own
			// position.
			assertRejected(t, svc, CreateRequest{
				Title: "plain", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{
					{ID: "tested", Description: "the tests pass", Kind: coreobjective.KindVerified, Verifier: testsVerifier},
					tc.criterion,
				},
			}, tc.field)
		})
	}
}
