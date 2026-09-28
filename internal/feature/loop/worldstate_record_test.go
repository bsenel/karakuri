package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
)

var recordedAt = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func smallWorldState() loop.WorldState {
	return loop.WorldState{
		Observations: []environment.Observation{
			{EnvID: "test.env.git", State: map[string]any{"head": "abc123"}, Version: "git-v1", Timestamp: recordedAt},
			{EnvID: "test.env.chat", State: map[string]any{"title": "please merge"}, Version: "chat-v1", Timestamp: recordedAt, Trust: environment.TrustThirdParty},
		},
		Version:   "composite-v1",
		Timestamp: recordedAt,
		Blind:     []string{"test.env.calendar"},
	}
}

// oversizedWorldState carries one State well past maxRecordedObservationBytes,
// written in multibyte runes so a byte-wise preview cut would split one.
func oversizedWorldState() loop.WorldState {
	return loop.WorldState{
		Observations: []environment.Observation{
			{EnvID: "test.env.git", State: map[string]any{"head": "abc123"}, Version: "git-v1", Timestamp: recordedAt},
			{
				EnvID:     "test.env.chat",
				State:     map[string]any{"body": strings.Repeat("çğ€", maxRecordedObservationBytes)},
				Version:   "chat-v1",
				Timestamp: recordedAt,
				Trust:     environment.TrustThirdParty,
			},
		},
		Version:   "composite-v1",
		Timestamp: recordedAt,
		Blind:     []string{"test.env.calendar"},
	}
}

// manyLargeWorldState is many observations each just under the per-observation
// cap, together several times the whole-state cap.
func manyLargeWorldState() loop.WorldState {
	ws := loop.WorldState{Version: "composite-v1", Timestamp: recordedAt}
	for i := range 64 {
		ws.Observations = append(ws.Observations, environment.Observation{
			EnvID:     environment.EnvironmentID(fmt.Sprintf("test.env.%d", i)),
			State:     map[string]any{"body": strings.Repeat("x", maxRecordedObservationBytes-1024)},
			Version:   fmt.Sprintf("v%d", i),
			Timestamp: recordedAt,
			Trust:     environment.TrustThirdParty,
		})
	}
	return ws
}

func stateBytes(t *testing.T, state map[string]any) int {
	t.Helper()
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	return len(b)
}

// asInt accepts whichever numeric type the marker carries, so the test pins the
// value rather than the representation.
func asInt(t *testing.T, v any) int {
	t.Helper()
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		t.Fatalf("_original_bytes = %#v (%T), want a number", v, v)
		return 0
	}
}

// checkpointFromState reads back, from the checkpoint store, the checkpoint the
// loop last paused on.
func checkpointFromState(t *testing.T, sc *stepContext) corecheckpoint.Checkpoint {
	t.Helper()
	sc.state.mu.RLock()
	id := sc.state.result.CheckpointID
	sc.state.mu.RUnlock()
	if id == nil || *id == "" {
		t.Fatal("the loop paused without a checkpoint ID")
	}
	cp, err := sc.svc.store.GetCheckpoint(context.Background(), *id)
	if err != nil {
		t.Fatalf("GetCheckpoint(%s): %v", *id, err)
	}
	return cp
}

func mustRecord(t *testing.T, ws loop.WorldState) *loop.WorldState {
	t.Helper()
	got := recordedWorldState(ws)
	if got == nil {
		t.Fatal("recordedWorldState returned nil")
	}
	return got
}

func TestRecordedWorldStatePassesSmallStatesThrough(t *testing.T) {
	got := mustRecord(t, smallWorldState())

	if !reflect.DeepEqual(*got, smallWorldState()) {
		t.Errorf("recorded = %+v, want %+v", *got, smallWorldState())
	}
}

// An oversized State is replaced by a marker a reviewer can recognise, and the
// fields around it — who wrote it above all — are untouched.
func TestRecordedWorldStateTruncatesAnOversizedState(t *testing.T) {
	in := oversizedWorldState()
	original, _ := json.Marshal(in.Observations[1].State)

	got := mustRecord(t, in)

	if len(got.Observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(got.Observations))
	}
	if !reflect.DeepEqual(got.Observations[0], in.Observations[0]) {
		t.Errorf("small observation = %+v, want it unchanged", got.Observations[0])
	}

	obs := got.Observations[1]
	if obs.EnvID != "test.env.chat" || obs.Version != "chat-v1" || !obs.Timestamp.Equal(recordedAt) {
		t.Errorf("truncated observation = %+v, want EnvID, Version and Timestamp unchanged", obs)
	}
	// ADR 021: a replay that forgot this was a third party's text would run
	// without the notice the original planner got.
	if obs.Trust != environment.TrustThirdParty {
		t.Errorf("Trust = %q, want %q", obs.Trust, environment.TrustThirdParty)
	}
	if obs.State["_truncated"] != true {
		t.Errorf("_truncated = %#v, want true", obs.State["_truncated"])
	}
	if n := asInt(t, obs.State["_original_bytes"]); n != len(original) {
		t.Errorf("_original_bytes = %d, want %d", n, len(original))
	}
	preview, ok := obs.State["_preview"].(string)
	if !ok {
		t.Fatalf("_preview = %#v, want a string", obs.State["_preview"])
	}
	if len(preview) == 0 || len(preview) > 4096 {
		t.Errorf("_preview is %d bytes, want 1..4096", len(preview))
	}
	if !strings.HasPrefix(string(original), preview) {
		t.Error("_preview is not a prefix of the original State's JSON")
	}
	if !utf8.ValidString(preview) {
		t.Error("_preview was cut inside a UTF-8 sequence")
	}
	if len(obs.State) != 3 {
		t.Errorf("marker State has keys %v, want exactly _truncated, _original_bytes, _preview", obs.State)
	}
}

