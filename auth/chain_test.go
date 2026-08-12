package auth_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bsenel/karakuri/auth"
)

func stubResolver(id string, err error, calls *int) auth.TokenResolver {
	return auth.ResolverFunc(func(*http.Request) (auth.Principal, error) {
		if calls != nil {
			*calls++
		}
		if err != nil {
			return auth.Principal{}, err
		}
		return auth.Principal{ID: id}, nil
	})
}

func TestChainResolverFirstSuccessWins(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	var second int
	chain := auth.ChainResolver{
		stubResolver("local", nil, nil),
		stubResolver("federated", nil, &second),
	}
	p, err := chain.Resolve(r)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ID != "local" {
		t.Errorf("principal = %q, want the first resolver's", p.ID)
	}
	if second != 0 {
		t.Errorf("later resolver ran %d times, want 0", second)
	}
}

// The whole reason this type exists: an identity-provider token is a bearer
// token, so the local resolver sees it first and fails to verify it. If the
// chain stopped there, federated authentication would never work.
func TestChainResolverContinuesPastVerificationFailure(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	chain := auth.ChainResolver{
		stubResolver("", errors.New("jwt: signature is not valid"), nil),
		stubResolver("federated", nil, nil),
	}
	p, err := chain.Resolve(r)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ID != "federated" {
		t.Errorf("principal = %q, want the second resolver's", p.ID)
	}
}

func TestChainResolverReportsTheFirstRealError(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	expired := errors.New("jwt: token has expired")

	// A resolver with no credential to offer must not mask the resolver that
	// actually looked at one and found it wanting.
	chain := auth.ChainResolver{
		stubResolver("", auth.ErrNoCredential, nil),
		stubResolver("", expired, nil),
		stubResolver("", errors.New("second failure"), nil),
	}
	if _, err := chain.Resolve(r); !errors.Is(err, expired) {
		t.Fatalf("err = %v, want the first substantive failure", err)
	}
}

func TestChainResolverNoCredential(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	chain := auth.ChainResolver{
		stubResolver("", auth.ErrNoCredential, nil),
		stubResolver("", auth.ErrNoCredential, nil),
	}
	if _, err := chain.Resolve(r); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("err = %v, want ErrNoCredential", err)
	}

	if _, err := (auth.ChainResolver{}).Resolve(r); !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("empty chain err = %v, want ErrNoCredential", err)
	}
}

// A nil entry is what an integration shim produces when a provider is not
// configured, so skipping them keeps the caller from having to build the slice
// conditionally.
func TestChainResolverSkipsNilEntries(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	chain := auth.ChainResolver{nil, stubResolver("local", nil, nil), nil}
	p, err := chain.Resolve(r)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ID != "local" {
		t.Errorf("principal = %q", p.ID)
	}
}

// The chain has to work as the argument to Authenticate, which is the only
// place Karakuri installs it.
func TestChainResolverThroughAuthenticate(t *testing.T) {
	t.Parallel()

	chain := auth.ChainResolver{
		stubResolver("", auth.ErrNoCredential, nil),
		stubResolver("oidc:alice", nil, nil),
	}
	var seen string
	handler := auth.Authenticate(chain)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		p, _ := auth.PrincipalFromContext(r.Context())
		seen = p.ID
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen != "oidc:alice" {
		t.Errorf("principal in context = %q, want oidc:alice", seen)
	}
}
