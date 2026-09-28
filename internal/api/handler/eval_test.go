package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/feature/eval"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
)

// fakeCalibrator records the filter it was asked for and answers a fixed report.
type fakeCalibrator struct {
	calls  int
	filter storage.ResolvedCheckpointFilter
	report eval.CalibrationReport
	err    error
}

func (f *fakeCalibrator) Calibrate(_ context.Context, filter storage.ResolvedCheckpointFilter) (eval.CalibrationReport, error) {
	f.calls++
	f.filter = filter
	return f.report, f.err
}

// evalRouter mounts the calibrate route the way server.go does: authenticated,
// then gated on eval:run. A bearer token names the principal; no token is no
// principal. The principal "admin" holds eval:run and anybody else does not.
func evalRouter(cal *fakeCalibrator) http.Handler {
	resolve := auth.ResolverFunc(func(r *http.Request) (auth.Principal, error) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			return auth.Principal{}, errors.New("no credential")
		}
		return auth.Principal{ID: tok}, nil
	})
	enf := auth.NewEnforcer(principalAuthorizer{"admin": {karakuriauth.ActionEvalRun: true}})
	h := &handler.EvalHandler{Calibrator: cal}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(auth.Authenticate(resolve))
		r.With(enf.Require(karakuriauth.ActionEvalRun, nil)).Post("/eval/calibrate", h.Calibrate)
	})
	return r
}

// principalAuthorizer grants each principal exactly the actions listed for it.
type principalAuthorizer map[string]map[auth.Action]bool

func (a principalAuthorizer) Authorize(_ context.Context, p auth.Principal, action auth.Action, _ auth.ResourceRef) (auth.Decision, error) {
	if a[p.ID][action] {
		return auth.Decision{Allowed: true, PrincipalID: p.ID, Action: action}, nil
	}
	return auth.Decision{Allowed: false, PrincipalID: p.ID, Action: action, Reason: "no binding grants " + string(action)}, nil
}

func postCalibrate(t *testing.T, h http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/eval/calibrate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEvalCalibrateRequiresAToken(t *testing.T) {
	cal := &fakeCalibrator{}
	rec := postCalibrate(t, evalRouter(cal), "", `{}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body)
	}
	if cal.calls != 0 {
		t.Fatal("an unauthenticated request reached the calibrator")
	}
}

func TestEvalCalibrateRequiresEvalRun(t *testing.T) {
	cal := &fakeCalibrator{}
	rec := postCalibrate(t, evalRouter(cal), "operator", `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body)
	}
	if cal.calls != 0 {
		t.Fatal("a caller without eval:run reached the calibrator")
	}
}

func TestEvalCalibrateRejectsBadInput(t *testing.T) {
	for name, body := range map[string]string{
		"malformed json": `{"twin":`,
		"bad since":      `{"since":"yesterday"}`,
		"bad until":      `{"until":"2026-13-45"}`,
	} {
		t.Run(name, func(t *testing.T) {
			cal := &fakeCalibrator{}
			rec := postCalibrate(t, evalRouter(cal), "admin", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
			}
			if cal.calls != 0 {
				t.Fatal("a bad request reached the calibrator")
			}
		})
	}
}

