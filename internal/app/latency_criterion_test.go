package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	featureobjective "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
)

// Phase 41 step 12, end to end: a measured criterion p95_latency_ms <= 300 ms
// is accepted at create against the software pack's own declaration, measured
// by the pack's verifier through a real Prometheus adapter served a recorded
// query_range response, and settled by code from that measurement.

const latencyVerifier = capability.CapabilityID(domainsw.CapP95Latency)

func latencyCriterion(unit string) coreobjective.Criterion {
	target := 300.0
	return coreobjective.Criterion{
		ID: "checkout-latency", Description: "checkout p95 latency stays at or under 300 ms",
		Kind: coreobjective.KindMeasured, Verifier: latencyVerifier,
		Metric: "p95_latency_ms", Op: coreobjective.OpLessEqual, Target: &target, Unit: unit,
		Params: map[string]any{
			"query":         `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_ms_bucket{service="checkout"}[5m])))`,
			"since_minutes": float64(30),
		},
	}
}

// packVerifiers is the capability registry create checks criteria against,
// holding the software pack's capabilities as bootstrap registers them.
func packVerifiers(t *testing.T) *capability.Registry {
	t.Helper()
	reg := capability.NewRegistry()
	for _, c := range domainsw.New().Capabilities() {
		if err := reg.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	return reg
}

func objectiveService(t *testing.T) *featureobjective.Service {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return featureobjective.NewService(storage.NewGORMStorage(db)).WithVerifiers(packVerifiers(t))
}

// (g) Create accepts p95_latency_ms <= 300 ms against the pack's declaration
// and refuses the same criterion in another unit.
func TestCreateChecksALatencyCriterionAgainstThePack(t *testing.T) {
	svc := objectiveService(t)
	req := func(c coreobjective.Criterion) featureobjective.CreateRequest {
		return featureobjective.CreateRequest{Title: "checkout stays fast", Domain: "software",
			SuccessCriteria: []coreobjective.Criterion{c}}
	}

	obj, err := svc.Create(context.Background(), req(latencyCriterion("ms")))
	if err != nil {
		t.Fatalf("p95_latency_ms <= 300 ms was refused: %v", err)
	}
	if len(obj.SuccessCriteria) != 1 || obj.SuccessCriteria[0].Verifier != latencyVerifier {
		t.Errorf("criteria = %+v, want the latency criterion", obj.SuccessCriteria)
	}

	for _, unit := range []string{"s", "ratio"} {
		t.Run(unit, func(t *testing.T) {
			_, err := svc.Create(context.Background(), req(latencyCriterion(unit)))
			var verr *featureobjective.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("p95_latency_ms <= 300 %s: err = %v, want a *ValidationError", unit, err)
			}
			if verr.Field != "success_criteria[0].unit" {
				t.Errorf("refused field = %q, want success_criteria[0].unit", verr.Field)
			}
		})
	}
}

// latencyEnv is the software pack's observability environment, built by its
// factory for a twin bound to a Prometheus instance that serves a recorded
// query_range response whose latest point is latest.
func latencyEnv(t *testing.T, latest float64) environment.Environment {
	t.Helper()
	body := fmt.Sprintf(`{"status":"success","data":{"resultType":"matrix","result":[`+
		`{"metric":{"service":"checkout"},"values":[[1700000000,"275"],[1700000060,"%v"]]}]}}`, latest)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	reg := tools.NewRegistry()
	reg.Observability.Set("prod", "prometheus", observability.NewPrometheus(srv.URL, ""))
	for _, f := range domainsw.NewWithTools(reg).EnvironmentFactories() {
		if f.EnvID != "software.env.observability" {
			continue
		}
		env, err := f.Build(environment.BuildContext{TwinID: "twin-checkout",
			AdapterBindings: map[string]string{"observability": "prod"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}
	t.Fatal("the software pack builds no observability environment")
	return nil
}

// (h) The criterion settles met from a recorded 250 and not_met from 400, by
// comparing the verifier's one first-party measurement with the target.
func TestALatencyCriterionSettlesFromARecordedResponse(t *testing.T) {
	for _, tc := range []struct {
		latest float64
		want   coreobjective.CriterionState
	}{
		{250, coreobjective.CriterionMet},
		{400, coreobjective.CriterionNotMet},
	} {
		t.Run(fmt.Sprint(tc.latest), func(t *testing.T) {
			obj, err := objectiveService(t).Create(context.Background(), featureobjective.CreateRequest{
				Title: "checkout stays fast", Domain: "software",
				SuccessCriteria: []coreobjective.Criterion{latencyCriterion("ms")},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			c := obj.SuccessCriteria[0]

			res, err := latencyEnv(t, tc.latest).Act(context.Background(),
				environment.Action{CapabilityID: c.Verifier, Params: c.Params})
			if err != nil {
				t.Fatalf("act: %v", err)
			}
			if !res.Success || len(res.Measurements) != 1 {
				t.Fatalf("the verifier measured %+v (success %v, error %q), want one measurement",
					res.Measurements, res.Success, res.Error)
			}
			m := res.Measurements[0]
			if m.Name != c.Metric || m.Trust.IsThirdParty() {
				t.Fatalf("measurement %+v cannot settle the required criterion %s", m, c.Metric)
			}

			// The loop's settlement of a measured criterion with no reducer:
			// every first-party value in the criterion's unit has to meet it.
			c.Reducer = coreobjective.ReducerAll
			state, reason := c.CompareSamples([]float64{m.Value}, m.Unit)
			if state != tc.want {
				t.Errorf("p95_latency_ms %v against <= 300 ms is %q (reason %q), want %q", m.Value, state, reason, tc.want)
			}
		})
	}
}
