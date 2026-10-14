package delegation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/auth/jwt"
	internalauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

const (
	ownTwin   = "twin-own"
	otherTwin = "twin-other"
)

type recorder struct{ events []storage.ToolEvent }

func (r *recorder) SaveToolEvent(_ context.Context, e storage.ToolEvent) error {
	r.events = append(r.events, e)
	return nil
}

// harness wires the issuer to the real store, the real authorizer and the real
// token validation the API uses. Only the audit sink is a recorder.
type harness struct {
	store  *auth.MemoryStore
	tokens *auth.TokenService
	authz  *auth.StoreAuthorizer
	issuer *Issuer
	events *recorder
	now    time.Time
	home   string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{now: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), home: t.TempDir()}
	// The operator's home is an empty temp dir: anything the issuer reads from
	// it is absent, and anything it writes there is caught by the disk test.
	t.Setenv("HOME", h.home)
	t.Setenv("XDG_CONFIG_HOME", h.home)

	h.store = auth.NewMemoryStore()
	for _, r := range internalauth.BuiltinRoles() {
		if err := h.store.PutRole(context.Background(), r); err != nil {
			t.Fatalf("seed role %q: %v", r.Name, err)
		}
	}
	key, err := jwt.NewHMACKey("k1", []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	keys, err := jwt.NewKeyring(key)
	if err != nil {
		t.Fatalf("keyring: %v", err)
	}
	cfg := auth.TokenConfig{Issuer: "karakuri", Audience: "karakuri-api"}
	clock := func() time.Time { return h.now }
	h.tokens, err = auth.NewTokenService(h.store, h.store, keys, cfg)
	if err != nil {
		t.Fatalf("token service: %v", err)
	}
	h.tokens.WithClock(clock)
	h.authz = auth.NewAuthorizer(h.store)
	h.events = &recorder{}
	h.issuer = NewIssuer(h.store, keys, cfg, h.events).WithClock(clock)
	return h
}

func (h *harness) issue(t *testing.T, timeout time.Duration) Credential {
	t.Helper()
	c, err := h.issuer.Issue(context.Background(), ownTwin, timeout)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if c.Token == "" || c.PrincipalID == "" {
		t.Fatalf("Issue returned an empty credential: %+v", c)
	}
	return c
}

