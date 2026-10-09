package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/api/handler"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
)

// withCheckpoints wires the checkpoint service over the fixture's store and
// stores cps in it.
func (f *mcpFixture) withCheckpoints(t *testing.T, cps ...corecheckpoint.Checkpoint) *mcpFixture {
	t.Helper()
	f.h.Checkpoints = featurecp.NewService(f.store, nil)
	for _, cp := range cps {
		if cp.Status == "" {
			cp.Status = corecheckpoint.StatusPending
		}
		if cp.CreatedAt.IsZero() {
			cp.CreatedAt = time.Now().UTC()
		}
		if err := f.store.SaveCheckpoint(context.Background(), cp); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// withCosts wires a ledger holding events into the fixture.
func (f *mcpFixture) withCosts(t *testing.T, events ...cost.Event) *mcpFixture {
	t.Helper()
	ledger := cost.NewMemoryLedger()
	for _, e := range events {
		if err := ledger.Record(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	f.h.Quota = karakuriquota.Deps{Costs: &karakuriquota.Recorder{Ledger: ledger}}
	return f
}

// restCost is what GET /cost answers the fixture's principal for args, read
// through the handler the route is served by, over the same ledger.
func (f *mcpFixture) restCost(t *testing.T, args map[string]any) any {
	t.Helper()
	q := url.Values{}
	for k, v := range args {
		q.Set(k, fmt.Sprint(v))
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cost?"+q.Encode(), nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.Principal{ID: "runtime-1"}))
	rec := httptest.NewRecorder()
	(&handler.QuotaHandler{Quota: f.h.Quota}).CostReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cost?%s = %d %s", q.Encode(), rec.Code, rec.Body.String())
	}
	return decodeAny(t, rec.Body.String())
}

func decodeAny(t *testing.T, text string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("not JSON: %v: %s", err, text)
	}
	return v
}

// sameJSON reports whether two values encode to the same JSON document.
func sameJSON(t *testing.T, got, want any) bool {
	t.Helper()
	a, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	return string(mustReencode(t, a)) == string(mustReencode(t, b))
}

func mustReencode(t *testing.T, raw []byte) []byte {
	t.Helper()
	out, err := json.Marshal(decodeAny(t, string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// checkpointIDs calls checkpoints_list and returns the IDs it listed, sorted.
func checkpointIDs(t *testing.T, f *mcpFixture, args map[string]any) []string {
	t.Helper()
	res := toolResult(t, f.call(t, "checkpoints_list", args))
	if res.IsError {
		t.Fatalf("checkpoints_list(%v) failed: %s", args, res.Text())
	}
	var cps []corecheckpoint.Checkpoint
	if err := json.Unmarshal([]byte(res.Text()), &cps); err != nil {
		t.Fatalf("checkpoints_list result is not a JSON array of checkpoints: %v: %s", err, res.Text())
	}
	ids := make([]string, 0, len(cps))
	for _, cp := range cps {
		ids = append(ids, cp.ID)
	}
	slices.Sort(ids)
	return ids
}

// Each tool is reachable exactly when its REST route would be: GET
// /checkpoints and GET /checkpoints/{id} demand checkpoint:read, GET /cost
// demands cost:read.
func TestMCPCheckpointAndCostToolsFollowTheirRoutesActions(t *testing.T) {
	cases := []struct {
		tool   string
		action auth.Action
		args   map[string]any
	}{
		{"checkpoints_list", karakuriauth.ActionCheckpointRead, nil},
		{"checkpoint_read", karakuriauth.ActionCheckpointRead, map[string]any{"checkpoint_id": "cp-1"}},
		{"cost_report", karakuriauth.ActionCostRead, nil},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			seed := corecheckpoint.Checkpoint{ID: "cp-1", ObjectiveID: "obj-1", TwinID: "twin-1", Summary: "Ship it?"}

			holder := newMCPFixture(t, tc.action).withCheckpoints(t, seed).withCosts(t)
			if !holder.toolNames(t)[tc.tool] {
				t.Errorf("%s is not listed for a principal holding %s", tc.tool, tc.action)
			}
			if res := toolResult(t, holder.call(t, tc.tool, tc.args)); res.IsError {
				t.Errorf("%s failed for a principal holding %s: %s", tc.tool, tc.action, res.Text())
			}

			// Everything nearby except the tool's own action.
			var others []auth.Action
			for _, a := range []auth.Action{
				karakuriauth.ActionObjectiveRead, karakuriauth.ActionReportRead, karakuriauth.ActionAuditRead,
				karakuriauth.ActionCheckpointRead, karakuriauth.ActionCheckpointResolve,
				karakuriauth.ActionCostRead, karakuriauth.ActionQuotaRead, karakuriauth.ActionTwinRead,
			} {
				if a != tc.action {
					others = append(others, a)
				}
			}
			without := newMCPFixture(t, others...).withCheckpoints(t, seed).withCosts(t)
			if without.toolNames(t)[tc.tool] {
				t.Errorf("%s is listed for a principal without %s", tc.tool, tc.action)
			}
			resp := without.call(t, tc.tool, tc.args)
			if resp.Error == nil || resp.Error.Code != -32003 {
				t.Fatalf("%s without %s = %+v, want the forbidden code", tc.tool, tc.action, resp)
			}
			if !strings.Contains(resp.Error.Message, string(tc.action)) {
				t.Errorf("refusal %q does not name %s", resp.Error.Message, tc.action)
			}
			if len(without.denied) != 1 || !strings.HasSuffix(without.denied[0], "/mcp/"+tc.tool) {
				t.Errorf("OnDeny saw %v, want one refusal named for %s", without.denied, tc.tool)
			}
		})
	}
}

// checkpoints_list is GET /checkpoints: the pending checkpoints, narrowed by
// the one filter that handler applies, twin_id.
func TestMCPCheckpointsListReturnsPendingCheckpoints(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionCheckpointRead).withCheckpoints(t,
		corecheckpoint.Checkpoint{ID: "cp-a", ObjectiveID: "obj-1", TwinID: "twin-1", Summary: "Merge the fix?"},
		corecheckpoint.Checkpoint{ID: "cp-b", ObjectiveID: "obj-2", TwinID: "twin-2", Summary: "Raise the budget?"},
		corecheckpoint.Checkpoint{ID: "cp-done", ObjectiveID: "obj-1", TwinID: "twin-1", Status: corecheckpoint.StatusResolved},
	)

	// A resolved checkpoint is not pending, and is not listed.
	if ids := checkpointIDs(t, f, nil); !slices.Equal(ids, []string{"cp-a", "cp-b"}) {
		t.Errorf("checkpoints_list = %v, want the two pending checkpoints", ids)
	}
	if ids := checkpointIDs(t, f, map[string]any{"twin_id": "twin-2"}); !slices.Equal(ids, []string{"cp-b"}) {
		t.Errorf("checkpoints_list twin_id=twin-2 = %v, want cp-b alone", ids)
	}

	// The same rows the service the REST handler calls returns.
	want, err := f.h.Checkpoints.ListPending(context.Background(), "twin-1")
	if err != nil {
		t.Fatal(err)
	}
	res := toolResult(t, f.call(t, "checkpoints_list", map[string]any{"twin_id": "twin-1"}))
	if !sameJSON(t, decodeAny(t, res.Text()), want) {
		t.Errorf("checkpoints_list twin_id=twin-1 = %s, want what ListPending returns", res.Text())
	}
}

// checkpoint_read is GET /checkpoints/{id}.
func TestMCPCheckpointReadReturnsOneCheckpoint(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionCheckpointRead).withCheckpoints(t,
		corecheckpoint.Checkpoint{ID: "cp-a", ObjectiveID: "obj-1", TwinID: "twin-1", Summary: "Merge the fix?", Options: []string{"approve", "reject"}},
		corecheckpoint.Checkpoint{ID: "cp-b", ObjectiveID: "obj-2", TwinID: "twin-2", Summary: "Raise the budget?"},
	)

	res := toolResult(t, f.call(t, "checkpoint_read", map[string]any{"checkpoint_id": "cp-a"}))
	if res.IsError {
		t.Fatalf("checkpoint_read failed: %s", res.Text())
	}
	var got corecheckpoint.Checkpoint
	if err := json.Unmarshal([]byte(res.Text()), &got); err != nil {
		t.Fatalf("checkpoint_read result is not a checkpoint: %v: %s", err, res.Text())
	}
	if got.ID != "cp-a" || got.ObjectiveID != "obj-1" || got.Summary != "Merge the fix?" || !slices.Equal(got.Options, []string{"approve", "reject"}) {
		t.Errorf("checkpoint_read cp-a = %+v", got)
	}
	want, err := f.h.Checkpoints.Get(context.Background(), "cp-a")
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, decodeAny(t, res.Text()), want) {
		t.Errorf("checkpoint_read cp-a = %s, want what Get returns", res.Text())
	}
}

// An id that names nothing is a bad answer to a fair question: a tool error,
// neither a protocol error nor an empty checkpoint a model would read as one.
func TestMCPCheckpointReadUnknownIDIsAToolError(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionCheckpointRead).withCheckpoints(t,
		corecheckpoint.Checkpoint{ID: "cp-a", ObjectiveID: "obj-1", TwinID: "twin-1"})

	for _, args := range []map[string]any{{"checkpoint_id": "cp-missing"}, nil} {
		resp := f.call(t, "checkpoint_read", args)
		if resp.Error != nil {
			t.Fatalf("checkpoint_read(%v) = protocol error %+v, want a tool error", args, resp.Error)
		}
		res := toolResult(t, resp)
		if !res.IsError {
			t.Errorf("checkpoint_read(%v) = %s, want a tool error", args, res.Text())
		}
		if text := strings.TrimSpace(res.Text()); text == "" || strings.HasPrefix(text, "{") {
			t.Errorf("checkpoint_read(%v) answered %q, want a message", args, res.Text())
		}
	}
}

