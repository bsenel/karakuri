package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
	"github.com/bsenel/karakuri/quota/cost"
	costsql "github.com/bsenel/karakuri/quota/cost/sql"
)

// createScopedUser adds a principal whose binding is confined to one scope, and
// returns its access token. It is createUser with the scope the tenancy tests
// need — "which twins" is the whole subject of the cases below.
func createScopedUser(t *testing.T, baseURL, adminToken, id, role, scope string) string {
	t.Helper()
	const password = "correct-horse-battery-staple"
	resp := doJSON(t, adminToken, http.MethodPost, baseURL+"/api/v1/auth/users", map[string]any{
		"id": id, "name": id, "roles": []string{role}, "scope": scope, "password": password,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create %q: %d %s", id, resp.StatusCode, body)
	}
	return login(t, baseURL, id, password)
}

// submitRaise asks for a bigger llm-tokens allowance for one twin and returns
// the request ID.
func submitRaise(t *testing.T, baseURL, token, twinID string, cap int) string {
	t.Helper()
	resp := doJSON(t, token, http.MethodPost, baseURL+"/api/v1/quota/requests", map[string]any{
		"tier": "llm-tokens", "twin": twinID, "cap": cap, "reason": "launch week",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("submit for twin %s: %d %s", twinID, resp.StatusCode, body)
	}
	var req struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req.ID == "" {
		t.Fatal("request came back with no id")
	}
	if req.Status != "pending" {
		t.Fatalf("submitted request is %q, want pending", req.Status)
	}
	return req.ID
}

func decide(t *testing.T, baseURL, token, id string, approve bool) *http.Response {
	t.Helper()
	return doJSON(t, token, http.MethodPost,
		baseURL+"/api/v1/quota/requests/"+id+"/decide",
		map[string]any{"approve": approve, "note": "reviewed"})
}

// llmTokenLimit reads the limit currently in force for a twin, which is what an
// approved raise has to move.
func llmTokenLimit(t *testing.T, baseURL, token, twinID string) float64 {
	t.Helper()
	resp := doJSON(t, token, http.MethodGet, baseURL+"/api/v1/quota/usage?twin="+twinID, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Tiers map[string]struct {
			Limit float64 `json:"limit"`
		} `json:"tiers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	return body.Tiers["llm_tokens"].Limit
}

// The self-service half of Phase 18, and the property that keeps it from being
// a way around tenancy: approving a raise requires holding the subject it
// raises. Without it, the permission to approve is the permission to raise
// anybody's limit — including in an organisation the approver has no claim on.
func TestQuotaApprovalIsConfinedToTheApproversTenant(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)

	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
		cfg.Quota.LLMTokensPerDay = 1000
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	// Rob may ask for anything; asking is not granting, which is why every role
	// down to viewer holds quota:request.
	robToken := createUser(t, baseURL, adminToken, "rob", karakuriauth.RoleViewer)
	acmeReq := submitRaise(t, baseURL, robToken, acmeTwin, 5000)
	globexReq := submitRaise(t, baseURL, robToken, globexTwin, 5000)

	// Ann administers acme's engineering team and nothing else.
	annToken := createScopedUser(t, baseURL, adminToken, "ann",
		karakuriauth.RoleAdmin, acmeEng.Label())

	// A viewer cannot decide at all — the route gate refuses before any subject
	// is looked at.
	resp := decide(t, baseURL, robToken, acmeReq, true)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Ann can decide, but not for the other tenant. This is the case the whole
	// check exists for: she holds quota:approve, and the subject is somebody
	// else's twin.
	resp = decide(t, baseURL, annToken, globexReq, true)
	assertStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	// Her own tenant's request goes through.
	resp = decide(t, baseURL, annToken, acmeReq, true)
	assertStatus(t, resp, http.StatusOK)
	var decided struct {
		Status    string `json:"status"`
		DecidedBy string `json:"decided_by"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decided); err != nil {
		t.Fatalf("decode decision: %v", err)
	}
	resp.Body.Close()
	if decided.Status != "approved" || decided.DecidedBy != "ann" {
		t.Fatalf("decision = %+v, want approved by ann", decided)
	}

	// The acceptance criterion: the approval changed the limit that is actually
	// enforced, not just a row in a table. The resolver is invalidated on
	// approval, so this holds immediately rather than a cache TTL later.
	if got := llmTokenLimit(t, baseURL, adminToken, acmeTwin); got != 5000 {
		t.Errorf("acme twin's llm-tokens limit = %v, want the approved 5000", got)
	}
	// And the refused request moved nothing.
	if got := llmTokenLimit(t, baseURL, adminToken, globexTwin); got != 1000 {
		t.Errorf("globex twin's limit = %v, want the configured 1000", got)
	}

	// Deciding twice is refused rather than ignored, so a duplicate approval
	// cannot silently do nothing while the approver believes it did something.
	resp = decide(t, baseURL, annToken, acmeReq, true)
	assertStatus(t, resp, http.StatusConflict)
	resp.Body.Close()

	// Rejecting is not gated on holding the subject: somebody who may decide at
	// all may always decline.
	resp = decide(t, baseURL, annToken, globexReq, false)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

// The listing is narrowed the same way the twin listing is: you see your own
// requests, and the ones you could approve. Otherwise quota:read — which every
// role holds — is a window into what every other tenant is asking for and why.
func TestQuotaRequestListingIsNarrowedToWhatTheCallerMaySee(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)

	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	robToken := createUser(t, baseURL, adminToken, "rob", karakuriauth.RoleViewer)
	acmeReq := submitRaise(t, baseURL, robToken, acmeTwin, 5000)
	submitRaise(t, baseURL, robToken, globexTwin, 5000)

	// Gwen may read quotas but administers nothing, and asked for nothing.
	gwenToken := createUser(t, baseURL, adminToken, "gwen", karakuriauth.RoleViewer)
	annToken := createScopedUser(t, baseURL, adminToken, "ann",
		karakuriauth.RoleAdmin, acmeEng.Label())

	list := func(token string) []string {
		t.Helper()
		resp := doJSON(t, token, http.MethodGet, baseURL+"/api/v1/quota/requests", nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
		var rows []struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
			t.Fatalf("decode requests: %v", err)
		}
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.ID)
		}
		return out
	}

	if got := list(gwenToken); len(got) != 0 {
		t.Errorf("gwen sees %v — she neither asked for these nor may approve them", got)
	}
	if got := list(robToken); len(got) != 2 {
		t.Errorf("rob sees %v, want both of his own requests", got)
	}
	if got := list(annToken); len(got) != 1 || got[0] != acmeReq {
		t.Errorf("ann sees %v, want only acme's %s", got, acmeReq)
	}
	if got := list(adminToken); len(got) != 2 {
		t.Errorf("admin sees %v, want everything — the wildcard is not a narrowing", got)
	}
}

