package loop

import "github.com/bsenel/karakuri/internal/core/loop"

const (
	// maxRecordedObservationBytes bounds one observation's State, as JSON, in
	// the world state a checkpoint records.
	maxRecordedObservationBytes = 16 << 10
	// maxRecordedWorldStateBytes bounds the running total across all of them.
	maxRecordedWorldStateBytes = 256 << 10
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
func recordedWorldState(ws loop.WorldState) *loop.WorldState {
	return nil
}
