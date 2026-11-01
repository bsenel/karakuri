package handler_test

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// Phase 41 step 7 part D2: PUT /objectives/{id}/criteria/{criterion_id}/baseline
// with {"value": <number>} is a person setting a measured criterion's baseline.
// 200 with the updated objective; 404 for an unknown objective or criterion;
// 400 naming the field when value is missing or not a finite number, or when
// the criterion is not measured or has no change (docs/roadmap.md, Phase 41
// decision 3 'Baselines' and step 7's acceptance).

const baselinePerson = "alice@example.com"

// baselineServer mounts the handler on the route it will be wired to, behind a
// principal, the way the router's auth middleware puts one on the request. The
// objective has a verified criterion (tested), a measured one with no change
// (coverage) and a relative at_start one not measured yet (latency).
func baselineServer(t *testing.T) (http.Handler, storage.StorageAdapter, objective.Objective) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	verifiers := capability.NewRegistry()
	for _, c := range []capability.Capability{
		{ID: "software.verify.run_tests", Domain: "software", Verifiable: true},
		{ID: "software.verify.coverage", Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "coverage", Unit: "percent", Direction: capability.DirectionHigherIsBetter,
		}}},
		{ID: "software.verify.latency", Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "p95_latency_ms", Unit: "ms", Direction: capability.DirectionLowerIsBetter,
		}}},
	} {
		if err := verifiers.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	svc := featureobj.NewService(store).WithVerifiers(verifiers)
	coverageTarget, latencyTarget := 80.0, 10.0
	obj, err := svc.Create(context.Background(), featureobj.CreateRequest{
		Title: "keep latency where it started", Domain: "software",
		SuccessCriteria: []objective.Criterion{
			{ID: "tested", Description: "the tests pass", Kind: objective.KindVerified, Verifier: "software.verify.run_tests"},
			{
				ID: "coverage", Description: "coverage holds", Kind: objective.KindMeasured, Verifier: "software.verify.coverage",
				Metric: "coverage", Op: objective.OpGreaterEqual, Target: &coverageTarget, Unit: "percent",
			},
			{
				ID: "latency", Description: "p95 latency is not more than 10% above where it started",
				Kind: objective.KindMeasured, Verifier: "software.verify.latency", Metric: "p95_latency_ms",
				Op: objective.OpLessEqual, Target: &latencyTarget, Unit: "ms",
				Baseline: &objective.Baseline{AtStart: true}, Change: objective.ChangePercent,
			},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	h := &handler.ObjectiveHandler{Objectives: svc}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{ID: baselinePerson})))
		})
	})
	r.Put("/api/v1/objectives/{id}/criteria/{criterion_id}/baseline", h.SetBaseline)
	return r, store, obj
}

func putBaseline(t *testing.T, srv http.Handler, objectiveID, criterionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/objectives/"+objectiveID+"/criteria/"+criterionID+"/baseline", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func storedBaselineRows(t *testing.T, store storage.StorageAdapter, id objective.ObjectiveID) []map[string]any {
	t.Helper()
	rows, err := store.ListToolEvents(context.Background(), storage.ToolEventFilter{ObjectiveID: string(id)})
	if err != nil {
		t.Fatalf("list tool events: %v", err)
	}
	var out []map[string]any
	for _, r := range rows {
		if r.Kind != storage.ToolEventBaseline {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(r.PayloadJSON), &p); err != nil {
			t.Fatalf("baseline row %s: payload is not JSON: %v (%q)", r.ID, err, r.PayloadJSON)
		}
		out = append(out, p)
	}
	return out
}

