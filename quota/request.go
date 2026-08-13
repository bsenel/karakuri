package quota

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Requests are how somebody asks for more without an operator editing a file.
//
// The record is here; who may approve one is not. That question needs to know
// what a principal is and what they administer, which is application
// vocabulary this module deliberately does not have — the host answers it
// before calling Decide.

// RequestStatus is where a request has got to.
type RequestStatus string

const (
	Pending  RequestStatus = "pending"
	Approved RequestStatus = "approved"
	Rejected RequestStatus = "rejected"
)

// Errors a request workflow returns.
var (
	// ErrRequestNotFound is returned for an ID no store holds.
	ErrRequestNotFound = errors.New("quota: request not found")

	// ErrRequestDecided is returned when a request has already been approved or
	// rejected. Deciding twice is refused rather than ignored: the second
	// decision is usually a duplicate submission, and silently keeping the
	// first would leave the second approver believing they did something.
	ErrRequestDecided = errors.New("quota: request already decided")
)

// Request is somebody asking for a limit to be raised.
//
// It carries the same fields an Override does, because approving one is exactly
// writing the other — a request that could not be turned into an override
// without further decisions would be a request nobody can act on.
type Request struct {
	ID string

	// Subject and Name say which limit, in the same terms an Override does.
	Subject Key
	Name    string

	// Cap and Window are what is being asked for.
	Cap    int
	Window time.Duration

	// ExpiresAt asks for a temporary raise. Zero asks for a permanent one.
	ExpiresAt time.Time

	// Reason is why, in the requester's words. Required: a limit raised for a
	// reason nobody wrote down is one nobody can review later.
	Reason string

	Status      RequestStatus
	RequestedBy string
	CreatedAt   time.Time

	// DecidedBy and DecidedAt are set when the request leaves Pending, and
	// DecisionNote carries the approver's words — most usefully on a rejection,
	// where "no" without a reason is the least useful answer an operator can
	// give.
	DecidedBy    string
	DecidedAt    time.Time
	DecisionNote string
}

// Validate reports whether the request is usable as submitted.
func (r Request) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: request has no id", ErrInvalidPolicy)
	}
	if strings.TrimSpace(r.RequestedBy) == "" {
		return fmt.Errorf("%w: request %q records no requester", ErrInvalidPolicy, r.ID)
	}
	if strings.TrimSpace(r.Reason) == "" {
		return fmt.Errorf("%w: request %q gives no reason", ErrInvalidPolicy, r.ID)
	}
	// The override it would become has to be valid, or the request is one
	// nobody could approve.
	return r.Override().Validate()
}

// Override is what approving this request writes.
func (r Request) Override() Override {
	return Override{
		Subject:   r.Subject,
		Name:      r.Name,
		Cap:       r.Cap,
		Window:    r.Window,
		ExpiresAt: r.ExpiresAt,
		Reason:    r.Reason,
	}
}

// RequestFilter narrows a request listing. Every field is optional, and an
// empty filter returns everything.
type RequestFilter struct {
	Subject Key
	Status  RequestStatus

	// RequestedBy returns one person's own requests, which is what somebody
	// without permission to see everyone's should get.
	RequestedBy string
}

func (f RequestFilter) matches(r Request) bool {
	switch {
	case f.Subject != "" && r.Subject != f.Subject:
		return false
	case f.Status != "" && r.Status != f.Status:
		return false
	case f.RequestedBy != "" && r.RequestedBy != f.RequestedBy:
		return false
	}
	return true
}

// RequestStore holds quota-increase requests.
type RequestStore interface {
	// PutRequest stores a request, replacing any with the same ID.
	PutRequest(ctx context.Context, r Request) error

	// GetRequest returns one, or ErrRequestNotFound.
	GetRequest(ctx context.Context, id string) (Request, error)

	// ListRequests returns matching requests, newest first.
	ListRequests(ctx context.Context, f RequestFilter) ([]Request, error)
}

// Requests is the workflow over a request store and an override store.
//
// The two stores are separate because they answer to different lifetimes: a
// request is a record of something somebody asked for, kept for as long as
// anybody might audit it, while an override is live state read on the hot path.
// Approving is the one operation that touches both.
type Requests struct {
	Store     RequestStore
	Overrides OverrideStore

	// Resolver, when set, is invalidated on approval so the process that
	// approved sees the new limit immediately rather than a TTL later.
	Resolver *Resolver
}

