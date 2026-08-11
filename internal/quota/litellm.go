package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// LiteLLMBudget delegates a twin's model spend to a LiteLLM gateway.
//
// The gateway counts dollars against a maintained per-model cost table, which
// is what most operators actually want to cap — tokens are a proxy for spend,
// and a bad one across models that differ by an order of magnitude in price.
//
// Karakuri stamps every model call with `x-litellm-customer-id: twin:<id>`
// (see internal/platform/llm), so the gateway attributes spend without anything
// having to be provisioned per twin: customers are upserted on first use.
//
// # What stays Karakuri's job
//
// The gateway refuses with an HTTP error. Roadmap step 6 wants the loop to
// pause on a checkpoint instead, so this maps the refusal onto the same
// ErrBudgetExhausted the native budget returns and the loop's behaviour is
// identical either way.
type LiteLLMBudget struct {
	baseURL string
	key     string
	client  *http.Client

	// cap mirrors the tier so Usage can report a limit even when the gateway
	// only tells us a spend. It is informational: the gateway is the authority
	// on whether a call proceeds.
	cap int
}

var _ TokenBudget = (*LiteLLMBudget)(nil)

// NewLiteLLMBudget returns a budget backed by the gateway at baseURL.
func NewLiteLLMBudget(baseURL, key string, cap int, client *http.Client) (*LiteLLMBudget, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("quota: litellm budget needs a gateway URL")
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("quota: litellm url: %w", err)
	}
	if client == nil {
		// A budget check must not outlive the request it is gating.
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &LiteLLMBudget{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		client:  client,
		cap:     cap,
	}, nil
}

// CustomerID is the identifier Karakuri gives a twin on the gateway. It is
// exported because the provider adapters have to stamp the same value on the
// header, and the two disagreeing would mean spend attributed to nobody.
func CustomerID(twinID string) string { return "twin:" + twinID }

// Reserve asks the gateway whether this twin has budget left.
//
// Unlike the native budget this is a real round trip, because the gateway is
// the only thing that knows the answer. It is one call per loop iteration, not
// per request.
func (b *LiteLLMBudget) Reserve(ctx context.Context, twinID string, _ int, now time.Time) error {
	dec, err := b.Usage(ctx, twinID, now)
	if err != nil {
		return err
	}
	if !dec.Allowed {
		return ErrBudgetExhausted
	}
	return nil
}

// Record is a no-op. The gateway saw the call and counted it; sending our own
// token count would double-count against a number it already has, and ours is
// the less accurate of the two.
func (b *LiteLLMBudget) Record(context.Context, string, int, time.Time) error { return nil }

// Usage reads the customer's spend from the gateway.
func (b *LiteLLMBudget) Usage(ctx context.Context, twinID string, _ time.Time) (quota.Decision, error) {
	endpoint := b.baseURL + "/customer/info?end_user_id=" + url.QueryEscape(CustomerID(twinID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return quota.Decision{}, err
	}
	if b.key != "" {
		req.Header.Set("Authorization", "Bearer "+b.key)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return quota.Decision{}, fmt.Errorf("quota: litellm: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// A twin that has never spent anything has no customer record yet —
		// the gateway upserts on first use. Nothing spent is not an error.
		return quota.Decision{Allowed: true, Limit: b.cap, Remaining: b.cap}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return quota.Decision{}, fmt.Errorf("quota: litellm: customer/info returned %s", resp.Status)
	}

	var info struct {
		Spend     float64  `json:"spend"`
		MaxBudget *float64 `json:"max_budget"`
		Blocked   bool     `json:"blocked"`
	}
	// The reply is bounded by http.MaxBytesReader rather than trusted: this is
	// another process's output, and a budget check should not be a way to make
	// this one allocate without limit.
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&info); err != nil {
		return quota.Decision{}, fmt.Errorf("quota: litellm: %w", err)
	}

	dec := quota.Decision{Limit: b.cap, Remaining: b.cap, Allowed: !info.Blocked}
	if info.MaxBudget != nil && *info.MaxBudget > 0 {
		// Spend is in dollars; the Decision counts whole units, so it is
		// reported in cents to keep some resolution without floats.
		limit := int(*info.MaxBudget * 100)
		spent := int(info.Spend * 100)
		dec.Limit = limit
		dec.Remaining = max(limit-spent, 0)
		if spent >= limit {
			dec.Allowed = false
		}
	}
	return dec.Normalize(), nil
}
