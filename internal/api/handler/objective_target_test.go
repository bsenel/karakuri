package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bsenel/karakuri/auth"
)

// Phase 41 step 8: "A person's target change restarts the window." The change
// is PUT /objectives/{id}/criteria/{criterion_id}/target with {"value":
// number}, beside the baseline PUT, and it restarts the sustain window the
// same way: the criterion carries sustain_since, the time of the change.

// targetRouter serves POST and GET /objectives and the person's target PUT,
// as the caller baselinePerson.
func targetRouter(t *testing.T) http.Handler {
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
	r.Put("/api/v1/objectives/{id}/criteria/{criterion_id}/target", h.SetTarget)
	return r
}

func TestSetTargetRestartsTheSustainWindowOnTheWire(t *testing.T) {
	srv := targetRouter(t)
	id := createdID(t, serve(t, srv, http.MethodPost, "/api/v1/objectives",
		`{"title":"standing","domain":"software","success_criteria":[`+sustainedLatency+`]}`))

	before := time.Now().UTC()
	rec := serve(t, srv, http.MethodPut, "/api/v1/objectives/"+id+"/criteria/latency/target", `{"value":5}`)
	after := time.Now().UTC()
	if rec.Code != http.StatusOK {
		t.Fatalf("set target status = %d, want 200: %s", rec.Code, rec.Body)
	}

	for name, body := range map[string][]byte{
		"PUT answer": rec.Body.Bytes(),
		"later GET":  serve(t, srv, http.MethodGet, "/api/v1/objectives/"+id, "").Body.Bytes(),
	} {
		c := wireCriterion(t, body, "latency")
		if c["target"] != float64(5) {
			t.Errorf("%s: target = %v, want the person's 5", name, c["target"])
		}
		raw, ok := c["sustain_since"].(string)
		if !ok {
			t.Errorf("%s: criterion carries no sustain_since after a person changed its target: %v", name, c)
			continue
		}
		since, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t.Errorf("%s: sustain_since %q is not RFC 3339: %v", name, raw, err)
			continue
		}
		if since.Before(before) || since.After(after) {
			t.Errorf("%s: sustain_since = %v, want the time of the change, between %v and %v", name, since, before, after)
		}
		if s, _ := c["sustain"].(map[string]any); s["window"] != "28d" || s["budget"] != 0.1 || s["min_passes"] != float64(3) {
			t.Errorf("%s: sustain = %v after the change, want it as created", name, c["sustain"])
		}
	}
}

// A value that is not a finite number is a 400 naming value, and the target
// and the window are left as they were.
func TestSetTargetRefusesAValueThatIsNotANumber(t *testing.T) {
	srv := targetRouter(t)
	id := createdID(t, serve(t, srv, http.MethodPost, "/api/v1/objectives",
		`{"title":"standing","domain":"software","success_criteria":[`+sustainedLatency+`]}`))

	for _, body := range []string{`{}`, `{"value":null}`, `{"value":"5"}`} {
		rec := serve(t, srv, http.MethodPut, "/api/v1/objectives/"+id+"/criteria/latency/target", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400: %s", body, rec.Code, rec.Body)
			continue
		}
		var got struct {
			Field string `json:"field"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Errorf("body %s: decode the 400 body: %v: %s", body, err, rec.Body)
			continue
		}
		if got.Field != "value" {
			t.Errorf("body %s: field = %q, want value (body: %s)", body, got.Field, rec.Body)
		}
	}
	c := wireCriterion(t, serve(t, srv, http.MethodGet, "/api/v1/objectives/"+id, "").Body.Bytes(), "latency")
	if c["target"] != float64(10) {
		t.Errorf("target = %v after refused changes, want 10 unchanged", c["target"])
	}
	if _, ok := c["sustain_since"]; ok {
		t.Errorf("sustain_since = %v after refused changes, want none", c["sustain_since"])
	}
}
