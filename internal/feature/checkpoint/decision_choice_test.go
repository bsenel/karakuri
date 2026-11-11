package checkpoint

import (
	"context"
	"errors"
	"testing"
	"time"

	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/event"
)

// The runner treats every choice other than reject or modify as approve, so a
// choice the service does not recognise must never reach it: "deny", a typo
// or an empty value would otherwise approve the plan it was meant to refuse.
func TestResolveRefusesAChoiceThatIsNotApproveRejectOrModify(t *testing.T) {
	for _, choice := range []string{"", "deny", "Approve", "approve ", "yes"} {
		t.Run("choice="+choice, func(t *testing.T) {
			store := newFakeStore()
			svc := NewService(store, event.NewHub())
			cp, _ := svc.Create(context.Background(), "obj", "twin", "r", "s",
				[]string{"approve", "reject", "modify"}, CreateOptions{})

			err := svc.Resolve(context.Background(), cp.ID, corecheckpoint.Decision{Choice: choice, Approver: "bsenel"})

			if !errors.Is(err, coreerrors.ErrInvalidInput) {
				t.Fatalf("Resolve(%q) = %v, want an ErrInvalidInput refusal", choice, err)
			}
			if len(store.events) != 0 {
				t.Errorf("Resolve(%q) wrote %d audit row(s); a refused decision records nothing", choice, len(store.events))
			}
			got, _ := store.GetCheckpoint(context.Background(), cp.ID)
			if got.Status == corecheckpoint.StatusResolved {
				t.Errorf("Resolve(%q) marked the checkpoint resolved; it must stay pending", choice)
			}
		})
	}
}

// A checkpoint that names its own options, such as reconcile's stall
// (resume, pause, investigate) or a budget's (approve, reject), is decided by
// those: each is accepted, and a choice it did not offer is refused, including
// a plan's "modify" on a budget checkpoint. Reject is accepted on every one,
// because an expiry and a stop close a checkpoint with it whatever it offered.
func TestResolveAcceptsOnlyTheCheckpointsOwnOptions(t *testing.T) {
	for _, tc := range []struct {
		options []string
		ok      []string
		refused []string
	}{
		{options: []string{"resume", "pause", "investigate"}, ok: []string{"resume", "pause", "investigate", "reject"}, refused: []string{"approve", "modify", ""}},
		{options: []string{"approve", "reject"}, ok: []string{"approve", "reject"}, refused: []string{"modify"}},
	} {
		for _, choice := range tc.ok {
			store := newFakeStore()
			svc := NewService(store, event.NewHub())
			cp, _ := svc.Create(context.Background(), "obj", "twin", "r", "s", tc.options, CreateOptions{})
			if err := svc.Resolve(context.Background(), cp.ID, corecheckpoint.Decision{Choice: choice, Approver: "bsenel"}); err != nil {
				t.Errorf("options %v: Resolve(%q) = %v, want accepted", tc.options, choice, err)
			}
		}
		for _, choice := range tc.refused {
			store := newFakeStore()
			svc := NewService(store, event.NewHub())
			cp, _ := svc.Create(context.Background(), "obj", "twin", "r", "s", tc.options, CreateOptions{})
			if err := svc.Resolve(context.Background(), cp.ID, corecheckpoint.Decision{Choice: choice, Approver: "bsenel"}); !errors.Is(err, coreerrors.ErrInvalidInput) {
				t.Errorf("options %v: Resolve(%q) = %v, want an ErrInvalidInput refusal", tc.options, choice, err)
			}
		}
	}
}

// An expiry closes a checkpoint that offered no reject, such as reconcile's
// stall checkpoint, all the same: refusing it would leave the checkpoint
// pending for ever.
func TestExpireDueClosesACheckpointWhateverItOffered(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, event.NewHub())
	past := time.Now().Add(-time.Minute)
	cp, _ := svc.Create(context.Background(), "obj", "twin", "stalled", "s",
		[]string{"resume", "pause", "investigate"}, CreateOptions{ExpiresAt: &past})

	n, err := svc.ExpireDue(context.Background(), time.Now())
	if err != nil || n != 1 {
		t.Fatalf("ExpireDue = (%d, %v), want (1, nil)", n, err)
	}
	if got, _ := store.GetCheckpoint(context.Background(), cp.ID); got.Status != corecheckpoint.StatusResolved {
		t.Errorf("status = %q, want resolved", got.Status)
	}
}
