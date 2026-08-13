package auth_test

import (
	"context"
	"errors"
	"testing"

	extauth "github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

type stubContainers struct {
	labels map[string][]string
	err    error
	calls  int
}

func (s *stubContainers) Closure(_ context.Context, id string) ([]string, error) {
	return s.labels["container:"+id], s.err
}

func (s *stubContainers) ScopesOf(_ context.Context, resourceType, resourceID string) ([]string, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.labels[resourceType+":"+resourceID], nil
}

func filterFor(t *testing.T, grants extauth.ScopeGrants, containers karakuriauth.ScopeResolver) *karakuriauth.StreamFilter {
	t.Helper()
	f, err := karakuriauth.NewStreamFilter(context.Background(),
		&stubScopeAuthorizer{grants: grants}, containers, "olive")
	if err != nil {
		t.Fatalf("NewStreamFilter: %v", err)
	}
	return f
}

// The property the endpoint exists for: a team-scoped watcher sees their own
// tenant's events and none of the other's.
func TestStreamFilterConfinesToTheCallersTenant(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{
		"twin:mine":   {"team:t_acme_eng", "org:o_acme"},
		"twin:theirs": {"team:t_globex_eng", "org:o_globex"},
	}}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}}, containers)

	if !f.Allow(ctx, karakuriauth.Event{TwinID: "mine"}) {
		t.Error("an event about the caller's own twin was withheld")
	}
	if f.Allow(ctx, karakuriauth.Event{TwinID: "theirs"}) {
		t.Error("an event about the other tenant's twin was delivered")
	}
}

// Labels the event already carries are used as-is. A cost event copied them at
// write time, which makes them both cheaper and more accurate than a lookup:
// they say where the spend sat when it happened.
func TestStreamFilterUsesTheEventsOwnLabels(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{}}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"org:o_acme"}}, containers)

	if !f.Allow(ctx, karakuriauth.Event{TwinID: "t1", Labels: []string{"org:o_acme"}}) {
		t.Error("an event carrying a label the caller holds was withheld")
	}
	if f.Allow(ctx, karakuriauth.Event{TwinID: "t2", Labels: []string{"org:o_globex"}}) {
		t.Error("an event carrying another tenant's label was delivered")
	}
	if containers.calls != 0 {
		t.Errorf("the tree was consulted %d times for events that carried their own labels", containers.calls)
	}
}

// An unrestricted reader watches the whole deployment, which is what a global
// stream is for — and no lookup happens at all.
func TestStreamFilterAllowsAnUnrestrictedReader(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{err: errors.New("should not be consulted")}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"*"}}, containers)

	for _, e := range []karakuriauth.Event{
		{TwinID: "anything"},
		{Labels: []string{"org:o_globex"}},
		{Subject: "principal|someone-else"},
		{}, // unclassifiable
	} {
		if !f.Allow(ctx, e) {
			t.Errorf("an unrestricted reader was refused %+v", e)
		}
	}
}

// Quota pressure is the one event with no resource of its own. It names a
// principal, and that principal is the only scoped reader it concerns.
func TestStreamFilterDeliversAPrincipalsOwnPressure(t *testing.T) {
	ctx := context.Background()
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}}, &stubContainers{})

	if !f.Allow(ctx, karakuriauth.Event{Subject: "principal|olive"}) {
		t.Error("olive was not shown her own quota pressure")
	}
	if f.Allow(ctx, karakuriauth.Event{Subject: "principal|someone-else"}) {
		t.Error("olive was shown somebody else's quota pressure")
	}
}

// A pressure event keyed on a twin — twin|id|capability — is the twin case, and
// the trailing capability must not break the lookup.
func TestStreamFilterReadsATwinSubject(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{
		"twin:mine":   {"team:t_acme_eng"},
		"twin:theirs": {"team:t_globex_eng"},
	}}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}}, containers)

	if !f.Allow(ctx, karakuriauth.Event{Subject: "twin|mine|github.create_pr"}) {
		t.Error("a capability pressure event about the caller's twin was withheld")
	}
	if f.Allow(ctx, karakuriauth.Event{Subject: "twin|theirs|github.create_pr"}) {
		t.Error("a capability pressure event about another tenant's twin was delivered")
	}
}

