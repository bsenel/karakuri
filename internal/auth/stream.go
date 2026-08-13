package auth

import (
	"context"
	"strings"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Filtering a stream is not filtering a list.
//
// A listing asks the database one question and gets rows back already narrowed.
// A stream is handed events one at a time by a hub that knows nothing about who
// is watching, so the narrowing has to happen per event — and a stream is the
// one place where getting it wrong leaks continuously rather than once.
//
// Two things follow. The selectors are read once, when the subscription opens,
// because re-reading a principal's grants per event would put a database read
// on the path of every loop step in the system. And the answer for a resource
// is remembered, because a busy loop emits dozens of events about the same twin.
//
// Reading grants once means a binding revoked mid-stream is still honoured until
// the client reconnects. That is the same staleness the quota resolver accepts
// for the same reason, and it is bounded by the life of one HTTP connection.

// StreamFilter decides which events one subscriber may see.
//
// The zero value delivers nothing, which is the right default for a type whose
// job is to withhold. Use NewStreamFilter.
type StreamFilter struct {
	principalID string
	containers  ScopeResolver

	// unrestricted short-circuits everything below it. An administrator holding
	// "*" watches the whole deployment, which is what a global stream is for.
	unrestricted bool

	visible *storage.ScopeSelector
	hidden  storage.ScopeSelector

	// decided remembers the verdict for a resource so a loop emitting fifty
	// events about one twin costs one container lookup rather than fifty.
	decided map[string]bool
}

// NewStreamFilter reads the caller's grants once and returns the filter to test
// each event against.
//
// It is built from the same action and the same selectors the twin listing uses,
// deliberately: a stream that showed events about twins the caller cannot list
// would be a second, looser answer to a question already settled.
func NewStreamFilter(ctx context.Context, a ScopeAuthorizer, containers ScopeResolver, principalID string) (*StreamFilter, error) {
	f := &StreamFilter{principalID: principalID, containers: containers, decided: map[string]bool{}}
	if a == nil || principalID == "" {
		// An internal caller rather than an anonymous one — every route here is
		// authenticated before a handler runs. Withholding everything would
		// silently empty the stream instead of failing loudly.
		f.unrestricted = true
		return f, nil
	}
	visible, hidden, err := ListFor(ctx, a, principalID, ActionTwinRead, "twin")
	if err != nil {
		return nil, err
	}
	f.visible, f.hidden = visible, hidden
	f.unrestricted = visible == nil && hidden.Empty()
	return f, nil
}

// Event is what the filter needs to know about one event, without importing the
// event package — which imports nothing from here and should stay that way.
type Event struct {
	// TwinID and ObjectiveID name what the event is about, when it is about
	// something. Either may be empty.
	TwinID      string
	ObjectiveID string

	// Labels are the containers the event already knows it belongs to. A cost
	// event carries them because the recorder copied them at write time, which
	// makes this the cheapest and most accurate case: no lookup, and the answer
	// is the tenancy as it was when the spend happened.
	Labels []string

	// Subject is the quota key an untenanted event names — "principal|alice",
	// "twin|t_7f2a". Quota pressure is the one event with no resource of its
	// own, and this is how it says who it is about.
	Subject string
}

// Allow reports whether this subscriber may see the event.
//
// The order is cheapest first, and the last case is the one that matters: an
// event nothing can classify is delivered only to an unrestricted reader.
// Broadcasting it instead would mean every new event type leaks by default,
// which is precisely the failure a stream must not have.
func (f *StreamFilter) Allow(ctx context.Context, e Event) bool {
	if f == nil {
		return false
	}
	if f.unrestricted {
		return true
	}

	// Labels the event carries are authoritative and free.
	if len(e.Labels) > 0 {
		return f.labelsAllowed(e.Labels)
	}

	// A twin, or an objective, resolved through the tenancy tree and then
	// remembered.
	if id := e.TwinID; id != "" {
		return f.resourceAllowed(ctx, "twin", id)
	}
	if id := e.ObjectiveID; id != "" {
		return f.resourceAllowed(ctx, "objective", id)
	}

	// Untenanted. A subject naming a principal is that principal's own business
	// and nobody else's; a subject naming a twin is the twin case above.
	if typ, id, ok := strings.Cut(e.Subject, "|"); ok && id != "" {
		switch typ {
		case "principal":
			return id == f.principalID
		case "twin":
			// The key may carry a third part — twin|id|capability — so take the
			// twin and ignore the rest.
			twinID, _, _ := strings.Cut(id, "|")
			return f.resourceAllowed(ctx, "twin", twinID)
		}
	}
	return false
}

func (f *StreamFilter) resourceAllowed(ctx context.Context, resourceType, id string) bool {
	key := resourceType + ":" + id
	if verdict, ok := f.decided[key]; ok {
		return verdict
	}

	verdict := f.idAllowed(key, id)
	if !verdict && f.containers != nil {
		labels, err := f.containers.ScopesOf(ctx, resourceType, id)
		// A tree that cannot be read withholds the event rather than
		// delivering it. This is the one place in the quota and container code
		// that fails closed rather than open, because the cost of being wrong
		// is asymmetric: a missing event is a gap in a dashboard, and an extra
		// one is another tenant's activity.
		verdict = err == nil && f.labelsAllowed(labels)
	}
	f.decided[key] = verdict
	return verdict
}

// idAllowed covers a binding that names the resource outright — "twin:abc" —
// which carries no labels and would otherwise need the tree to say yes.
func (f *StreamFilter) idAllowed(key, id string) bool {
	if f.visible == nil {
		return true
	}
	for _, want := range f.visible.IDs {
		if want == id || want == key {
			return true
		}
	}
	return false
}

func (f *StreamFilter) labelsAllowed(labels []string) bool {
	for _, label := range labels {
		if matchesSelector(f.hidden, label) {
			// Deny wins, here as everywhere else.
			return false
		}
	}
	if f.visible == nil {
		return true
	}
	for _, label := range labels {
		if matchesSelector(*f.visible, label) {
			return true
		}
	}
	return false
}

func matchesSelector(s storage.ScopeSelector, label string) bool {
	for _, want := range s.Labels {
		if want == label {
			return true
		}
	}
	for _, prefix := range s.LabelPrefixes {
		if strings.HasPrefix(label, prefix) {
			return true
		}
	}
	return false
}

// StreamAction is what a global stream is gated on at the route.
//
// It is twin:read rather than an action of its own: the stream shows what is
// happening to twins, and inventing a permission for the same information in a
// different shape would let a deployment grant one and not the other.
const StreamAction = ActionTwinRead

var _ auth.Action = StreamAction