// 200 with the updated objective, the value stored, and one audit row naming
// the person the request came from.
func TestSetBaselineAnswersWithTheUpdatedObjective(t *testing.T) {
	srv, store, obj := baselineServer(t)

	rec := putBaseline(t, srv, string(obj.ID), "latency", `{"value": 200}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var answered objective.Objective
	if err := json.Unmarshal(rec.Body.Bytes(), &answered); err != nil {
		t.Fatalf("decode the objective: %v: %s", err, rec.Body)
	}
	if answered.ID != obj.ID {
		t.Errorf("answered objective %q, want %q", answered.ID, obj.ID)
	}
	if c := findCriterion(t, answered.SuccessCriteria, "latency"); c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Errorf("answered baseline_value = %v, want 200", c.BaselineValue)
	}

	stored, err := store.GetObjective(context.Background(), obj.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c := findCriterion(t, stored.SuccessCriteria, "latency"); c.BaselineValue == nil || *c.BaselineValue != 200 {
		t.Errorf("stored baseline_value = %v, want 200", c.BaselineValue)
	}

	rows := storedBaselineRows(t, store, obj.ID)
	if len(rows) != 1 {
		t.Fatalf("%d baseline audit rows, want 1: %v", len(rows), rows)
	}
	if rows[0]["set_by"] != baselinePerson {
		t.Errorf("row set_by = %#v, want %q, the principal the request came from (row %v)", rows[0]["set_by"], baselinePerson, rows[0])
	}
}

// 400 in JSON naming the field, and nothing stored or audited.
func TestSetBaselineRefusesNamingTheField(t *testing.T) {
	for _, tc := range []struct {
		name      string
		criterion string
		body      string
		field     string
	}{
		{name: "value missing", criterion: "latency", body: `{}`, field: "value"},
		{name: "value null", criterion: "latency", body: `{"value": null}`, field: "value"},
		{name: "value a string", criterion: "latency", body: `{"value": "200"}`, field: "value"},
		// Out of float64's range, the closest JSON comes to a value that is not finite.
		{name: "value not finite", criterion: "latency", body: `{"value": 1e999}`, field: "value"},
		{name: "a verified criterion", criterion: "tested", body: `{"value": 200}`, field: "kind"},
		{name: "a measured criterion with no change", criterion: "coverage", body: `{"value": 80}`, field: "change"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, store, obj := baselineServer(t)

			rec := putBaseline(t, srv, string(obj.ID), tc.criterion, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 naming %s: %s", rec.Code, tc.field, rec.Body)
			}
			if mediaType, _, _ := mime.ParseMediaType(rec.Header().Get("Content-Type")); mediaType != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json: %s", rec.Header().Get("Content-Type"), rec.Body)
			}
			var got struct {
				Error string `json:"error"`
				Field string `json:"field"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode the 400 body: %v: %s", err, rec.Body)
			}
			if got.Field != tc.field {
				t.Errorf("field = %q, want %q (body: %s)", got.Field, tc.field, rec.Body)
			}
			if got.Error == "" {
				t.Errorf("the 400 body says no reason: %s", rec.Body)
			}

			stored, err := store.GetObjective(context.Background(), obj.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if c := findCriterion(t, stored.SuccessCriteria, tc.criterion); c.BaselineValue != nil {
				t.Errorf("stored baseline_value = %v after a refused set, want none", *c.BaselineValue)
			}
			if rows := storedBaselineRows(t, store, obj.ID); len(rows) != 0 {
				t.Errorf("a refused set wrote %d baseline audit rows: %v", len(rows), rows)
			}
		})
	}
}

// 404 for an objective or a criterion that does not exist.
func TestSetBaselineAnswersNotFound(t *testing.T) {
	srv, store, obj := baselineServer(t)

	for name, path := range map[string][2]string{
		"unknown objective": {"no-such-objective", "latency"},
		"unknown criterion": {string(obj.ID), "no-such-criterion"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := putBaseline(t, srv, path[0], path[1], `{"value": 200}`)
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404: %s", rec.Code, rec.Body)
			}
		})
	}
	if rows := storedBaselineRows(t, store, obj.ID); len(rows) != 0 {
		t.Errorf("a refused set wrote %d baseline audit rows: %v", len(rows), rows)
	}
}
