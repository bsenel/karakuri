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
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
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
	// EvidenceAt is the newest evidence across the streams: the latest of
	// each handover's time, its attempts' and its clears', to the second.
	EvidenceAt time.Time `json:"evidence_at"`
	// FiledThrough is the evidence time the newest Engineering Backlog entry
	// filed for the kind covers, nil while none is. The kind is the owner's
	// turn, and gives the stream no work, while it is not before EvidenceAt.
	FiledThrough *time.Time `json:"filed_through,omitempty"`
}

// filed reports whether an entry filed for the kind covers all its evidence.
func (h HandedBlocker) filed() bool {
	return h.FiledThrough != nil && !h.FiledThrough.Before(h.EvidenceAt)
}

// backlogEntryMarker starts the one line a backlog pull request's description
// carries per kind it files: "Karakuri-Backlog-Entry: kind=<kind>
// evidence=<RFC 3339 time>". The observation hands the pass the exact line.
const backlogEntryMarker = "Karakuri-Backlog-Entry:"

func backlogEntryLine(kind string, evidence time.Time) string {
	return fmt.Sprintf("%s kind=%s evidence=%s", backlogEntryMarker, kind, evidence.UTC().Format(time.RFC3339))
}

// NewHandedBlockersEnvironment observes the handovers on the reconcile states
// read returns, and reads from vc which of them the backlog stream already
// filed.
func NewHandedBlockersEnvironment(id environment.EnvironmentID, read func(context.Context) ([]reconcile.State, error), vc versioncontrol.VersionControlAdapter) environment.Environment {
	return &handedBlockersEnv{id: id, read: read, vc: vc}
}

// backlogFilings reads, from the description of each of the engineering
// backlog stream's own pull requests, the newest evidence time its marker
// line covers per kind. A pull request counts when its branch is the stream's
// and an own author opened it, whether open, merged, or closed unmerged by the
// owner (an answer too, until new evidence comes): a stranger's pull request,
// whatever its branch, never retires a handover. A list the forge cannot read
// is an error, never "nothing filed".
func backlogFilings(ctx context.Context, vc versioncontrol.VersionControlAdapter) (map[string]time.Time, error) {
	if vc == nil || !vc.Active() {
		return nil, errors.New("no version control adapter is bound to read the backlog pull requests from")
	}
	open, err := vc.ListPRs(ctx, "", time.Time{})
	if err != nil {
		return nil, fmt.Errorf("open pull requests: %w", err)
	}
	merged, err := vc.ListMergedPRs(ctx, "", 0)
	if err != nil {
		return nil, fmt.Errorf("merged pull requests: %w", err)
	}
	closed, err := vc.ListClosedPRs(ctx, "", 0)
	if err != nil {
		return nil, fmt.Errorf("closed pull requests: %w", err)
	}
	filings := map[string]time.Time{}
	for _, pr := range append(append(open, merged...), closed...) {
		if !pr.OwnAuthor || branchStream(pr.HeadBranch) != engineeringBacklogTemplateID {
			continue
		}
		for _, line := range strings.Split(pr.Body, "\n") {
			kind, at, ok := parseBacklogEntryLine(strings.TrimSpace(line))
			if ok && at.After(filings[kind]) {
				filings[kind] = at
			}
		}
	}
	return filings, nil
}

// parseBacklogEntryLine reads one marker line; any other line is not one.
func parseBacklogEntryLine(line string) (string, time.Time, bool) {
	rest, ok := strings.CutPrefix(line, backlogEntryMarker+" kind=")
	if !ok {
		return "", time.Time{}, false
	}
	kind, evidence, ok := strings.Cut(rest, " evidence=")
	if !ok || kind == "" {
		return "", time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, evidence)
	if err != nil {
		return "", time.Time{}, false
	}
	return kind, at.UTC(), true
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
	vc   versioncontrol.VersionControlAdapter
}

func (e *handedBlockersEnv) ID() environment.EnvironmentID { return e.id }
func (e *handedBlockersEnv) Domain() string                { return "software" }

// Observe lists the handed-over blockers, one per kind and sorted by kind, the
// lines that tell the backlog pass what to file, and has_work when there is
// any: a kind with no entry filed, or with evidence newer than its entry. The
// forge is read only when something is handed over. States or pull requests
// that cannot be read are an error, never "nothing to file". The diagnoses are
// the deployment's own agents' reasoning, and of the pull requests only a
// handed kind and a time are kept, so the observation is the operator's.
func (e *handedBlockersEnv) Observe(ctx context.Context, _ environment.ObservationQuery) (environment.Observation, error) {
	if e.read == nil {
		return environment.Observation{}, errors.New("no source for the reconcile states is bound")
	}
	states, err := e.read(ctx)
	if err != nil {
		return environment.Observation{}, fmt.Errorf("the handed-over blockers could not be read: %w", err)
	}
	handed := handedBlockers(states)
	if len(handed) > 0 {
		filings, err := backlogFilings(ctx, e.vc)
		if err != nil {
			return environment.Observation{}, fmt.Errorf("the backlog entries filed for the handed-over blockers could not be read: %w", err)
		}
		for i := range handed {
			if at, ok := filings[handed[i].Kind]; ok {
				handed[i].FiledThrough = &at
			}
		}
	}
	lines := handedBlockerLines(handed)
	raw, err := json.Marshal(handed)
	if err != nil {
		return environment.Observation{}, fmt.Errorf("the handed-over blockers could not be hashed: %w", err)
	}
	sum := sha256.Sum256(raw)
	return environment.Observation{
		EnvID: e.id,
		State: map[string]any{
			"handed_blockers":       handed,
			"handed_blockers_lines": lines,
			"has_work":              len(lines) > 0,
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
			hb.EvidenceAt = latest(hb.EvidenceAt, h.At)
			for _, a := range h.Attempts {
				hb.EvidenceAt = latest(hb.EvidenceAt, a.At)
			}
			for _, c := range h.Cleared {
				hb.EvidenceAt = latest(hb.EvidenceAt, c)
			}
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

// latest is the later of a and b, b in UTC to the second, the precision the
// marker line carries.
func latest(a, b time.Time) time.Time {
	b = b.UTC().Truncate(time.Second)
	if b.After(a) {
		return b
	}
	return a
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
// evidence, propose the change to the brief, the template or the code that
// would stop the blocker, and put the marker line in the pull request's
// description. Only a kind with no entry, or with evidence newer than its
// entry, has a line: a filed kind is the owner's turn.
func handedBlockerLines(handed []HandedBlocker) []string {
	lines := make([]string, 0, len(handed))
	for _, h := range handed {
		if h.filed() {
			continue
		}
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
		if h.FiledThrough != nil {
			fmt.Fprintf(&b, " The entry filed for it covers the evidence up to %s: add what came since to that entry.", h.FiledThrough.Format(time.RFC3339))
		}
		fmt.Fprintf(&b, " Put this exact line in the pull request's description: %s", backlogEntryLine(h.Kind, h.EvidenceAt))
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
