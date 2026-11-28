package software

// The blockers the streams handed over (Phase 40 step 14c): one a stream could
// not remove after its declared attempts, or a kind that came back inside the
// declared window. The engineering_backlog stream observes them and files one
// Engineering Backlog entry per kind with the evidence.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// EnvHandedBlockers is the environment the engineering_backlog stream reads
// the handed-over blockers from.
const EnvHandedBlockers environment.EnvironmentID = "software.env.handed_blockers"

// HandedBlocker is one kind of blocker handed to the backlog, across every
// stream that handed it, with its evidence.
type HandedBlocker struct {
	Kind string `json:"kind"`
	// Streams are the objectives that handed it over, sorted.
	Streams []string `json:"streams"`
	// Subjects are what it was about, sorted.
	Subjects  []string `json:"subjects"`
	Diagnosis string   `json:"diagnosis,omitempty"`
	// Attempts are every failed attempt, with its audit rows, oldest first.
	Attempts []reconcile.Attempt `json:"attempts,omitempty"`
	// Occurrences is how many times a blocker of the kind was cleared and came
	// back, summed over the streams.
	Occurrences int `json:"occurrences"`
}

// NewHandedBlockersEnvironment observes the handovers on the reconcile states
// read returns.
func NewHandedBlockersEnvironment(id environment.EnvironmentID, read func(context.Context) ([]reconcile.State, error)) environment.Environment {
	return &handedBlockersEnv{id: id, read: read}
}

// readTwinHandovers reads the reconcile states of the twin's standing
// objectives from the store: another twin's handovers are not this backlog's
// to file. An objective declared but not yet reconciled has no state and adds
// nothing; any other read error is returned, so the environment is blind.
func readTwinHandovers(store storage.StorageAdapter, twinID string) func(context.Context) ([]reconcile.State, error) {
	return func(ctx context.Context) ([]reconcile.State, error) {
		objs, err := store.ListObjectives(ctx, storage.ObjectiveFilter{TwinID: twinID, Mode: string(objective.ModeStanding)})
		if err != nil {
			return nil, err
		}
		var states []reconcile.State
		for _, o := range objs {
			st, err := store.GetReconcileState(ctx, o.ID)
			if errors.Is(err, coreerrors.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			states = append(states, st)
		}
		return states, nil
	}
}

type handedBlockersEnv struct {
	id   environment.EnvironmentID
	read func(context.Context) ([]reconcile.State, error)
}

func (e *handedBlockersEnv) ID() environment.EnvironmentID { return e.id }
func (e *handedBlockersEnv) Domain() string                { return "software" }

// Observe lists the handed-over blockers, one per kind and sorted by kind, the
// lines that tell the backlog pass what to file, and has_work when there is
// any. States that cannot be read are an error, never "nothing handed over".
// The diagnoses are the deployment's own agents' reasoning, so the
// observation is the operator's.
func (e *handedBlockersEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	if e.read == nil {
		return environment.Observation{}, errors.New("no source for the reconcile states is bound")
	}
	states, err := e.read(ctx)
	if err != nil {
		return environment.Observation{}, fmt.Errorf("the handed-over blockers could not be read: %w", err)
	}
	handed := handedBlockers(states)
	raw, err := json.Marshal(handed)
	if err != nil {
		return environment.Observation{}, fmt.Errorf("the handed-over blockers could not be hashed: %w", err)
	}
	sum := sha256.Sum256(raw)
	return environment.Observation{
		EnvID: e.id,
		State: map[string]any{
			"handed_blockers":       handed,
			"handed_blockers_lines": handedBlockerLines(handed),
			"has_work":              len(handed) > 0,
		},
		Version:   hex.EncodeToString(sum[:])[:16],
		Timestamp: time.Now().UTC(),
	}, nil
}

// handedBlockers groups the states' handovers by kind, sorted by kind.
func handedBlockers(states []reconcile.State) []HandedBlocker {
	byKind := map[string]*HandedBlocker{}
	streams := map[string]map[string]bool{}
	subjects := map[string]map[string]bool{}
	for _, st := range states {
		for _, h := range st.Handovers {
			kind := string(h.Kind)
			hb, ok := byKind[kind]
			if !ok {
				hb = &HandedBlocker{Kind: kind}
				byKind[kind] = hb
				streams[kind] = map[string]bool{}
				subjects[kind] = map[string]bool{}
			}
			streams[kind][string(st.ObjectiveID)] = true
			if h.Subject != "" {
				subjects[kind][h.Subject] = true
			}
			if hb.Diagnosis == "" {
				hb.Diagnosis = h.Diagnosis
			}
			hb.Attempts = append(hb.Attempts, h.Attempts...)
			hb.Occurrences += len(h.Cleared)
		}
	}
	out := make([]HandedBlocker, 0, len(byKind))
	for kind, hb := range byKind {
		hb.Streams = sortedKeys(streams[kind])
		hb.Subjects = sortedKeys(subjects[kind])
		sort.SliceStable(hb.Attempts, func(i, j int) bool { return hb.Attempts[i].At.Before(hb.Attempts[j].At) })
		out = append(out, *hb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// handedBlockerLines are what the planner reads: for each kind, file one
// Engineering Backlog entry, or add to the one already filed, with all the
// evidence, and propose the change to the brief, the template or the code
// that would stop the blocker.
func handedBlockerLines(handed []HandedBlocker) []string {
	lines := make([]string, 0, len(handed))
	for _, h := range handed {
		var b strings.Builder
		fmt.Fprintf(&b, "Blocker kind %s, handed over by %s, about %s: file one Engineering Backlog entry for it, "+
			"or add this evidence to the entry already filed for the kind, and propose the change to the stream brief, the template or the code that removes it.",
			h.Kind, strings.Join(h.Streams, ", "), strings.Join(h.Subjects, ", "))
		if h.Diagnosis != "" {
			fmt.Fprintf(&b, " Diagnosis: %s.", h.Diagnosis)
		}
		if h.Occurrences > 0 {
			fmt.Fprintf(&b, " Cleared and came back %d time(s).", h.Occurrences)
		}
		for i, a := range h.Attempts {
			fmt.Fprintf(&b, " Attempt %d (loop %s, %s): tried %q", i+1, a.LoopID, a.At.UTC().Format(time.RFC3339), a.Remedy)
			if len(a.AuditRows) > 0 {
				fmt.Fprintf(&b, ", audit rows %s", strings.Join(a.AuditRows, ", "))
			}
			b.WriteString(".")
		}
		lines = append(lines, b.String())
	}
	return lines
}

func (e *handedBlockersEnv) Act(_ context.Context, a environment.Action) (environment.ActionResult, error) {
	return environment.ActionResult{Success: false, Error: fmt.Sprintf("%s: the handed-blockers environment only observes", a.CapabilityID)}, nil
}

func (e *handedBlockersEnv) Subscribe(context.Context, environment.EventFilter) (<-chan environment.EnvironmentEvent, error) {
	return nil, nil
}

// Snapshot has an empty SHA when Observe could not read (AGENTS.md rule 10).
func (e *handedBlockersEnv) Snapshot(ctx context.Context) (environment.EnvironmentSnapshot, error) {
	obs, err := e.Observe(ctx, environment.ObservationQuery{})
	if err != nil {
		return environment.EnvironmentSnapshot{EnvID: e.id}, nil
	}
	return environment.EnvironmentSnapshot{SHA: obs.Version, EnvID: e.id, State: obs.State, Timestamp: obs.Timestamp}, nil
}