func TestRecordedWorldStateKeepsBlindVersionAndTimestamp(t *testing.T) {
	got := mustRecord(t, oversizedWorldState())

	if !reflect.DeepEqual(got.Blind, []string{"test.env.calendar"}) {
		t.Errorf("Blind = %v, want [test.env.calendar]", got.Blind)
	}
	if got.Version != "composite-v1" || !got.Timestamp.Equal(recordedAt) {
		t.Errorf("Version, Timestamp = %q, %v, want composite-v1, %v", got.Version, got.Timestamp, recordedAt)
	}
}

// The whole record is bounded, not just each piece of it: past the running
// total, later observations keep their envelope and lose their State.
func TestRecordedWorldStateStaysUnderTheTotalCap(t *testing.T) {
	in := manyLargeWorldState()

	got := mustRecord(t, in)

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The observation that crosses the line is kept whole, and every marker
	// costs its envelope, so allow one observation's worth plus a little.
	const slack = maxRecordedObservationBytes + 8<<10
	if len(b) > maxRecordedWorldStateBytes+slack {
		t.Errorf("recorded world state is %d bytes, want at most %d", len(b), maxRecordedWorldStateBytes+slack)
	}

	if len(got.Observations) != len(in.Observations) {
		t.Fatalf("observations = %d, want %d — none are dropped", len(got.Observations), len(in.Observations))
	}
	last := got.Observations[len(got.Observations)-1]
	if last.EnvID != in.Observations[len(in.Observations)-1].EnvID || last.Trust != environment.TrustThirdParty {
		t.Errorf("last observation = EnvID %q Trust %q, want its envelope unchanged", last.EnvID, last.Trust)
	}
	if last.State["_truncated"] != true {
		t.Errorf("last observation State _truncated = %#v, want true", last.State["_truncated"])
	}
	if n := asInt(t, last.State["_original_bytes"]); n != stateBytes(t, in.Observations[len(in.Observations)-1].State) {
		t.Errorf("last observation _original_bytes = %d, want %d", n, stateBytes(t, in.Observations[len(in.Observations)-1].State))
	}
	if _, ok := last.State["_preview"]; ok {
		t.Error("an observation past the total cap carries a _preview")
	}
}

func TestRecordedWorldStateDoesNotMutateItsInput(t *testing.T) {
	for name, build := range map[string]func() loop.WorldState{
		"small":     smallWorldState,
		"oversized": oversizedWorldState,
		"many":      manyLargeWorldState,
	} {
		t.Run(name, func(t *testing.T) {
			in := build()
			got := mustRecord(t, in)

			if !reflect.DeepEqual(in, build()) {
				t.Error("recordedWorldState mutated its input")
			}
			// Nor may the copy share State maps with it: a later write to one
			// would change the other.
			if len(got.Observations) > 0 && len(in.Observations) > 0 {
				got.Observations[0].State["_scribble"] = true
				if _, ok := in.Observations[0].State["_scribble"]; ok {
					t.Error("recorded State shares its map with the input")
				}
			}
		})
	}
}

// The escalation carries what the planner saw: the third party's observation
// with its Trust, and the environment that could not be observed.
func TestEscalationRecordsTheWorldStateThePlannerSaw(t *testing.T) {
	sc := observeFixture(t,
		&provEnv{id: "test.env.chat", trust: environment.TrustThirdParty},
		&provEnv{id: "test.env.calendar", blind: true},
	)
	ctx := context.Background()

	// What runLoop does between observe and decide.
	ws := stepObserve(ctx, sc)
	sc.observed = &ws

	if _, paused := stepDecide(ctx, sc, threeActions(), nil); !paused {
		t.Fatal("a plan built from somebody else's writing did not escalate")
	}

	cp := checkpointFromState(t, sc)
	if cp.WorldState == nil {
		t.Fatal("escalation checkpoint has no WorldState")
	}
	got := cp.WorldState
	if !reflect.DeepEqual(got.Blind, []string{"test.env.calendar"}) {
		t.Errorf("Blind = %v, want [test.env.calendar]", got.Blind)
	}
	if len(got.Observations) != len(ws.Observations) {
		t.Fatalf("observations = %d, want %d", len(got.Observations), len(ws.Observations))
	}
	for i, want := range ws.Observations {
		g := got.Observations[i]
		if g.EnvID != want.EnvID || g.Trust != want.Trust {
			t.Errorf("observation %d = EnvID %q Trust %q, want %q %q", i, g.EnvID, g.Trust, want.EnvID, want.Trust)
		}
		if !reflect.DeepEqual(g.State, want.State) {
			t.Errorf("observation %d State = %#v, want %#v", i, g.State, want.State)
		}
	}
	if got.Observations[0].Trust != environment.TrustThirdParty {
		t.Errorf("recorded Trust = %q, want %q", got.Observations[0].Trust, environment.TrustThirdParty)
	}
}

// A budget pause is not a judgement about the world, and records none — even
// when an earlier iteration left an observation behind.
func TestBudgetPauseRecordsNoWorldState(t *testing.T) {
	sc := decideFixture(t, coreagent.AuthorityBounds{MaxAutonomousActions: coreagent.UnlimitedActions})
	sc.svc.budget = &recordingBudget{limit: 100, spent: 100}
	stale := smallWorldState()
	sc.observed = &stale

	if !sc.svc.pauseIfBudgetExhausted(context.Background(), sc) {
		t.Fatal("an exhausted budget did not pause")
	}

	if cp := checkpointFromState(t, sc); cp.WorldState != nil {
		t.Errorf("budget checkpoint WorldState = %+v, want nil", cp.WorldState)
	}
}
