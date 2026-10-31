package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 41 step 8 part C: a criterion's sustain is fixed at create time and
// read back as it was sent; an invalid one is a 400 naming the field. A change
// a person makes through the API restarts the sustain window, which the
// criterion records as sustain_since (docs/roadmap.md, Phase 41 section 6
// 'Targets belong to their author').

// sustainedLatency is a measured criterion relative to a baseline of 200 ms
// fixed by hand, held with step 8's sustain: window 28d, budget 0.1,
// min_passes 3.
const sustainedLatency = `{"id":"latency","description":"p95 latency is not more than 10% above the baseline",` +
	`"kind":"measured","verifier":"software.verify.latency","metric":"p95_latency_ms","op":"<=","target":10,"unit":"ms",` +
	`"baseline":200,"change":"percent","sustain":{"window":"28d","budget":0.1,"min_passes":3}}`

// sustainRouter serves POST and GET /objectives and the person's baseline PUT,
// as the caller baselinePerson.
func sustainRouter(t *testing.T) (http.Handler, *handler.ObjectiveHandler) {
	t.Helper()
	h := criteriaHandler(t)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{ID: baselinePerson})))
		})
	})
	r.Post("/api/v1/objectives", h.Create)
	r.Get("/api/v1/objectives/{id}", h.Get)
	r.Put("/api/v1/objectives/{id}/criteria/{criterion_id}/baseline", h.SetBaseline)
	return r, h
}

func serve(t *testing.T, srv http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// wireCriterion is the criterion as JSON on the wire, decoded loosely so a
// missing or renamed key shows up rather than a zero value.
func wireCriterion(t *testing.T, body []byte, id string) map[string]any {
	t.Helper()
	var o struct {
		SuccessCriteria []map[string]any `json:"success_criteria"`
	}
	if err := json.Unmarshal(body, &o); err != nil {
		t.Fatalf("decode the objective: %v: %s", err, body)
	}
	for _, c := range o.SuccessCriteria {
		if c["id"] == id {
			return c
		}
	}
	t.Fatalf("no criterion %q on the wire: %s", id, body)
	return nil
}

func createdID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var o objective.Objective
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatalf("decode the objective: %v: %s", err, rec.Body)
	}
	return string(o.ID)
}

// POST /objectives with a sustained criterion stores the sustain as sent, and
// GET answers it with the same window, budget and min_passes. A new criterion
// has no sustain_since: its window was never restarted.
func TestCreateObjectiveRoundTripsASustain(t *testing.T) {
	srv, _ := sustainRouter(t)
	id := createdID(t, serve(t, srv, http.MethodPost, "/api/v1/objectives",
		`{"title":"standing","domain":"software","success_criteria":[`+sustainedLatency+`]}`))

	rec := serve(t, srv, http.MethodGet, "/api/v1/objectives/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200: %s", rec.Code, rec.Body)
	}
	c := wireCriterion(t, rec.Body.Bytes(), "latency")
	sustain, ok := c["sustain"].(map[string]any)
	if !ok {
		t.Fatalf("GET criterion carries no sustain object: %v", c)
	}
	for key, want := range map[string]any{"window": "28d", "budget": 0.1, "min_passes": float64(3)} {
		if sustain[key] != want {
			t.Errorf("GET sustain.%s = %#v, want %#v (sustain %v)", key, sustain[key], want, sustain)
		}
	}
	if len(sustain) != 3 {
		t.Errorf("GET sustain = %v, want exactly window, budget and min_passes", sustain)
	}
	if since, ok := c["sustain_since"]; ok {
		t.Errorf("a new criterion carries sustain_since %v; its window was never restarted", since)
	}
}

// An invalid sustain is a 400 naming the field the way the request spells it,
// and so is a sustain_since in the request: when the window restarts is the
// server's record of a person's audited change, not something a request
// declares.
func TestCreateObjectiveRefusesAnInvalidSustainNamingTheField(t *testing.T) {
	const tested = `{"id":"tested","description":"the tests pass","kind":"verified","verifier":"software.verify.run_tests"}`
	const latency = `"id":"latency","description":"p95 latency holds","kind":"measured","verifier":"software.verify.latency",` +
		`"metric":"p95_latency_ms","op":"<=","target":200,"unit":"ms"`

	for name, tc := range map[string]struct {
		sustain string
		field   string
	}{
		"no window":             {`"sustain":{"budget":0.1,"min_passes":3}`, "success_criteria[1].sustain.window"},
		"a calendar window":     {`"sustain":{"window":"fortnight","budget":0.1,"min_passes":3}`, "success_criteria[1].sustain.window"},
		"a window with no unit": {`"sustain":{"window":"28","budget":0.1,"min_passes":3}`, "success_criteria[1].sustain.window"},
		"a zero window":         {`"sustain":{"window":"0d","budget":0.1,"min_passes":3}`, "success_criteria[1].sustain.window"},
		"a negative budget":     {`"sustain":{"window":"28d","budget":-0.1,"min_passes":3}`, "success_criteria[1].sustain.budget"},
		"a budget above 1":      {`"sustain":{"window":"28d","budget":1.5,"min_passes":3}`, "success_criteria[1].sustain.budget"},
		"a negative min_passes": {`"sustain":{"window":"28d","budget":0.1,"min_passes":-1}`, "success_criteria[1].sustain.min_passes"},
		"a request sustain_since": {
			`"sustain":{"window":"28d","budget":0.1,"min_passes":3},"sustain_since":"2026-10-01T00:00:00Z"`,
			"success_criteria[1].sustain_since",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assertFieldRefused(t, criteriaHandler(t),
				`{"title":"standing","domain":"software","success_criteria":[`+tested+`,{`+latency+`,`+tc.sustain+`}]}`, tc.field)
		})
	}
}

// A person setting the baseline through PUT .../baseline is a change to the
// criterion, so it restarts the sustain window: the answer and a later GET
// carry sustain_since, the time of the set, and the sustain itself is as it
// was created.
func TestSetBaselineRestartsTheSustainWindowOnTheWire(t *testing.T) {
	srv, _ := sustainRouter(t)
	id := createdID(t, serve(t, srv, http.MethodPost, "/api/v1/objectives",
		`{"title":"standing","domain":"software","success_criteria":[`+sustainedLatency+`]}`))

	before := time.Now().UTC()
	rec := serve(t, srv, http.MethodPut, "/api/v1/objectives/"+id+"/criteria/latency/baseline", `{"value":180}`)
	after := time.Now().UTC()
	if rec.Code != http.StatusOK {
		t.Fatalf("set baseline status = %d, want 200: %s", rec.Code, rec.Body)
	}

	for name, body := range map[string][]byte{
		"PUT answer": rec.Body.Bytes(),
		"later GET":  serve(t, srv, http.MethodGet, "/api/v1/objectives/"+id, "").Body.Bytes(),
	} {
		c := wireCriterion(t, body, "latency")
		raw, ok := c["sustain_since"].(string)
		if !ok {
			t.Errorf("%s: criterion carries no sustain_since after a person set its baseline: %v", name, c)
			continue
		}
		since, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t.Errorf("%s: sustain_since %q is not RFC 3339: %v", name, raw, err)
			continue
		}
		if since.Before(before) || since.After(after) {
			t.Errorf("%s: sustain_since = %v, want the time of the set, between %v and %v", name, since, before, after)
		}
		if s, _ := c["sustain"].(map[string]any); s["window"] != "28d" || s["budget"] != 0.1 || s["min_passes"] != float64(3) {
			t.Errorf("%s: sustain = %v after the set, want it as created", name, c["sustain"])
		}
	}
}
