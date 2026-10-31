package handler_test

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/objective"
	featureobj "github.com/bsenel/karakuri/internal/feature/objective"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 Step 5: success_criteria in POST /objectives. Every request here is
// raw JSON, so a field the handler fails to decode shows up as a case that is
// accepted when it should be refused, or stored without what was sent.

// criteriaHandler serves objectives with a verifier lookup holding a pass/fail
// verifier, two measuring ones and a discovered tool, and a template with one
// verified and two optional measured criteria a request can tighten.
func criteriaHandler(t *testing.T) *handler.ObjectiveHandler {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "objective.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	verifiers := capability.NewRegistry()
	for _, c := range []capability.Capability{
		{ID: "software.verify.run_tests", Domain: "software", Verifiable: true},
		{ID: "software.verify.coverage", Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "coverage", Unit: "percent", Direction: capability.DirectionHigherIsBetter,
			Params: []capability.MetricParam{{Name: "packages", Type: capability.ParamString}},
		}}},
		{ID: "software.verify.latency", Domain: "software", Verifiable: true, Metrics: []capability.Metric{{
			Name: "p95_latency_ms", Unit: "ms", Direction: capability.DirectionLowerIsBetter,
		}}},
		capability.NewMCPCapability("github", "merge_check", "says whether a PR can merge", capability.Schema{}),
	} {
		if err := verifiers.Register(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}
	optional, coverageTarget, latencyTarget := false, 80.0, 200.0
	svc := featureobj.NewService(storage.NewGORMStorage(db)).WithVerifiers(verifiers)
	svc.RegisterTemplate(objective.Template{ID: "tmpl.delivery", SuccessCriteria: []objective.Criterion{
		{ID: "tested", Description: "the tests pass", Kind: objective.KindVerified, Verifier: "software.verify.run_tests"},
		{
			ID: "coverage", Description: "coverage holds", Kind: objective.KindMeasured, Verifier: "software.verify.coverage",
			Metric: "coverage", Op: objective.OpGreaterEqual, Target: &coverageTarget, Unit: "percent", Required: &optional,
		},
		{
			ID: "latency", Description: "p95 latency holds", Kind: objective.KindMeasured, Verifier: "software.verify.latency",
			Metric: "p95_latency_ms", Op: objective.OpLessEqual, Target: &latencyTarget, Unit: "ms", Required: &optional,
		},
	}})
	return &handler.ObjectiveHandler{Objectives: svc}
}

func postObjective(t *testing.T, h *handler.ObjectiveHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/objectives", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)
	return rec
}

