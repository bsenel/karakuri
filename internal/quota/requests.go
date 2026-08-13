package quota

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/quota"
)

// Self-service quota requests, in Karakuri's terms.
//
// The workflow lives in the quota module; what lives here is which limits can
// be asked for and how a subject is named — the two things that module
// deliberately does not know.

// Tier names a limit a request can target.
//
// They are the names an operator types, and they have to match what the
// resolver looks an override up by — which is why they are constants rather
// than free text on both sides.
const (
	TierRequest    = "request"
	TierCapability = "capability"
	TierLLMTokens  = "llm-tokens"
	TierAdapter    = "adapter"
)

// Tiers a request may name. A request for anything else is refused at
// submission rather than accepted and then silently ignored, which is what
// would happen if an override were written under a name nothing resolves.
var requestableTiers = map[string]bool{
	TierRequest:    true,
	TierCapability: true,
	TierLLMTokens:  true,
	TierAdapter:    true,
}

// RequestableTiers lists what may be asked for, sorted, for an error message
// and for the CLI's help text.
func RequestableTiers() []string {
	return []string{TierAdapter, TierCapability, TierLLMTokens, TierRequest}
}

// SubjectFor builds the key a tier is counted under.
//
// The request tier is per principal and the rest are per twin, which mirrors
// exactly how the limits are enforced: RequestKey throttles whoever is making
// requests, and the twin quotas bound a twin's work. Getting this wrong would
// write an override nothing ever reads.
func SubjectFor(tier, principalID, twinID string) (quota.Key, error) {
	switch tier {
	case TierRequest:
		if principalID == "" {
			return "", fmt.Errorf("a %s override needs a principal", tier)
		}
		return quota.JoinKey("principal", principalID), nil
	case TierCapability, TierLLMTokens, TierAdapter:
		if twinID == "" {
			return "", fmt.Errorf("a %s override needs a twin", tier)
		}
		return TwinKey(twinID), nil
	default:
		return "", fmt.Errorf("unknown tier %q; one of %s", tier, strings.Join(RequestableTiers(), ", "))
	}
}

// SubmitRequest records a request for more of a tier.
type SubmitRequest struct {
	Tier        string
	PrincipalID string
	TwinID      string
	Cap         int
	Window      time.Duration
	ExpiresAt   time.Time
	Reason      string
	RequestedBy string
}

// Submit records a quota-increase request.
//
// The ID and the timestamp are minted here rather than in the module, which
// takes neither a clock nor an identifier generator — both are the host's, and
// keeping them out is what makes the module's tests deterministic.
func (d Deps) Submit(ctx context.Context, req SubmitRequest) (quota.Request, error) {
	if !requestableTiers[req.Tier] {
		return quota.Request{}, fmt.Errorf("unknown tier %q; one of %s",
			req.Tier, strings.Join(RequestableTiers(), ", "))
	}
	subject, err := SubjectFor(req.Tier, req.PrincipalID, req.TwinID)
	if err != nil {
		return quota.Request{}, err
	}
	return d.Requests().Submit(ctx, quota.Request{
		ID:          newRequestID(),
		Subject:     subject,
		Name:        req.Tier,
		Cap:         req.Cap,
		Window:      req.Window,
		ExpiresAt:   req.ExpiresAt,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
		CreatedAt:   time.Now().UTC(),
	})
}

// Decide approves or rejects a request. Whether the caller may is decided
// before this is reached — see internal/api/handler.QuotaHandler.
func (d Deps) Decide(ctx context.Context, id, by, note string, approved bool) (quota.Request, error) {
	return d.Requests().Decide(ctx, id, quota.Decisions{
		By: by, Note: note, At: time.Now().UTC(), Approved: approved,
	})
}

// ListRequests returns matching requests.
func (d Deps) ListRequests(ctx context.Context, f quota.RequestFilter) ([]quota.Request, error) {
	return d.Requests().List(ctx, f)
}

// Requests assembles the workflow over whatever stores are configured.
func (d Deps) Requests() quota.Requests {
	return quota.Requests{Store: d.RequestStore, Overrides: d.OverrideStore, Resolver: d.Resolver}
}

// newRequestID mints an identifier. Random rather than sequential: a request ID
// appears in URLs and approval links, and a guessable one invites somebody to
// try approving the next number up.
func newRequestID() string {
	b := make([]byte, 8)
	// crypto/rand.Read cannot fail on any platform this runs on.
	_, _ = rand.Read(b)
	return "qr_" + hex.EncodeToString(b)
}