// Submit records a new request, in Pending.
//
// The caller supplies the ID and the clock, because this module does not
// generate identifiers or read the time — both are the host's, and both are
// what makes a submission reproducible in a test.
func (q Requests) Submit(ctx context.Context, r Request) (Request, error) {
	if q.Store == nil {
		return Request{}, errors.New("quota: no request store configured")
	}
	r.Status = Pending
	r.DecidedBy, r.DecisionNote = "", ""
	r.DecidedAt = time.Time{}
	if err := r.Validate(); err != nil {
		return Request{}, err
	}
	if err := q.Store.PutRequest(ctx, r); err != nil {
		return Request{}, err
	}
	return r, nil
}

// Decision records an approval or a rejection.
type Decisions struct {
	By       string
	Note     string
	At       time.Time
	Approved bool
}

// Decide approves or rejects a pending request, and on approval writes the
// override that makes it take effect.
//
// The order matters: the override is written first, and only then is the
// request marked approved. A crash between the two leaves an override in force
// and a request that still looks pending — somebody re-approves it, writes the
// same override again, and nothing is wrong. The other order would leave a
// request marked approved and a limit that never moved, which is the failure
// nobody notices until the person who asked complains.
func (q Requests) Decide(ctx context.Context, id string, d Decisions) (Request, error) {
	if q.Store == nil {
		return Request{}, errors.New("quota: no request store configured")
	}
	if strings.TrimSpace(d.By) == "" {
		return Request{}, fmt.Errorf("%w: a decision records who made it", ErrInvalidPolicy)
	}

	r, err := q.Store.GetRequest(ctx, id)
	if err != nil {
		return Request{}, err
	}
	if r.Status != Pending {
		return Request{}, fmt.Errorf("%w: %q is %s", ErrRequestDecided, id, r.Status)
	}

	if d.Approved {
		if q.Overrides == nil {
			return Request{}, errors.New("quota: no override store configured, so an approval would change nothing")
		}
		if err := q.Overrides.PutOverride(ctx, r.Override()); err != nil {
			return Request{}, fmt.Errorf("apply approved request %q: %w", id, err)
		}
		q.Resolver.Invalidate(r.Subject)
	}

	r.Status = Rejected
	if d.Approved {
		r.Status = Approved
	}
	r.DecidedBy, r.DecidedAt, r.DecisionNote = d.By, d.At, d.Note
	if err := q.Store.PutRequest(ctx, r); err != nil {
		return Request{}, err
	}
	return r, nil
}

// List returns matching requests.
func (q Requests) List(ctx context.Context, f RequestFilter) ([]Request, error) {
	if q.Store == nil {
		return nil, errors.New("quota: no request store configured")
	}
	return q.Store.ListRequests(ctx, f)
}

// MemoryRequestStore keeps requests in this process. Reference implementation;
// see MemoryOverrideStore for what that costs on a restart.
type MemoryRequestStore struct {
	mu sync.Mutex
	by map[string]Request
}

// NewMemoryRequestStore returns an empty in-process request store.
func NewMemoryRequestStore() *MemoryRequestStore {
	return &MemoryRequestStore{by: map[string]Request{}}
}

var _ RequestStore = (*MemoryRequestStore)(nil)

func (s *MemoryRequestStore) PutRequest(_ context.Context, r Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.by[r.ID] = r
	return nil
}

func (s *MemoryRequestStore) GetRequest(_ context.Context, id string) (Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.by[id]
	if !ok {
		return Request{}, fmt.Errorf("%w: %q", ErrRequestNotFound, id)
	}
	return r, nil
}

func (s *MemoryRequestStore) ListRequests(_ context.Context, f RequestFilter) ([]Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []Request
	for _, r := range s.by {
		if f.matches(r) {
			out = append(out, r)
		}
	}
	// Newest first, ID breaking ties so two requests submitted in the same
	// instant still list in a stable order.
	slices.SortFunc(out, func(a, b Request) int {
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return b.CreatedAt.Compare(a.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}
