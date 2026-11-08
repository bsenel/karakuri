package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	coretelemetry "github.com/bsenel/karakuri/internal/core/telemetry"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 step 13, end to end: a required measured criterion
// cost_per_pass <= 1.00 USD is accepted at create against the software pack's
// own declaration, measured by the platform telemetry environment the pack's
// factory builds over a telemetry reader, and settled by code from that
// measurement (docs/roadmap.md, Phase 41 step 13; ADR 029).
//
// The settlement below is the loop's own rule for a measured criterion with
// no reducer and no sample count (settleByMeasurement and qualifyingSamples in
// internal/feature/loop/verify.go:245-286): only first-party measurements of
// the criterion's metric in its unit are samples, every one has to meet the
// target (ReducerAll), and no sample is inconclusive: no_data.

const costVerifier = capability.CapabilityID(domainsw.CapCostPerPass)

func costCriterion(unit string) coreobjective.Criterion {
	target := 1.0
	return coreobjective.Criterion{
		ID: "cost-per-pass", Description: "a reconcile pass costs at most one dollar",
		Kind: coreobjective.KindMeasured, Verifier: costVerifier,
		Metric: "cost_per_pass", Op: coreobjective.OpLessEqual, Target: &target, Unit: unit,
		Params: map[string]any{"window": "24h"},
	}
}

// telemetryPackVerifiers is the capability registry create checks criteria
// against, holding the software pack's capabilities as bootstrap registers them.
func telemetryPackVerifiers(t *testing.T) *capability.Registry {
	t.Helper()
	reg := capability.NewRegistry()
	for _, c := range domainsw.New().Capabilities() {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	return reg
}

func telemetryObjectiveService(t *testing.T) *featureobjective.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return featureobjective.NewService(storage.NewGORMStorage(db)).WithVerifiers(telemetryPackVerifiers(t))
}

// snapshotReader is a telemetry reader that returns one snapshot.
type snapshotReader struct{ snap coretelemetry.Snapshot }

func (r snapshotReader) Snapshot(context.Context, coretelemetry.Query) (coretelemetry.Snapshot, error) {
	return r.snap, nil
}

// telemetryEnvFor is the software pack's platform telemetry environment, built
// by its factory for a twin with reader wired, as bootstrap wires it.
func telemetryEnvFor(t *testing.T, reader coretelemetry.Reader) environment.Environment {
	t.Helper()
	for _, f := range domainsw.New().EnvironmentFactories() {
		if f.EnvID != domainsw.EnvPlatformTelemetry {
			continue
		}
		env, err := f.Build(environment.BuildContext{TwinID: "twin-maintainer", Telemetry: reader})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}
	t.Fatal("the software pack builds no platform telemetry environment")
	return nil
}

// settleMeasured applies the loop's settlement of a required measured
// criterion to one verifier result (internal/feature/loop/verify.go:245-286).
func settleMeasured(c coreobjective.Criterion, res environment.ActionResult) (coreobjective.CriterionState, coreobjective.InconclusiveReason) {
	var samples []float64
	units := c.Unit
	for _, m := range res.Measurements {
		if m.Name != c.Metric || (m.Trust.IsThirdParty() && c.IsRequired()) || m.Unit != c.Unit {
			continue
		}
		samples = append(samples, m.Value)
	}
	if len(samples) == 0 {
		return coreobjective.CriterionInconclusive, coreobjective.ReasonNoData
	}
	c.Reducer = coreobjective.ReducerAll
	return c.CompareSamples(samples, units)
}

// Create accepts cost_per_pass <= 1.00 USD over a 24h window against the
// pack's declaration, and refuses the same criterion in another unit.
func TestCreateChecksACostPerPassCriterionAgainstThePack(t *testing.T) {
	svc := telemetryObjectiveService(t)
	req := func(c coreobjective.Criterion) featureobjective.CreateRequest {
		return featureobjective.CreateRequest{Title: "passes stay cheap", Domain: "software",
			SuccessCriteria: []coreobjective.Criterion{c}}
	}

	obj, err := svc.Create(context.Background(), req(costCriterion("usd")))
	if err != nil {
		t.Fatalf("cost_per_pass <= 1.00 usd was refused: %v", err)
	}
	if len(obj.SuccessCriteria) != 1 || obj.SuccessCriteria[0].Verifier != costVerifier {
		t.Errorf("criteria = %+v, want the cost criterion", obj.SuccessCriteria)
	}

	_, err = svc.Create(context.Background(), req(costCriterion("ratio")))
	var verr *featureobjective.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("cost_per_pass <= 1.00 ratio: err = %v, want a *ValidationError", err)
	}
	if verr.Field != "success_criteria[0].unit" {
		t.Errorf("refused field = %q, want success_criteria[0].unit", verr.Field)
	}
}

// The criterion settles met from 3.00 USD over four passes, not_met from 6.00
// USD over four, and inconclusive no_data when no pass ran or spend was not
// priced: a window with nothing to divide never reads as a cheap one.
func TestACostPerPassCriterionSettlesFromTelemetry(t *testing.T) {
	snap := func(cost float64, priced bool, passes int) coretelemetry.Snapshot {
		return coretelemetry.Snapshot{
			Work:  coretelemetry.WorkStats{Reconciles: passes},
			Spend: coretelemetry.SpendStats{Cost: cost, Priced: priced},
		}
	}
	for _, tc := range []struct {
		name       string
		snap       coretelemetry.Snapshot
		want       coreobjective.CriterionState
		wantReason coreobjective.InconclusiveReason
	}{
		{"0.75 per pass", snap(3.0, true, 4), coreobjective.CriterionMet, ""},
		{"1.50 per pass", snap(6.0, true, 4), coreobjective.CriterionNotMet, ""},
		{"no pass", snap(3.0, true, 0), coreobjective.CriterionInconclusive, coreobjective.ReasonNoData},
		{"unpriced", snap(0, false, 4), coreobjective.CriterionInconclusive, coreobjective.ReasonNoData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := telemetryObjectiveService(t).Create(context.Background(), featureobjective.CreateRequest{
				Title: "passes stay cheap", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{costCriterion("usd")},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			c := obj.SuccessCriteria[0]
			if !c.IsRequired() {
				t.Fatalf("criterion %s is not required", c.ID)
			}

			res, err := telemetryEnvFor(t, snapshotReader{snap: tc.snap}).Act(context.Background(),
				environment.Action{CapabilityID: c.Verifier, Params: c.Params})
			if err != nil {
				t.Fatalf("act: %v", err)
			}

			state, reason := settleMeasured(c, res)
			if state != tc.want || reason != tc.wantReason {
				t.Errorf("cost_per_pass from %+v settles %q (reason %q), want %q (reason %q); result %s",
					tc.snap, state, reason, tc.want, tc.wantReason, describeResult(res))
			}
			if tc.want == coreobjective.CriterionInconclusive {
				if got := res.StateDelta["reason"]; got != string(coreobjective.ReasonNoData) {
					t.Errorf("verifier reason = %v, want no_data", got)
				}
			}
		})
	}
}

func describeResult(res environment.ActionResult) string {
	return fmt.Sprintf("{success %v, measurements %+v, delta %v, error %q}", res.Success, res.Measurements, res.StateDelta, res.Error)
}