// refAuthorizer allows an action only against the references it names, and
// records every reference it was asked about.
type refAuthorizer struct {
	action  auth.Action
	allowed map[string]bool // "type/id"
	asked   []auth.ResourceRef
}

func (a *refAuthorizer) Authorize(_ context.Context, p auth.Principal, action auth.Action, ref auth.ResourceRef) (auth.Decision, error) {
	a.asked = append(a.asked, ref)
	if action == a.action && a.allowed[ref.Type+"/"+ref.ID] {
		return auth.Decision{Allowed: true, PrincipalID: p.ID, Action: action}, nil
	}
	return auth.Decision{Allowed: false, PrincipalID: p.ID, Action: action, Reason: "no binding grants " + string(action)}, nil
}

// GET /checkpoints/{id} is gated on checkpoint:read against
// karakuriauth.CheckpointResource: the checkpoint named in the URL, by id,
// with no containers attached. The tool asks the same question of the same
// reference, so a principal whose grant does not reach a checkpoint over REST
// does not reach it here, and holding objective:read on its objective is not a
// way in on either surface.
func TestMCPCheckpointReadIsScopedAsItsRouteIs(t *testing.T) {
	f := newMCPFixture(t).withCheckpoints(t,
		corecheckpoint.Checkpoint{ID: "cp-mine", ObjectiveID: "obj-1", TwinID: "twin-1", Summary: "Mine"},
		corecheckpoint.Checkpoint{ID: "cp-theirs", ObjectiveID: "obj-2", TwinID: "twin-2", Summary: "Theirs"},
	)
	authz := &refAuthorizer{action: karakuriauth.ActionCheckpointRead, allowed: map[string]bool{"checkpoint/cp-mine": true}}
	enf := auth.NewEnforcer(authz)
	enf.OnDeny = f.h.Enforcer.OnDeny
	f.h.Enforcer = enf

	if res := toolResult(t, f.call(t, "checkpoint_read", map[string]any{"checkpoint_id": "cp-mine"})); res.IsError || !strings.Contains(res.Text(), "Mine") {
		t.Errorf("checkpoint_read cp-mine = %+v", res)
	}

	// What the REST route would have asked about, built by the route's own
	// resource function; there is no chi route here to carry the id.
	rest := karakuriauth.CheckpointResource(httptest.NewRequest(http.MethodGet, "/api/v1/checkpoints/", nil))
	rest.ID = "cp-theirs"

	authz.asked = nil
	resp := f.call(t, "checkpoint_read", map[string]any{"checkpoint_id": "cp-theirs"})
	if resp.Error == nil || resp.Error.Code != -32003 {
		t.Fatalf("checkpoint_read cp-theirs = %+v, want the forbidden code", resp)
	}
	if strings.Contains(string(resp.Result), "Theirs") {
		t.Errorf("a refused read leaked the checkpoint: %s", resp.Result)
	}
	if len(authz.asked) != 1 || authz.asked[0].Type != rest.Type || authz.asked[0].ID != rest.ID || len(authz.asked[0].Scopes) != len(rest.Scopes) {
		t.Errorf("checkpoint_read was decided against %+v, want the route's %+v", authz.asked, rest)
	}
	if len(f.denied) != 1 || !strings.HasSuffix(f.denied[0], "/mcp/checkpoint_read") {
		t.Errorf("OnDeny saw %v, want one refusal named for checkpoint_read", f.denied)
	}

	// objective:read on the checkpoint's objective opens neither surface.
	f.h.Enforcer = auth.NewEnforcer(&refAuthorizer{
		action: karakuriauth.ActionObjectiveRead, allowed: map[string]bool{"objective/obj-2": true}})
	if resp := f.call(t, "checkpoint_read", map[string]any{"checkpoint_id": "cp-theirs"}); resp.Error == nil || resp.Error.Code != -32003 {
		t.Errorf("checkpoint_read with only objective:read = %+v, want the forbidden code", resp)
	}
}

