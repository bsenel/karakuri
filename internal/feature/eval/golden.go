package eval

import (
	"encoding/json"
	"fmt"
	"os"
)

// GoldenSet is a versioned set of recorded judge replies with the human label
// each should agree with, and the agreement the shipped parser must reach.
//
// It pins the verdict parser, not the judge: every reply is recorded, so
// scoring the set calls no model and a parser change that reads "not met" as a
// pass shows up as lost agreement before it ships.
type GoldenSet struct {
	Version  int           `json:"version"`
	Baseline float64       `json:"baseline"`
	Entries  []GoldenEntry `json:"entries"`
}

// GoldenEntry is one labelled judge reply. Title, Criterion and Actions are
// what the judge was shown; the gate reads only Label and Reply.
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

// LoadGoldenSet reads a golden set from path. A set that could not be scored
// the way it claims — no version, no entries, an entry without an id,
// provenance, reply or recognisable label, a repeated id, or a baseline
// outside [0,1] — is an error rather than a set the gate quietly passes.
func LoadGoldenSet(path string) (GoldenSet, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator- or test-chosen golden file path; never from a request
	if err != nil {
		return GoldenSet{}, err
	}
	var set GoldenSet
	if err := json.Unmarshal(b, &set); err != nil {
		return GoldenSet{}, fmt.Errorf("golden set %s: %w", path, err)
	}
	if set.Version < 1 {
		return GoldenSet{}, fmt.Errorf("golden set %s: version %d, want >= 1", path, set.Version)
	}
	if set.Baseline < 0 || set.Baseline > 1 {
		return GoldenSet{}, fmt.Errorf("golden set %s: baseline %v, want within [0,1]", path, set.Baseline)
	}
	if len(set.Entries) == 0 {
		return GoldenSet{}, fmt.Errorf("golden set %s: no entries", path)
	}
	seen := make(map[string]bool, len(set.Entries))
	for i, e := range set.Entries {
		switch {
		case e.ID == "":
			return GoldenSet{}, fmt.Errorf("golden set %s: entry %d has no id", path, i)
		case seen[e.ID]:
			return GoldenSet{}, fmt.Errorf("golden set %s: duplicate id %q", path, e.ID)
		case e.Provenance == "":
			return GoldenSet{}, fmt.Errorf("golden set %s: entry %q has no provenance", path, e.ID)
		case e.Reply == "":
			return GoldenSet{}, fmt.Errorf("golden set %s: entry %q has no reply", path, e.ID)
		case e.Label != decisionApprove && e.Label != decisionReject && e.Label != decisionModify:
			return GoldenSet{}, fmt.Errorf("golden set %s: entry %q has label %q, want approve, reject or modify", path, e.ID, e.Label)
		}
		seen[e.ID] = true
	}
	return set, nil
}

// GateResult says whether a verdict parser agrees with the golden labels at
// least as often as the set's baseline.
type GateResult struct {
	N, Agreed           int
	Agreement, Baseline float64
	Pass                bool
	Disagreements       []string
}

// Gate scores verdict against every entry in set, with the label mapping
// Calibrate uses: approve is a pass, reject and modify are not.
//
// What it measures is agreement between the parser, reading replies the judge
// already gave, and the humans' labels. That is not correctness: it says
// nothing about whether the judge would give those replies today, or whether
// the humans were right. It calls no model. An empty set fails, because
// agreement with nothing is not evidence the parser works.
func Gate(set GoldenSet, verdict func(reply string) bool) GateResult {
	res := GateResult{N: len(set.Entries), Baseline: set.Baseline}
	for _, e := range set.Entries {
		got := verdict(e.Reply)
		if got == humanApproves(e.Label) {
			res.Agreed++
			continue
		}
		res.Disagreements = append(res.Disagreements,
			fmt.Sprintf("%s: label=%s verdict=%t", e.ID, e.Label, got))
	}
	if res.N > 0 {
		res.Agreement = float64(res.Agreed) / float64(res.N)
		res.Pass = res.Agreement >= res.Baseline
	}
	return res
}
