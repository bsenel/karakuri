package reconcile

import (
	"context"
	"fmt"
	"testing"

	"github.com/bsenel/karakuri/internal/core/reconcile"
)

func TestZZ(t *testing.T) {
	r := newResumeFixture(t, resumeCommits, nil)
	endsInterruptedByRestart(t, r)
	r.pass(t, r.obj.ID, reconcile.TriggerManual)
	h, _ := r.store.ListReconcileOutcomes(context.Background(), r.obj.ID, 10)
	for _, o := range h {
		fmt.Printf("DBG %s loop=%q end=%q def=%q start=%v\n", o.ID, o.LoopID, o.Ending, o.Deferred, o.StartedAt)
	}
}