// assertFieldRefused checks a 400 whose JSON body names the field the caller
// has to change, and that nothing was saved.
func assertFieldRefused(t *testing.T, h *handler.ObjectiveHandler, body, field string) {
	t.Helper()
	rec := postObjective(t, h, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 naming %s: %s", rec.Code, field, rec.Body)
	}
	// JSON, so a client can read the field without parsing prose, and so the
	// integration suite checks the body against docs/openapi.yaml.
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
	if got.Field != field {
		t.Errorf("field = %q, want %q (body: %s)", got.Field, field, rec.Body)
	}
	if got.Error == "" {
		t.Errorf("the 400 body says no reason: %s", rec.Body)
	}
	objs, err := h.Objectives.List(context.Background(), storage.ObjectiveFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(objs) != 0 {
		t.Errorf("a refused objective was saved anyway: %+v", objs)
	}
}

// createdCriteria posts body, wants a 200, and returns the criteria both as the
// response carried them and as they were stored.
func createdCriteria(t *testing.T, h *handler.ObjectiveHandler, body string) (answered, stored []objective.Criterion) {
	t.Helper()
	rec := postObjective(t, h, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var o objective.Objective
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatalf("decode the objective: %v: %s", err, rec.Body)
	}
	got, err := h.Objectives.Get(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get %s: %v", o.ID, err)
	}
	return o.SuccessCriteria, got.SuccessCriteria
}

func findCriterion(t *testing.T, cs []objective.Criterion, id string) objective.Criterion {
	t.Helper()
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no criterion %q in %+v", id, cs)
	return objective.Criterion{}
}

func targetOf(c objective.Criterion) string {
	if c.Target == nil {
		return "none"
	}
	return strconv.FormatFloat(*c.Target, 'g', -1, 64)
}

// Each case the service refuses for a new criterion is a 400 through the
// handler, naming the field the way the request spells it.
func TestCreateObjectiveRefusesAnInvalidCriterionNamingTheField(t *testing.T) {
	// A valid criterion first, so the index is the request's own position.
	const tested = `{"id":"tested","description":"the tests pass","kind":"verified","verifier":"software.verify.run_tests"}`
	const coverage = `"id":"core-coverage","description":"core stays covered","kind":"measured","verifier":"software.verify.coverage","metric":"coverage"`

	for name, tc := range map[string]struct {
		criterion string
		field     string
	}{
		"no id":                       {`{"description":"x","kind":"verified","verifier":"software.verify.run_tests"}`, "success_criteria[1].id"},
		"no description":              {`{"id":"x","kind":"verified","verifier":"software.verify.run_tests"}`, "success_criteria[1].description"},
		"repeated id":                 {tested, "success_criteria[1].id"},
		"no kind":                     {`{"id":"x","description":"x","verifier":"software.verify.run_tests"}`, "success_criteria[1].kind"},
		"unknown kind":                {`{"id":"x","description":"x","kind":"vibes","verifier":"software.verify.run_tests"}`, "success_criteria[1].kind"},
		"judged without evidence":     {`{"id":"x","description":"an open PR with tests","kind":"judged"}`, "success_criteria[1].evidence"},
		"judged with a verifier":      {`{"id":"x","description":"x","kind":"judged","evidence":"the PR","verifier":"software.verify.run_tests"}`, "success_criteria[1].verifier"},
		"verified without a verifier": {`{"id":"x","description":"x","kind":"verified"}`, "success_criteria[1].verifier"},
		"verified with a parameter":   {`{"id":"x","description":"x","kind":"verified","verifier":"software.verify.run_tests","params":{"packages":"./..."}}`, "success_criteria[1].params.packages"},
		"unknown verifier":            {`{"id":"x","description":"x","kind":"verified","verifier":"software.verify.nope"}`, "success_criteria[1].verifier"},
		"reserved verifier":           {`{"id":"x","description":"x","kind":"verified","verifier":"mcp.ghost.check"}`, "success_criteria[1].verifier"},
		"discovered verifier":         {`{"id":"x","description":"x","kind":"verified","verifier":"` + string(capability.MCPCapabilityID("github", "merge_check")) + `"}`, "success_criteria[1].verifier"},
		"measured without a metric":   {`{"id":"x","description":"x","kind":"measured","verifier":"software.verify.coverage","op":">=","target":90,"unit":"percent"}`, "success_criteria[1].metric"},
		"undeclared metric":           {`{"id":"x","description":"x","kind":"measured","verifier":"software.verify.coverage","metric":"branch_coverage","op":">=","target":90,"unit":"percent"}`, "success_criteria[1].metric"},
		"unit mismatch":               {`{` + coverage + `,"op":">=","target":90,"unit":"ratio"}`, "success_criteria[1].unit"},
		"target without an op":        {`{` + coverage + `,"target":90,"unit":"percent"}`, "success_criteria[1].op"},
		"unknown op":                  {`{` + coverage + `,"op":"~=","target":90,"unit":"percent"}`, "success_criteria[1].op"},
		"op without a target":         {`{` + coverage + `,"op":">=","unit":"percent"}`, "success_criteria[1].target"},
		"unknown parameter":           {`{` + coverage + `,"op":">=","target":90,"unit":"percent","params":{"modules":"core"}}`, "success_criteria[1].params.modules"},
	} {
		t.Run(name, func(t *testing.T) {
			assertFieldRefused(t, criteriaHandler(t),
				`{"title":"plain","domain":"software","success_criteria":[`+tested+`,`+tc.criterion+`]}`, tc.field)
		})
	}
}

// Request criteria whose every entry is required: false leave nothing to gate
// completion, and the list itself is the field to change.
func TestCreateObjectiveRefusesCriteriaWithNoneRequired(t *testing.T) {
	assertFieldRefused(t, criteriaHandler(t), `{"title":"plain","domain":"software","success_criteria":[
		{"id":"reviewed","description":"an open PR with tests","kind":"judged","evidence":"the PR","required":false},
		{"id":"tested","description":"the tests pass","kind":"verified","verifier":"software.verify.run_tests","required":false}
	]}`, "success_criteria")
}

// A request may make a template criterion stricter, and the stricter one is
// what is stored, in the template's place.
func TestCreateObjectiveAcceptsATightenedTemplateCriterion(t *testing.T) {
	answered, stored := createdCriteria(t, criteriaHandler(t), `{"title":"ship it","domain":"software","template_id":"tmpl.delivery",
		"success_criteria":[{"id":"coverage","description":"coverage holds at 90","kind":"measured",
			"verifier":"software.verify.coverage","metric":"coverage","op":">=","target":90,"unit":"percent","required":true}]}`)

	for name, cs := range map[string][]objective.Criterion{"answered": answered, "stored": stored} {
		if len(cs) != 3 {
			t.Errorf("%s criteria = %+v, want the template's three with coverage tightened in place", name, cs)
		}
		c := findCriterion(t, cs, "coverage")
		if c.Target == nil || *c.Target != 90 {
			t.Errorf("%s coverage target = %s, want the request's 90", name, targetOf(c))
		}
		if !c.IsRequired() {
			t.Errorf("%s coverage is optional, want the request's required: true", name)
		}
		if c.Description != "coverage holds at 90" {
			t.Errorf("%s coverage description = %q, want the request's", name, c.Description)
		}
	}
}

// Anything else a request does to a template criterion loosens or replaces it,
// and is refused naming what it changed.
func TestCreateObjectiveRefusesALoosenedTemplateCriterion(t *testing.T) {
	const coverage = `"id":"coverage","description":"coverage holds","kind":"measured","verifier":"software.verify.coverage","metric":"coverage"`
	const latency = `"id":"latency","description":"p95 latency holds","kind":"measured","verifier":"software.verify.latency","metric":"p95_latency_ms"`

	for name, tc := range map[string]struct {
		criterion string
		field     string
	}{
		"lower target on >=":   {`{` + coverage + `,"op":">=","target":70,"unit":"percent"}`, "success_criteria[0].target"},
		"higher target on <=":  {`{` + latency + `,"op":"<=","target":300,"unit":"ms"}`, "success_criteria[0].target"},
		"target removed":       {`{` + coverage + `,"op":">=","unit":"percent"}`, "success_criteria[0].target"},
		"required to optional": {`{"id":"tested","description":"the tests pass","kind":"verified","verifier":"software.verify.run_tests","required":false}`, "success_criteria[0].required"},
		"different kind":       {`{"id":"tested","description":"the tests pass","kind":"judged","evidence":"the CI log"}`, "success_criteria[0].kind"},
		"different verifier":   {`{"id":"coverage","description":"coverage holds","kind":"measured","verifier":"software.verify.latency","metric":"p95_latency_ms","op":">=","target":80,"unit":"ms"}`, "success_criteria[0].verifier"},
		"different metric":     {`{"id":"latency","description":"p95 latency holds","kind":"measured","verifier":"software.verify.latency","metric":"coverage","op":"<=","target":200,"unit":"ms"}`, "success_criteria[0].metric"},
		"different unit":       {`{` + coverage + `,"op":">=","target":80,"unit":"ratio"}`, "success_criteria[0].unit"},
		"different op":         {`{` + coverage + `,"op":"==","target":80,"unit":"percent"}`, "success_criteria[0].op"},
		"different params":     {`{` + coverage + `,"op":">=","target":80,"unit":"percent","params":{"packages":"./internal/core/..."}}`, "success_criteria[0].params"},
		"different domain":     {`{` + coverage + `,"op":">=","target":80,"unit":"percent","domain":"agriculture"}`, "success_criteria[0].domain"},
		"description dropped":  {`{"id":"coverage","description":"","kind":"measured","verifier":"software.verify.coverage","metric":"coverage","op":">=","target":80,"unit":"percent"}`, "success_criteria[0].description"},
	} {
		t.Run(name, func(t *testing.T) {
			assertFieldRefused(t, criteriaHandler(t),
				`{"title":"ship it","domain":"software","template_id":"tmpl.delivery","success_criteria":[`+tc.criterion+`]}`, tc.field)
		})
	}
}

// A valid request criterion of each kind is stored with every field it was
// sent with, and the response carries the same, description and evidence
// among them.
func TestCreateObjectiveStoresRequestCriteria(t *testing.T) {
	answered, stored := createdCriteria(t, criteriaHandler(t), `{"title":"plain","domain":"software","success_criteria":[
		{"id":"reviewed","description":"an open PR with tests","kind":"judged","evidence":"the PR's diff and its test files","weight":0.5},
		{"id":"tested","description":"the tests pass","kind":"verified","verifier":"software.verify.run_tests","domain":"software"},
		{"id":"core-coverage","description":"core stays covered","kind":"measured","verifier":"software.verify.coverage",
			"metric":"coverage","op":">=","target":85.5,"unit":"percent","params":{"packages":"./internal/core/..."},"required":false}
	]}`)

	for name, cs := range map[string][]objective.Criterion{"answered": answered, "stored": stored} {
		t.Run(name, func(t *testing.T) {
			if len(cs) != 3 {
				t.Fatalf("criteria = %+v, want the request's three", cs)
			}

			judged := findCriterion(t, cs, "reviewed")
			if judged.Description != "an open PR with tests" {
				t.Errorf("reviewed description = %q, want the request's", judged.Description)
			}
			if judged.Kind != objective.KindJudged {
				t.Errorf("reviewed kind = %q, want judged", judged.Kind)
			}
			if judged.Evidence != "the PR's diff and its test files" {
				t.Errorf("reviewed evidence = %q, want the request's", judged.Evidence)
			}
			if judged.Weight != 0.5 {
				t.Errorf("reviewed weight = %v, want 0.5", judged.Weight)
			}

			verified := findCriterion(t, cs, "tested")
			if verified.Description != "the tests pass" || verified.Kind != objective.KindVerified ||
				verified.Verifier != "software.verify.run_tests" || verified.Domain != "software" {
				t.Errorf("tested = %+v, want the request's description, kind, verifier and domain", verified)
			}
			if !verified.IsRequired() {
				t.Errorf("tested is optional, want required by default")
			}

			measured := findCriterion(t, cs, "core-coverage")
			if measured.Description != "core stays covered" || measured.Kind != objective.KindMeasured ||
				measured.Verifier != "software.verify.coverage" || measured.Metric != "coverage" ||
				measured.Op != objective.OpGreaterEqual || measured.Unit != "percent" {
				t.Errorf("core-coverage = %+v, want the request's description, kind, verifier, metric, op and unit", measured)
			}
			if measured.Target == nil || *measured.Target != 85.5 {
				t.Errorf("core-coverage target = %s, want 85.5", targetOf(measured))
			}
			if measured.Params["packages"] != "./internal/core/..." {
				t.Errorf("core-coverage params = %v, want packages ./internal/core/...", measured.Params)
			}
			if measured.IsRequired() {
				t.Errorf("core-coverage is required, want the request's required: false")
			}
		})
	}
}