// The default for anything the filter cannot classify is to withhold. Otherwise
// every event type added later leaks by default, which is exactly the failure a
// stream must not have.
func TestStreamFilterWithholdsWhatItCannotClassify(t *testing.T) {
	ctx := context.Background()
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}}, &stubContainers{})

	for _, e := range []karakuriauth.Event{
		{},
		{Subject: "something-unparseable"},
		{Subject: "session|abc"},
	} {
		if f.Allow(ctx, e) {
			t.Errorf("an unclassifiable event was delivered to a scoped reader: %+v", e)
		}
	}
}

// A tree that cannot be read withholds. This is the one place the container
// code fails closed rather than open, because the costs are asymmetric: a
// missing event is a gap in a dashboard, and an extra one is another tenant's
// activity.
func TestStreamFilterWithholdsWhenTheTreeCannotBeRead(t *testing.T) {
	ctx := context.Background()
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}},
		&stubContainers{err: errors.New("database down")})

	if f.Allow(ctx, karakuriauth.Event{TwinID: "mine"}) {
		t.Error("an event was delivered while the tenancy tree was unreadable")
	}
}

// A binding that names a twin outright carries no labels, so the ID has to be
// matched directly or such a reader would see nothing at all.
func TestStreamFilterAllowsATwinNamedByTheBinding(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{}}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"twin:mine"}}, containers)

	if !f.Allow(ctx, karakuriauth.Event{TwinID: "mine"}) {
		t.Error("an event about the twin the binding names was withheld")
	}
	if f.Allow(ctx, karakuriauth.Event{TwinID: "theirs"}) {
		t.Error("an event about an unrelated twin was delivered")
	}
}

// Deny wins here as everywhere else.
func TestStreamFilterHonoursADeny(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{
		"twin:sensitive": {"team:t_acme_hr", "org:o_acme"},
		"twin:ordinary":  {"team:t_acme_eng", "org:o_acme"},
	}}
	f := filterFor(t, extauth.ScopeGrants{
		Allow: []string{"org:o_acme"},
		Deny:  []string{"team:t_acme_hr"},
	}, containers)

	if !f.Allow(ctx, karakuriauth.Event{TwinID: "ordinary"}) {
		t.Error("an allowed twin's event was withheld")
	}
	if f.Allow(ctx, karakuriauth.Event{TwinID: "sensitive"}) {
		t.Error("a denied team's event was delivered")
	}
}

// A verdict is remembered, because a loop emits dozens of events about one twin
// and a container lookup each would put the tenancy tree on the path of every
// loop step in the system.
func TestStreamFilterRemembersItsVerdicts(t *testing.T) {
	ctx := context.Background()
	containers := &stubContainers{labels: map[string][]string{
		"twin:mine": {"team:t_acme_eng"},
	}}
	f := filterFor(t, extauth.ScopeGrants{Allow: []string{"team:t_acme_eng"}}, containers)

	for range 25 {
		if !f.Allow(ctx, karakuriauth.Event{TwinID: "mine"}) {
			t.Fatal("an allowed event was withheld")
		}
	}
	if containers.calls != 1 {
		t.Errorf("the tree was consulted %d times for one twin, want 1", containers.calls)
	}
}

// Grants that cannot be read are an error rather than an empty filter: a caller
// that silently withheld everything would look identical to a quiet system.
func TestNewStreamFilterReportsUnreadableGrants(t *testing.T) {
	_, err := karakuriauth.NewStreamFilter(context.Background(),
		&stubScopeAuthorizer{err: errors.New("database down")}, &stubContainers{}, "olive")
	if err == nil {
		t.Fatal("a filter was built from grants that could not be read")
	}
}

// A nil filter is what a caller gets from a zero value, and it must not be a
// way to see everything.
func TestNilStreamFilterWithholds(t *testing.T) {
	var f *karakuriauth.StreamFilter
	if f.Allow(context.Background(), karakuriauth.Event{TwinID: "anything"}) {
		t.Error("a nil filter delivered an event")
	}
}
