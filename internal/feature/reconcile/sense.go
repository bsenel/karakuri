package reconcile

import (
	"context"
	"sort"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/core/vfs"
)

// fingerprint hashes what every environment says its current state is.
//
// This is the cheap tier, and the economics of the whole feature rest on it.
// Snapshot is an adapter call — a git ref, a ticket board's ETag — not a model
// call, so an objective can be checked every fifteen minutes all year and only
// spend money on the days something moved. A design that ran the full loop on
// every check would cost a hundred times more and tell the operator the same
// thing on ninety-nine of those hundred occasions.
//
// Environment IDs are sorted before hashing. Environments are built by walking
// the objective's domains and each domain's factory registry, and neither order
// is guaranteed stable across builds or across replicas. An unsorted hash would
// report drift every time a pack's registration order changed, which is to say
// it would report drift on deploys and not on commits.
//
// (stepObserve computes a superficially similar composite over observation
// versions and is deliberately left alone. It hashes what the loop actually
// read, in the order it read it, as a record of one iteration's input; this
// hashes what the world claims to be, order-independently, as a value to
// compare against later. Sharing the two would mean giving one of them an
// ordering guarantee it does not need or taking one away from the other.)
func fingerprint(ctx context.Context, envs []environment.Environment) reconcile.Fingerprint {
	fp := reconcile.Fingerprint{Environments: map[string]string{}}

	for _, env := range envs {
		id := string(env.ID())
		snap, err := env.Snapshot(ctx)
		if err != nil || snap.SHA == "" {
			// An environment that cannot hash itself is saying "I don't
			// know", and a system that read that as "unchanged" would go
			// quiet exactly when it should not. It contributes nothing to
			// the hash and is named as blind instead.
			fp.Blind = append(fp.Blind, id)
			continue
		}
		fp.Environments[id] = snap.SHA
	}

	ids := make([]string, 0, len(fp.Environments))
	for id := range fp.Environments {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	sort.Strings(fp.Blind)

	if len(ids) > 0 {
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = id + ":" + fp.Environments[id]
		}
		fp.SHA = vfs.SHA([]byte(strings.Join(parts, ",")))
	}
	return fp
}

// compare measures a fresh fingerprint against the one taken when the
// objective last converged.
//
// Against the last convergence rather than against the previous observation,
// which matters more than it sounds: an environment that changes and changes
// back is not drift, and an objective that never converged has nothing to have
// drifted from. Comparing consecutive observations would fire twice on a
// reverted commit and stay silent on a change that has been sitting there
// unaddressed since yesterday.
func compare(prev, next reconcile.Fingerprint) reconcile.Drift {
	drift := reconcile.Drift{From: prev.SHA, To: next.SHA}

	if next.SHA == "" {
		// Nothing could be hashed, so the comparison proves nothing either
		// way. Reporting this as "no drift" would be a lie of the most
		// expensive kind — it is precisely the case where the schedule, not
		// the hash, has to be what moves the objective.
		drift.Blind = true
		return drift
	}
	if prev.SHA == "" {
		// No baseline: the objective has never converged, so there is
		// nothing to have drifted from. The schedule decides whether to run,
		// and the first convergence establishes the baseline.
		return drift
	}
	if prev.SHA == next.SHA {
		return drift
	}

	drift.Changed = true
	for id, sha := range next.Environments {
		if prev.Environments[id] != sha {
			drift.Environments = append(drift.Environments, id)
		}
	}
	// An environment that has disappeared since the last convergence changed
	// the world as surely as one that gained a commit.
	for id := range prev.Environments {
		if _, still := next.Environments[id]; !still {
			drift.Environments = append(drift.Environments, id)
		}
	}
	sort.Strings(drift.Environments)
	return drift
}