func (h *harness) verify(t *testing.T, c Credential) auth.Principal {
	t.Helper()
	p, err := h.tokens.Verify(context.Background(), c.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return p
}

func (h *harness) decide(t *testing.T, p auth.Principal, a auth.Action, res auth.ResourceRef) auth.Decision {
	t.Helper()
	d, err := h.authz.Authorize(context.Background(), p, a, res)
	if err != nil {
		t.Fatalf("Authorize %s on %s: %v", a, res, err)
	}
	return d
}

func inTwin(typ, id, twin string) auth.ResourceRef {
	return auth.Resource(typ, id).WithScopes(auth.ScopeLabel("twin", twin))
}

func TestIssuedTokenReadsItsOwnTwinOnly(t *testing.T) {
	h := newHarness(t)
	c := h.issue(t, 10*time.Minute)
	p := h.verify(t, c)

	if p.ID != c.PrincipalID || p.Kind != auth.KindService {
		t.Fatalf("token authenticates as %+v, want service principal %q", p, c.PrincipalID)
	}
	for _, a := range []auth.Action{internalauth.ActionObjectiveRead, internalauth.ActionCheckpointRead} {
		if d := h.decide(t, p, a, inTwin("objective", "o1", ownTwin)); !d.Allowed {
			t.Errorf("%s on own twin refused: %s", a, d.Reason)
		}
	}
	d := h.decide(t, p, internalauth.ActionObjectiveRead, inTwin("objective", "o2", otherTwin))
	if d.Allowed {
		t.Fatalf("objective read on another twin allowed via %q scope %q", d.ViaRole, d.BindingScope)
	}
	// No binding reaches the other twin at all, so no role is in play.
	if len(d.ConsideredRoles) != 0 {
		t.Errorf("considered roles on another twin = %v, want none", d.ConsideredRoles)
	}
	if d := h.decide(t, p, internalauth.ActionObjectiveRead, auth.Collection("objective")); d.Allowed {
		t.Errorf("unscoped objective read allowed via %q scope %q", d.ViaRole, d.BindingScope)
	}
}

func TestIssuedPrincipalIsRefusedEveryMutation(t *testing.T) {
	h := newHarness(t)
	p := h.verify(t, h.issue(t, 10*time.Minute))

	for _, a := range []auth.Action{
		internalauth.ActionCheckpointResolve,
		internalauth.ActionObjectiveUpdate,
		internalauth.ActionObjectiveCreate,
		internalauth.ActionObjectiveDeclare,
		internalauth.ActionObjectiveReconcile,
		internalauth.ActionObjectivePause,
	} {
		d := h.decide(t, p, a, inTwin("objective", "o1", ownTwin))
		if d.Allowed {
			t.Errorf("%s on own twin allowed via %q (%s)", a, d.ViaRole, d.MatchedPolicy)
		}
	}
}

func TestBindingsAreReadOnlyAndTwinScoped(t *testing.T) {
	h := newHarness(t)
	c := h.issue(t, 10*time.Minute)

	bs, err := h.store.ListBindings(context.Background(), c.PrincipalID)
	if err != nil {
		t.Fatalf("ListBindings: %v", err)
	}
	if len(bs) == 0 {
		t.Fatal("issued principal holds no binding")
	}
	for _, b := range bs {
		if b.Scope != auth.ScopeLabel("twin", ownTwin) {
			t.Errorf("binding %q scope = %q, want %q", b.ID, b.Scope, auth.ScopeLabel("twin", ownTwin))
		}
		if b.Role != internalauth.RoleViewer && b.Role != internalauth.RoleAuditor {
			t.Errorf("binding %q role = %q, want a read-only built-in role", b.ID, b.Role)
		}
	}
}

func TestTokenExpiresWithTheActionTimeout(t *testing.T) {
	h := newHarness(t)
	timeout := 90 * time.Second
	issuedAt := h.now
	c := h.issue(t, timeout)

	if c.ExpiresAt.After(issuedAt.Add(timeout)) {
		t.Fatalf("ExpiresAt = %s, later than now+timeout %s", c.ExpiresAt, issuedAt.Add(timeout))
	}
	if !c.ExpiresAt.After(issuedAt) {
		t.Fatalf("ExpiresAt = %s, not after issue time %s", c.ExpiresAt, issuedAt)
	}
	h.verify(t, c)

	// Validation allows jwt.DefaultLeeway of clock skew past exp.
	h.now = issuedAt.Add(timeout + jwt.DefaultLeeway + time.Second)
	if _, err := h.tokens.Verify(context.Background(), c.Token); !errors.Is(err, jwt.ErrExpired) {
		t.Fatalf("Verify past expiry = %v, want jwt.ErrExpired", err)
	}
}

func TestIssueRefusesANonPositiveTimeout(t *testing.T) {
	h := newHarness(t)
	for _, d := range []time.Duration{0, -time.Second} {
		c, err := h.issuer.Issue(context.Background(), ownTwin, d)
		if err == nil || errors.Is(err, ErrNotImplemented) {
			t.Errorf("Issue(timeout=%s) = %+v, %v; want a refusal", d, c, err)
		}
	}
	if _, err := h.issuer.Issue(context.Background(), "", time.Minute); err == nil || errors.Is(err, ErrNotImplemented) {
		t.Errorf("Issue with no twin = %v; want a refusal", err)
	}
}

func TestRevokeRefusesTheTokenAndLeavesNothingBehind(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	before, err := h.store.ListPrincipals(ctx)
	if err != nil {
		t.Fatalf("ListPrincipals: %v", err)
	}
	c := h.issue(t, 10*time.Minute)
	p := h.verify(t, c)

	if err := h.issuer.Revoke(ctx, c); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := h.tokens.Verify(ctx, c.Token); !errors.Is(err, auth.ErrPrincipalNotFound) {
		t.Fatalf("Verify after revoke = %v, want auth.ErrPrincipalNotFound", err)
	}
	// A caller still holding the principal value gets nothing from it either.
	if d := h.decide(t, p, internalauth.ActionObjectiveRead, inTwin("objective", "o1", ownTwin)); d.Allowed {
		t.Errorf("objective read allowed after revoke via %q", d.ViaRole)
	}
	if bs, err := h.store.ListBindings(ctx, c.PrincipalID); err != nil || len(bs) != 0 {
		t.Errorf("bindings after revoke = %v, %v; want none", bs, err)
	}
	after, err := h.store.ListPrincipals(ctx)
	if err != nil {
		t.Fatalf("ListPrincipals: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("principals after revoke = %d, want %d", len(after), len(before))
	}
	if _, err := h.store.GetCredential(ctx, c.PrincipalID); err == nil {
		t.Error("a stored credential outlived revoke")
	}
}

func TestIssueTouchesNoDiskAndStoresNoRefreshToken(t *testing.T) {
	h := newHarness(t)
	c := h.issue(t, 10*time.Minute)

	entries, err := os.ReadDir(h.home)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("issue wrote %d entries under the home directory, first %q", len(entries), entries[0].Name())
	}
	// A delegation holds an access token only: nothing that could be exchanged
	// for a longer life after the action ends.
	if _, err := h.store.GetCredential(context.Background(), c.PrincipalID); err == nil {
		t.Error("issue stored a password credential for the delegation principal")
	}
	if _, err := h.tokens.IssueForRefresh(context.Background(), c.Token); err == nil {
		t.Error("the delegation token was accepted as a refresh token")
	}
}

func TestIssueAuditsTheScopeAndNotTheToken(t *testing.T) {
	h := newHarness(t)
	c := h.issue(t, 10*time.Minute)

	if len(h.events.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(h.events.events))
	}
	e := h.events.events[0]
	if string(e.Kind) != EventKindIssued {
		t.Errorf("event kind = %q, want %q", e.Kind, EventKindIssued)
	}
	if e.AgentID != c.PrincipalID {
		t.Errorf("event agent = %q, want the delegation principal %q", e.AgentID, c.PrincipalID)
	}
	for _, want := range []string{ownTwin, "read_only", c.ExpiresAt.UTC().Format(time.RFC3339)} {
		if !strings.Contains(e.PayloadJSON, want) {
			t.Errorf("event payload %s does not carry %q", e.PayloadJSON, want)
		}
	}
	all := strings.Join([]string{e.ID, e.AgentID, e.Capability, string(e.Kind), e.EscalationReason, e.PayloadJSON}, "\n")
	if strings.Contains(all, c.Token) {
		t.Error("the recorded event contains the token")
	}
	// Not even a recognisable piece of it: the signature is the secret part.
	if sig := c.Token[strings.LastIndex(c.Token, ".")+1:]; sig != "" && strings.Contains(all, sig) {
		t.Error("the recorded event contains the token's signature")
	}
}
