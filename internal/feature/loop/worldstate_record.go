package loop

import (
	"encoding/json"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/loop"
)

const (
	// maxRecordedObservationBytes bounds one observation's State, as JSON, in
	// the world state a checkpoint records.
	maxRecordedObservationBytes = 16 << 10
	// maxRecordedWorldStateBytes bounds the running total across all of them.
	maxRecordedWorldStateBytes = 256 << 10
	// recordedPreviewBytes bounds the _preview of a truncated State.
	recordedPreviewBytes = 4096
)

// recordedWorldState returns the copy of ws a checkpoint records: what the
// planner saw when it escalated, bounded so one chatty environment cannot
// bloat the checkpoint row.
//
// It returns a copy and never mutates ws. Blind, Version and Timestamp are
// copied unchanged. Each observation keeps EnvID, Version, Timestamp and Trust
// unchanged. Trust in particular must survive (ADR 021): a replay that forgot
// which text was third-party would run without the notice the original
// planner got.
//
// An observation whose State marshals to more than maxRecordedObservationBytes
// of JSON has its State replaced by
//
//	{"_truncated": true, "_original_bytes": n, "_preview": <first 4096 bytes of that JSON, cut on a UTF-8 boundary>}
//
// Once the running total passes maxRecordedWorldStateBytes, every later
// observation's State becomes {"_truncated": true, "_original_bytes": n}.
//
// A State that does not marshal is recorded as truncated with _original_bytes
// 0: there is no JSON to measure or preview.
func recordedWorldState(ws loop.WorldState) *loop.WorldState {
	out := &loop.WorldState{
		Version:   ws.Version,
		Timestamp: ws.Timestamp,
		Blind:     slices.Clone(ws.Blind),
	}
	if ws.Observations != nil {
		out.Observations = make([]environment.Observation, len(ws.Observations))
	}
	total := 0
	for i, obs := range ws.Observations {
		b, err := json.Marshal(obs.State)
		switch {
		case err != nil:
			obs.State = map[string]any{"_truncated": true, "_original_bytes": 0}
		case total > maxRecordedWorldStateBytes:
			obs.State = map[string]any{"_truncated": true, "_original_bytes": len(b)}
		case len(b) > maxRecordedObservationBytes:
			preview := jsonPreview(b)
			obs.State = map[string]any{"_truncated": true, "_original_bytes": len(b), "_preview": preview}
			total += len(preview)
		default:
			obs.State = maps.Clone(obs.State)
			total += len(b)
		}
		out.Observations[i] = obs
	}
	return out
}

// jsonPreview returns at most recordedPreviewBytes of b, cut back to the start
// of a rune so the preview never ends inside a UTF-8 sequence.
func jsonPreview(b []byte) string {
	if len(b) <= recordedPreviewBytes {
		return string(b)
	}
	n := recordedPreviewBytes
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return string(b[:n])
}
