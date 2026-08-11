package quota

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubGateway stands in for LiteLLM. Running a real one in CI would mean a
// Python service plus its Postgres for a path Karakuri only talks to over four
// documented fields; the shape of the reply is what matters here, and a
// release-time smoke against a live gateway is the thing that proves the rest.
func stubGateway(t *testing.T, status int, body string) (*LiteLLMBudget, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	b, err := NewLiteLLMBudget(srv.URL, "sk-test", 1000, srv.Client())
	if err != nil {
		t.Fatalf("NewLiteLLMBudget: %v", err)
	}
	return b, &seen
}

func TestLiteLLMBudgetReadsSpend(t *testing.T) {
	b, seen := stubGateway(t, http.StatusOK, `{"spend": 0.40, "max_budget": 1.00}`)

	dec, err := b.Usage(context.Background(), "t1", base)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if !dec.Allowed {
		t.Error("40 cents of a dollar should still be allowed")
	}
	// Reported in cents: the Decision counts whole units, and dollars would
	// lose everything below one.
	if dec.Limit != 100 || dec.Remaining != 60 {
		t.Errorf("limit/remaining = %d/%d, want 100/60", dec.Limit, dec.Remaining)
	}

	// The customer id has to match what the provider adapters stamp, or spend
	// is attributed to nobody.
	if got := (*seen)[0].URL.Query().Get("end_user_id"); got != CustomerID("t1") {
		t.Errorf("end_user_id = %q, want %q", got, CustomerID("t1"))
	}
	if got := (*seen)[0].Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestLiteLLMBudgetExhaustionMapsToTheSentinel(t *testing.T) {
	// The whole point of this implementation: the gateway refuses in its own
	// vocabulary, and the loop only understands one.
	b, _ := stubGateway(t, http.StatusOK, `{"spend": 1.50, "max_budget": 1.00}`)

	if err := b.Reserve(context.Background(), "t1", 0, base); !errors.Is(err, ErrBudgetExhausted) {
		t.Errorf("Reserve = %v, want ErrBudgetExhausted", err)
	}
}

func TestLiteLLMBudgetHonoursBlocked(t *testing.T) {
	// A customer can be blocked outright, independently of spend.
	b, _ := stubGateway(t, http.StatusOK, `{"spend": 0, "blocked": true}`)

	if err := b.Reserve(context.Background(), "t1", 0, base); !errors.Is(err, ErrBudgetExhausted) {
		t.Errorf("Reserve on a blocked customer = %v, want ErrBudgetExhausted", err)
	}
}

func TestLiteLLMBudgetTreatsAnUnknownCustomerAsUnspent(t *testing.T) {
	// Customers are upserted on first use, so a twin that has never made a
	// call has no record. That is not an error.
	b, _ := stubGateway(t, http.StatusNotFound, `{"error": "not found"}`)

	if err := b.Reserve(context.Background(), "fresh", 0, base); err != nil {
		t.Errorf("Reserve for an unknown customer = %v, want nil", err)
	}
}

func TestLiteLLMBudgetSurfacesGatewayFailures(t *testing.T) {
	// An unreachable gateway is "I could not find out", not "you are out of
	// budget" — the loop fails open on the former and pauses on the latter, so
	// confusing them would either stop healthy work or let spend run.
	b, _ := stubGateway(t, http.StatusInternalServerError, `boom`)

	err := b.Reserve(context.Background(), "t1", 0, base)
	if err == nil {
		t.Fatal("a 500 from the gateway was treated as success")
	}
	if errors.Is(err, ErrBudgetExhausted) {
		t.Error("a gateway failure was reported as an exhausted budget")
	}
}

func TestLiteLLMBudgetDoesNotDoubleCount(t *testing.T) {
	// The gateway saw the call and counted it. Sending our own token count
	// would charge twice, and ours is the less accurate number.
	b, seen := stubGateway(t, http.StatusOK, `{"spend": 0}`)

	if err := b.Record(context.Background(), "t1", 5000, base); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(*seen) != 0 {
		t.Errorf("Record made %d calls to the gateway, want none", len(*seen))
	}
}

func TestNewLiteLLMBudgetRequiresAURL(t *testing.T) {
	// Failing at boot beats discovering it on the first model call, when the
	// budget would fail open.
	if _, err := NewLiteLLMBudget("", "", 100, nil); err == nil {
		t.Error("accepted an empty gateway URL")
	}
}

func TestCustomerIDIsStable(t *testing.T) {
	// Two places construct this — here and internal/platform/llm — and a
	// disagreement would silently attribute spend to nobody.
	if got := CustomerID("t1"); got != "twin:t1" {
		t.Errorf("CustomerID = %q, want twin:t1", got)
	}
}