func TestEvalCalibratePassesTheWindowThrough(t *testing.T) {
	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cal := &fakeCalibrator{}
	body := `{"twin":"t1","since":"` + since.Format(time.RFC3339) +
		`","until":"` + until.Format(time.RFC3339) + `","limit":5}`

	rec := postCalibrate(t, evalRouter(cal), "admin", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if cal.calls != 1 {
		t.Fatalf("calibrator called %d times, want 1", cal.calls)
	}
	f := cal.filter
	if f.TwinID != "t1" {
		t.Errorf("twin = %q, want t1", f.TwinID)
	}
	if !f.Since.Equal(since) {
		t.Errorf("since = %v, want %v", f.Since, since)
	}
	if !f.Until.Equal(until) {
		t.Errorf("until = %v, want %v", f.Until, until)
	}
	if f.Limit != 5 {
		t.Errorf("limit = %d, want 5", f.Limit)
	}
}

func TestEvalCalibrateAcceptsAnEmptyBody(t *testing.T) {
	cal := &fakeCalibrator{}
	rec := postCalibrate(t, evalRouter(cal), "admin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if cal.calls != 1 {
		t.Fatalf("calibrator called %d times, want 1", cal.calls)
	}
	if cal.filter.TwinID != "" {
		t.Errorf("twin = %q, want every twin", cal.filter.TwinID)
	}
}

// The report goes out as eval.CalibrationReport encodes. It carries no JSON
// tags, so the fields are its Go names — the CLI decodes the same names.
func TestEvalCalibrateReturnsTheReport(t *testing.T) {
	cal := &fakeCalibrator{report: eval.CalibrationReport{
		TwinID: "t1", N: 2, Agreed: 1, Skipped: 3, Agreement: 0.5, Replayable: 4,
		Confusion: eval.Confusion{JudgePassHumanApprove: 1, JudgePassHumanReject: 1},
		ByDecision: map[string]eval.DecisionStats{
			"approve": {N: 1, Agreed: 1, JudgePass: 1},
			"reject":  {N: 1, Agreed: 0, JudgePass: 1},
		},
		Items: []eval.Item{
			{CheckpointID: "cp-1", Choice: "approve", HumanApprove: true, JudgePass: true, Agreed: true, Reply: "PASS"},
			{CheckpointID: "cp-2", Choice: "reject", JudgePass: true, Reply: "PASS — looks fine"},
		},
	}}
	rec := postCalibrate(t, evalRouter(cal), "admin", `{"twin":"t1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content type = %q", ct)
	}

	var got struct {
		TwinID             string
		N, Agreed, Skipped int
		Agreement          float64
		Replayable         int
		Confusion          eval.Confusion
		ByDecision         map[string]eval.DecisionStats
		Items              []struct{ CheckpointID, Choice, Reply string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body)
	}
	if got.TwinID != "t1" || got.N != 2 || got.Agreed != 1 || got.Skipped != 3 || got.Agreement != 0.5 {
		t.Errorf("summary = %+v", got)
	}
	if got.Replayable != 4 {
		t.Errorf("Replayable = %d, want 4", got.Replayable)
	}
	if got.Confusion != cal.report.Confusion {
		t.Errorf("Confusion = %+v, want %+v", got.Confusion, cal.report.Confusion)
	}
	if got.ByDecision["reject"].JudgePass != 1 {
		t.Errorf("ByDecision = %+v", got.ByDecision)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", got.Items)
	}
	if got.Items[1].CheckpointID != "cp-2" || got.Items[1].Reply != "PASS — looks fine" {
		t.Errorf("item = %+v, want the judge's reply kept", got.Items[1])
	}
}

func TestEvalCalibrateServiceErrorIs500(t *testing.T) {
	cal := &fakeCalibrator{err: errors.New("database is gone")}
	rec := postCalibrate(t, evalRouter(cal), "admin", `{}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
}

// scopeGrants answers the same grants for every principal and action.
type scopeGrants auth.ScopeGrants

func (g scopeGrants) GrantedScopes(context.Context, string, auth.Action) (auth.ScopeGrants, error) {
	return auth.ScopeGrants(g), nil
}

// scopedEvalRouter mounts the handler behind authentication only, with the
// caller's twin grants fixed, so a test exercises the handler's own scoping.
func scopedEvalRouter(cal *fakeCalibrator, grants auth.ScopeGrants) http.Handler {
	resolve := auth.ResolverFunc(func(*http.Request) (auth.Principal, error) {
		return auth.Principal{ID: "admin"}, nil
	})
	h := &handler.EvalHandler{Calibrator: cal, Scopes: scopeGrants(grants)}
	r := chi.NewRouter()
	r.With(auth.Authenticate(resolve)).Post("/api/v1/eval/calibrate", h.Calibrate)
	return r
}

func TestEvalCalibrateScopesTheTwin(t *testing.T) {
	oneTwin := auth.ScopeGrants{Allow: []string{"twin:t1"}}
	for name, tc := range map[string]struct {
		grants   auth.ScopeGrants
		body     string
		status   int
		wantTwin string
	}{
		"unrestricted keeps every twin": {auth.ScopeGrants{Allow: []string{"*"}}, `{}`, http.StatusOK, ""},
		"granted twin":                  {oneTwin, `{"twin":"t1"}`, http.StatusOK, "t1"},
		"other twin":                    {oneTwin, `{"twin":"t2"}`, http.StatusForbidden, ""},
		"no twin names the only one":    {oneTwin, `{}`, http.StatusOK, "t1"},
		"no twin with several":          {auth.ScopeGrants{Allow: []string{"twin:t1", "twin:t2"}}, `{}`, http.StatusForbidden, ""},
		"denied twin":                   {auth.ScopeGrants{Allow: []string{"*"}, Deny: []string{"twin:t2"}}, `{"twin":"t2"}`, http.StatusForbidden, ""},
	} {
		t.Run(name, func(t *testing.T) {
			cal := &fakeCalibrator{}
			rec := postCalibrate(t, scopedEvalRouter(cal, tc.grants), "", tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body)
			}
			if tc.status != http.StatusOK {
				if cal.calls != 0 {
					t.Fatal("a forbidden request reached the calibrator")
				}
				return
			}
			if cal.filter.TwinID != tc.wantTwin {
				t.Errorf("twin = %q, want %q", cal.filter.TwinID, tc.wantTwin)
			}
		})
	}
}

func TestEvalCalibrateWithNoTwinGrantsReadsNothing(t *testing.T) {
	cal := &fakeCalibrator{}
	rec := postCalibrate(t, scopedEvalRouter(cal, auth.ScopeGrants{Allow: []string{}}), "", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if cal.calls != 0 {
		t.Fatal("a caller with no twin grants reached the calibrator")
	}
}
