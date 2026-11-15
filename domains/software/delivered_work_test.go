package software

import (
	"reflect"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
)

// Phase 44 step 3 slice C: the git observation says, in the core's own terms,
// which steps its checklists tick, so the decide policy can ask about a plan
// that would do one again without reading pull requests or Markdown.

// Delivered holds every ticked item that names a step, with its SHAs and its
// pull request, and nothing else: not an unticked item, not a ticked item
// whose step cannot be read, and nothing from a body with no task list.
func TestGitObservationDeliveredHoldsTheTickedStepsOnly(t *testing.T) {
	obs, _ := observedPRs(t, deliveredPRs())

	want := []environment.DeliveredWork{
		{Ref: "phase 44 step 4", Where: "pull request #41", SHAs: []string{"abc1234"}},
		{Ref: "phase 44 step 5", Where: "pull request #41", SHAs: []string{"abcdef1"}},
		{Ref: "phase 7 step 3", Where: "pull request #43"},
	}
	if !reflect.DeepEqual(obs.Delivered, want) {
		t.Errorf("Delivered =\n  %#v\nwant\n  %#v", obs.Delivered, want)
	}
}

// No pull request ticks a step, so nothing is delivered, and the field says so
// by being empty rather than by carrying the unticked items.
func TestGitObservationDeliveredIsEmptyWhenNothingIsTicked(t *testing.T) {
	prs := deliveredPRs()[:1]
	prs[0].Body = "- [ ] Step 4 first\n- [ ] Step 5 second"
	obs, _ := observedPRs(t, prs)
	if len(obs.Delivered) != 0 {
		t.Errorf("Delivered = %#v, want none when no item is ticked", obs.Delivered)
	}
}
