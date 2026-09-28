package eval

import "time"

// GoldenSet is a versioned set of recorded judge replies with the human label
// each should agree with, and the agreement the shipped parser must reach.
type GoldenSet struct {
	Version  int           `json:"version"`
	Baseline float64       `json:"baseline"`
	Entries  []GoldenEntry `json:"entries"`
}

// GoldenEntry is one labelled judge reply.
type GoldenEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Criterion  string `json:"criterion"`
	Actions    string `json:"actions"`
	Label      string `json:"label"`
	Reply      string `json:"reply"`
	Provenance string `json:"provenance"`
	Note       string `json:"note,omitempty"`
}

// LoadGoldenSet reads a golden set from path.
func LoadGoldenSet(path string) (GoldenSet, error) {
	return GoldenSet{}, nil
}

// GateResult says whether a verdict parser agrees with the golden labels at
// least as often as the set's baseline.
type GateResult struct {
	N, Agreed           int
	Agreement, Baseline float64
	Pass                bool
	Disagreements       []string
}

// Gate scores verdict against every entry in set.
func Gate(set GoldenSet, verdict func(reply string) bool) GateResult {
	return GateResult{}
}

// ExportGolden turns a calibration report into golden entries.
func ExportGolden(report CalibrationReport, deployment string, at time.Time) []GoldenEntry {
	return nil
}