// recordSpend writes cost events straight to the ledger the server reads.
//
// The alternative is running a loop, which needs a model provider; what is
// under test here is the report's tenancy filter, and the ledger is the same
// storage either path writes to.
func recordSpend(t *testing.T, dbPath string, events ...cost.Event) {
	t.Helper()
	gdb, err := platformdb.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	defer sqlDB.Close()

	ledger, err := costsql.New(sqlDB, costsql.Options{Dialect: costsql.SQLite})
	if err != nil {
		t.Fatalf("ledger: %v", err)
	}
	ctx := context.Background()
	if err := ledger.Migrate(ctx); err != nil {
		t.Fatalf("migrate ledger: %v", err)
	}
	for _, e := range events {
		if err := ledger.Record(ctx, e); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
}

func costReport(t *testing.T, baseURL, token, query string) []cost.Bucket {
	t.Helper()
	resp := doJSON(t, token, http.MethodGet, baseURL+"/api/v1/cost"+query, nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	var buckets []cost.Bucket
	if err := json.NewDecoder(resp.Body).Decode(&buckets); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return buckets
}

func total(buckets []cost.Bucket) float64 {
	var sum float64
	for _, b := range buckets {
		sum += b.Cost
	}
	return sum
}

// Phase 17's property, restated for money: two organisations, spend in each,
// and a report from one that contains neither the other's rows nor its totals.
//
// A per-resource check that stops somebody reading another tenant's twin is not
// isolation while a report totals that twin's spend.
func TestCostReportIsConfinedToTheCallersTenant(t *testing.T) {
	dbPath, svc, acmeEng, globexEng := seedTenancy(t)

	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	globexTwin := createTwin(t, baseURL, adminToken, "globex twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	if err := svc.SetResourceContainers(ctx, "twin", globexTwin, []string{globexEng.ID}); err != nil {
		t.Fatalf("place globex twin: %v", err)
	}

	// The labels an event carries are the ones the resource sat in when the
	// spend happened, which is what the recorder copies from the tree.
	acmeLabels, err := svc.ScopesOf(ctx, "twin", acmeTwin)
	if err != nil {
		t.Fatalf("acme labels: %v", err)
	}
	globexLabels, err := svc.ScopesOf(ctx, "twin", globexTwin)
	if err != nil {
		t.Fatalf("globex labels: %v", err)
	}

	now := time.Now().UTC()
	recordSpend(t, dbPath,
		cost.Event{
			Subject: karakuriquota.CostSubject(acmeTwin), ResourceType: "twin", ResourceID: acmeTwin,
			Provider: "anthropic", Model: "opus", Units: 1000, UnitKind: cost.UnitTokens,
			Cost: 3, OccurredAt: now, Labels: acmeLabels,
		},
		cost.Event{
			Subject: karakuriquota.CostSubject(globexTwin), ResourceType: "twin", ResourceID: globexTwin,
			Provider: "anthropic", Model: "opus", Units: 1000, UnitKind: cost.UnitTokens,
			Cost: 7, OccurredAt: now, Labels: globexLabels,
		},
	)

	// Olive is an operator on acme's engineering team. Operators hold cost:read.
	oliveToken := createScopedUser(t, baseURL, adminToken, "olive",
		karakuriauth.RoleOperator, acmeEng.Label())

	got := costReport(t, baseURL, oliveToken, "?group_by=label")
	if len(got) == 0 {
		t.Fatal("olive's report is empty — she should see her own team's spend")
	}
	for _, b := range got {
		if len(b.Key) != 1 {
			t.Fatalf("bucket %+v has no label key", b)
		}
		for _, label := range globexLabels {
			if b.Key[0] == label {
				t.Errorf("olive's report contains %q, which is the other tenant's", label)
			}
		}
	}
	// Not just the rows: the money. A filter that hid the rows and kept the
	// totals would be the same leak wearing a hat.
	if sum := total(got); sum != 3 {
		t.Errorf("olive's total = %v, want 3 — globex's 7 must not be in it", sum)
	}

	// Naming the other tenant's team narrows to nothing rather than widening to
	// their spend, which is the difference between a filter and a query.
	if got := costReport(t, baseURL, oliveToken, "?label="+globexEng.Label()); len(got) != 0 {
		t.Errorf("asking for globex's team returned %+v", got)
	}

	// An unrestricted reader still totals everything, so the filter did not
	// become a second denial path.
	if sum := total(costReport(t, baseURL, adminToken, "")); sum != 10 {
		t.Errorf("admin total = %v, want both tenants' 10", sum)
	}
}

// A twin in no container belongs to no tenant, and a scope-bound reader does
// not see its spend — the same answer the twin listing gives.
func TestCostReportHidesUncontainedSpendFromScopedReaders(t *testing.T) {
	dbPath, svc, acmeEng, _ := seedTenancy(t)

	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	ctx := context.Background()
	looseTwin := createTwin(t, baseURL, adminToken, "twin in no container")
	acmeTwin := createTwin(t, baseURL, adminToken, "acme twin")
	if err := svc.SetResourceContainers(ctx, "twin", acmeTwin, []string{acmeEng.ID}); err != nil {
		t.Fatalf("place acme twin: %v", err)
	}
	acmeLabels, err := svc.ScopesOf(ctx, "twin", acmeTwin)
	if err != nil {
		t.Fatalf("acme labels: %v", err)
	}

	now := time.Now().UTC()
	recordSpend(t, dbPath,
		cost.Event{
			Subject: karakuriquota.CostSubject(looseTwin), ResourceType: "twin", ResourceID: looseTwin,
			Provider: "openai", Units: 10, UnitKind: cost.UnitCalls, Cost: 4, OccurredAt: now,
		},
		cost.Event{
			Subject: karakuriquota.CostSubject(acmeTwin), ResourceType: "twin", ResourceID: acmeTwin,
			Provider: "openai", Units: 10, UnitKind: cost.UnitCalls, Cost: 6,
			OccurredAt: now, Labels: acmeLabels,
		},
	)

	oliveToken := createScopedUser(t, baseURL, adminToken, "olive",
		karakuriauth.RoleOperator, acmeEng.Label())
	if sum := total(costReport(t, baseURL, oliveToken, "")); sum != 6 {
		t.Errorf("olive's total = %v, want only her team's 6", sum)
	}
	if sum := total(costReport(t, baseURL, adminToken, "")); sum != 10 {
		t.Errorf("admin total = %v, want everything", sum)
	}
}

// A reader whose grant names individual twins is answered by naming those
// twins, rather than by a label filter it has nothing to fill in with. Without
// this branch such a reader would see either nothing or everything.
func TestCostReportAnswersATwinScopedReaderExactly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cost.db")
	baseURL, adminToken, cleanup := startServerWith(t, func(cfg *config.Config) {
		cfg.Database.DSN = dbPath
	})
	defer cleanup()

	mine := createTwin(t, baseURL, adminToken, "mine")
	theirs := createTwin(t, baseURL, adminToken, "theirs")

	now := time.Now().UTC()
	recordSpend(t, dbPath,
		cost.Event{
			Subject: karakuriquota.CostSubject(mine), ResourceType: "twin", ResourceID: mine,
			Provider: "openai", Units: 1, UnitKind: cost.UnitCalls, Cost: 2, OccurredAt: now,
		},
		cost.Event{
			Subject: karakuriquota.CostSubject(theirs), ResourceType: "twin", ResourceID: theirs,
			Provider: "openai", Units: 1, UnitKind: cost.UnitCalls, Cost: 5, OccurredAt: now,
		},
	)

	scoutToken := createScopedUser(t, baseURL, adminToken, "scout",
		karakuriauth.RoleOperator, "twin:"+mine)
	if sum := total(costReport(t, baseURL, scoutToken, "")); sum != 2 {
		t.Errorf("scout's total = %v, want only the twin its binding names", sum)
	}
}