// cost_report is GET /cost: the same buckets for the same parameters, and each
// parameter changes the answer, so one the tool drops is caught.
func TestMCPCostReportMatchesTheRESTReport(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }
	f := newMCPFixture(t, karakuriauth.ActionCostRead).withCosts(t,
		cost.Event{Subject: karakuriquota.CostSubject("twin-1"), ResourceType: "twin", ResourceID: "twin-1", Provider: "anthropic", Model: "m-1", Units: 100, UnitKind: "tokens", Cost: 4, OccurredAt: day(1), Labels: []string{"org:o_1"}},
		cost.Event{Subject: karakuriquota.CostSubject("twin-1"), ResourceType: "twin", ResourceID: "twin-1", Provider: "openai", Model: "m-2", Units: 200, UnitKind: "tokens", Cost: 2, OccurredAt: day(10), Labels: []string{"org:o_1"}},
		cost.Event{Subject: karakuriquota.CostSubject("twin-2"), ResourceType: "twin", ResourceID: "twin-2", Provider: "anthropic", Model: "m-1", Units: 300, UnitKind: "tokens", Cost: 1, OccurredAt: day(20), Labels: []string{"org:o_2"}},
	)

	report := func(args map[string]any) any {
		t.Helper()
		res := toolResult(t, f.call(t, "cost_report", args))
		if res.IsError {
			t.Fatalf("cost_report(%v) failed: %s", args, res.Text())
		}
		return decodeAny(t, res.Text())
	}

	all := f.restCost(t, nil)
	if got := report(nil); !sameJSON(t, got, all) {
		t.Errorf("cost_report = %v, want what GET /cost returns: %v", got, all)
	}

	// The arguments are the route's query parameters, by the same names.
	for _, args := range []map[string]any{
		{"since": day(5).Format(time.RFC3339)},
		{"until": day(5).Format(time.RFC3339)},
		{"twin": "twin-2"},
		{"provider": "openai"},
		{"label": "org:o_2"},
		{"group_by": "provider"},
		{"group_by": "provider,day"},
		{"group_by": "provider,day", "limit": 1},
	} {
		want := f.restCost(t, args)
		if sameJSON(t, want, all) {
			t.Fatalf("GET /cost with %v answers as it does with nothing; the case tests no parameter", args)
		}
		if got := report(args); !sameJSON(t, got, want) {
			t.Errorf("cost_report(%v) = %v, want what GET /cost returns: %v", args, got, want)
		}
	}

	// limit, against the same grouping without it.
	if sameJSON(t, f.restCost(t, map[string]any{"group_by": "provider,day", "limit": 1}), f.restCost(t, map[string]any{"group_by": "provider,day"})) {
		t.Fatal("GET /cost answers the same with and without limit; the case tests no parameter")
	}

	// GET /cost answers 400 to a timestamp it cannot parse; here that is a tool
	// error rather than a report over a window nobody asked for.
	for _, key := range []string{"since", "until"} {
		if res := toolResult(t, f.call(t, "cost_report", map[string]any{key: "yesterday"})); !res.IsError {
			t.Errorf("cost_report %s=yesterday = %s, want a tool error", key, res.Text())
		}
	}
}

