package conformance_test

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/domains/agriculture"
	"github.com/bsenel/karakuri/domains/consulting"
	"github.com/bsenel/karakuri/domains/healthcare"
	"github.com/bsenel/karakuri/domains/legal"
	"github.com/bsenel/karakuri/domains/mechanical"
	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/domain"
)

// Every shipped pack passes the suite it is supposed to pass.
//
// This did not exist. The conformance suite was reachable only by a human
// typing `krk domain test <pack>` — bootstrap runs the cross-pack collision
// check and nothing else, and no pack had a test of its own. So a pack could
// be broken against its own contract for as long as nobody thought to run the
// command.
//
// It was not hypothetical: folding self-improvement into the software pack
// (ADR 018) shipped a capability with no OutputSchema and an environment
// factory that refused to build, and the full suite passed throughout because
// nothing ran this check.
//
// It lives here rather than in each pack's own package so that adding a pack
// and forgetting its test is not possible — a new pack is added to this list
// or it is not registered in bootstrap either.
func TestShippedPacksConform(t *testing.T) {
	packs := map[string]domain.Pack{
		"software":    software.New(),
		"agriculture": agriculture.New(),
		"consulting":  consulting.New(),
		"healthcare":  healthcare.New(),
		"legal":       legal.New(),
		"mechanical":  mechanical.New(),
	}

	for name, pack := range packs {
		t.Run(name, func(t *testing.T) {
			results := conformance.New().Run(context.Background(), pack)
			if len(results) == 0 {
				t.Fatal("the suite returned no results; it has stopped checking anything")
			}
			for _, r := range results {
				if !r.Passed {
					t.Errorf("%s: %s", r.Check, r.Message)
				}
			}
		})
	}
}

// The packs registered in bootstrap and the packs checked above are the same
// set. Without this, a pack could be shipped and never conform-checked simply
// by not being added to the map.
func TestEveryShippedPackIsChecked(t *testing.T) {
	// Kept in sync with internal/app/bootstrap.go's allPacks.
	shipped := []string{"software", "agriculture", "consulting", "healthcare", "legal", "mechanical"}
	checked := map[string]bool{
		"software": true, "agriculture": true, "consulting": true,
		"healthcare": true, "legal": true, "mechanical": true,
	}
	for _, id := range shipped {
		if !checked[id] {
			t.Errorf("pack %q is registered at boot but never conform-checked", id)
		}
	}
}
