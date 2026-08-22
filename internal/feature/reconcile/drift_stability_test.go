package reconcile

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/core/environment"
)

// The cheap tier's whole economic argument, asserted against the shipped
// environments rather than against a fake that hashes the way we hope they do.
//
// fingerprint → compare is what decides whether a standing objective spends a
// model call. Every environment feeds it Snapshot().SHA, and six of the
// software pack's built that SHA by ranging a Go map — randomised per range, so
// two consecutive senses over a world that had not moved produced two different
// composites and `compare` reported drift. Every sense tick ran the expensive
// tier. An objective checked every fifteen minutes would have paid for the full
// loop ninety-six times a day to learn nothing had changed.
//
// This test imports the pack from a feature test, which is the wrong direction
// for production code and deliberate here: the defect was in the pack and the
// consequence was in the supervisor, and a test that stayed on one side of that
// line is exactly what missed it for six phases. It is a _test.go, so the
// production dependency graph is unchanged.
func TestSupervisorSeesNoDriftWhenNothingMoved(t *testing.T) {
	ctx := context.Background()

	// Built through the pack's own factories, so this covers whatever the pack
	// ships rather than a list somebody remembered to update.
	var envs []environment.Environment
	for _, f := range software.New().EnvironmentFactories() {
		env, err := f.Build(environment.BuildContext{})
		if err != nil {
			t.Fatalf("build %q: %v", f.EnvID, err)
		}
		envs = append(envs, env)
	}
	if len(envs) == 0 {
		t.Fatal("the software pack registered no environments")
	}

	baseline := fingerprint(ctx, envs)
	if baseline.SHA == "" {
		t.Fatal("nothing could be hashed; the comparison below would prove nothing")
	}

	// Repeated, because map order agrees by luck often enough that one
	// re-sense passes on a broken hash.
	for i := 0; i < 20; i++ {
		next := fingerprint(ctx, envs)
		drift := compare(baseline, next)
		if drift.Blind {
			t.Fatalf("round %d: the sense went blind", i)
		}
		if drift.Changed {
			t.Fatalf("round %d: the supervisor reported drift in %v with nothing moved — "+
				"the expensive tier would run on every tick", i, drift.Environments)
		}
	}
}