// A deployment recording no spend answers GET /cost with an empty array, and
// so does the tool.
func TestMCPCostReportWithNoLedgerIsEmpty(t *testing.T) {
	f := newMCPFixture(t, karakuriauth.ActionCostRead)
	res := toolResult(t, f.call(t, "cost_report", nil))
	if res.IsError || strings.Join(strings.Fields(res.Text()), "") != "[]" {
		t.Errorf("cost_report with no ledger = %+v, want []", res)
	}
}

// Reading checkpoints is offered; deciding them is not, whatever is held.
func TestMCPCheckpointToolsReadAndNeverResolve(t *testing.T) {
	f := newMCPFixture(t,
		karakuriauth.ActionObjectiveRead, karakuriauth.ActionReportRead, karakuriauth.ActionAuditRead,
		karakuriauth.ActionCostRead, karakuriauth.ActionCheckpointRead, karakuriauth.ActionCheckpointResolve,
	).withCheckpoints(t, corecheckpoint.Checkpoint{ID: "cp-1", ObjectiveID: "obj-1", TwinID: "twin-1"}).withCosts(t)

	_, resp := f.rpc(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	var list mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &list); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
		for _, verb := range []string{"resolve", "approve", "reject", "decide"} {
			if strings.Contains(tool.Name, verb) {
				t.Errorf("tools/list offers %q", tool.Name)
			}
		}
	}
	for _, want := range []string{"checkpoints_list", "checkpoint_read", "cost_report"} {
		if !names[want] {
			t.Errorf("%s missing from tools/list: %v", want, names)
		}
	}

	for _, name := range []string{"checkpoint_resolve", "checkpoint_approve", "checkpoint_reject"} {
		resp := f.call(t, name, map[string]any{"checkpoint_id": "cp-1"})
		if resp.Error == nil || !strings.Contains(resp.Error.Message, "stays with a person") {
			t.Errorf("%s: response = %+v, want a refusal naming why", name, resp)
		}
	}
	cp, err := f.h.Checkpoints.Get(context.Background(), "cp-1")
	if err != nil || cp.Status != corecheckpoint.StatusPending {
		t.Errorf("cp-1 = %+v (%v), want it still pending", cp, err)
	}
}
