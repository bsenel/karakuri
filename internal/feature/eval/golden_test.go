package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/feature/loop"
)

const goldenPath = "testdata/golden.v1.json"

// phase25VerdictIsPass is the verdict parser as it was before Phase 25: it
// lowercases the whole reply and passes it if any positive word appears
// anywhere, so "not met", "does not pass" and "not approved" all read as a
// pass. The golden set exists to catch a regression back to this.
func phase25VerdictIsPass(reply string) bool {
	lower := strings.ToLower(reply)
	for _, p := range []string{"pass", "met", "approved", "yes"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func loadGolden(t *testing.T) GoldenSet {
	t.Helper()
	set, err := LoadGoldenSet(goldenPath)
	if err != nil {
		t.Fatalf("LoadGoldenSet(%q): %v", goldenPath, err)
	}
	return set
}

func TestGoldenSetLoads(t *testing.T) {
	set := loadGolden(t)

	if set.Version < 1 {
		t.Errorf("version = %d, want >= 1", set.Version)
	}
	if n := len(set.Entries); n < 12 || n > 30 {
		t.Fatalf("entries = %d, want 12 to 30", n)
	}

	seen := map[string]bool{}
	negatedRejection, approval := false, false
	for i, e := range set.Entries {
		if e.ID == "" {
			t.Errorf("entry %d: empty id", i)
		}
		if seen[e.ID] {
			t.Errorf("entry %d: duplicate id %q", i, e.ID)
		}
		seen[e.ID] = true

		if e.Provenance == "" {
			t.Errorf("entry %q: empty provenance", e.ID)
		}
		switch e.Label {
		case choiceApprove, choiceReject, choiceModify:
		default:
			t.Errorf("entry %q: label %q, want approve, reject or modify", e.ID, e.Label)
		}
		if e.Reply == "" {
			t.Errorf("entry %q: empty reply", e.ID)
		}
		if e.Provenance == "constructed" && e.Note == "" {
			t.Errorf("entry %q: constructed entry has no note saying what it pins", e.ID)
		}

		if e.Label == choiceApprove {
			approval = true
		} else if phase25VerdictIsPass(e.Reply) {
			// A rejection worded with a positive word the pre-Phase-25
			// parser would have taken for a pass.
			negatedRejection = true
		}
	}
	if !negatedRejection {
		t.Error("no reject or modify entry with a negated reply (one containing pass, met, approved or yes)")
	}
	if !approval {
		t.Error("no approve entry")
	}
}

func TestGatePassesWithVerdictIsPass(t *testing.T) {
	set := loadGolden(t)

	res := Gate(set, loop.VerdictIsPass)
	if !res.Pass {
		t.Fatalf("Gate(VerdictIsPass).Pass = false; agreement %v, baseline %v, disagreements %v",
			res.Agreement, res.Baseline, res.Disagreements)
	}
	if res.Agreement < set.Baseline {
		t.Errorf("agreement = %v, want >= baseline %v", res.Agreement, set.Baseline)
	}
	if res.N != len(set.Entries) {
		t.Errorf("N = %d, want %d", res.N, len(set.Entries))
	}

	// The baseline is the agreement the shipped parser actually reaches on the
	// set, not a number somebody picked: recompute it without Gate.
	agreed := 0
	for _, e := range set.Entries {
		if loop.VerdictIsPass(e.Reply) == (e.Label == choiceApprove) {
			agreed++
		}
	}
	if got := float64(agreed) / float64(len(set.Entries)); got != set.Baseline {
		t.Errorf("baseline = %v, but VerdictIsPass agrees on %v of the set", set.Baseline, got)
	}
}

// TestGateGoesRedWithPhase25Parser is the acceptance test for the golden set:
// the parser Phase 25 fixed must fail the gate.
func TestGateGoesRedWithPhase25Parser(t *testing.T) {
	set := loadGolden(t)

	res := Gate(set, phase25VerdictIsPass)
	if res.Pass {
		t.Fatalf("Gate(phase25VerdictIsPass).Pass = true; agreement %v, baseline %v",
			res.Agreement, res.Baseline)
	}
	if len(res.Disagreements) == 0 {
		t.Error("Gate(phase25VerdictIsPass) reported no disagreements")
	}
	if res.Agreement >= set.Baseline {
		t.Errorf("agreement = %v, want below baseline %v", res.Agreement, set.Baseline)
	}
}

func TestLoadGoldenSetRejectsMissingVersionOrProvenance(t *testing.T) {
	const entry = `{"id":"e1","title":"t","criterion":"c","actions":"1. shell.run\n","label":"approve","reply":"PASS"%s}`
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "valid",
			body: `{"version":1,"baseline":1,"entries":[` +
				strings.Replace(entry, "%s", `,"provenance":"constructed","note":"n"`, 1) + `]}`,
		},
		{
			name: "missing version",
			body: `{"baseline":1,"entries":[` +
				strings.Replace(entry, "%s", `,"provenance":"constructed","note":"n"`, 1) + `]}`,
			wantErr: true,
		},
		{
			name: "missing provenance",
			body: `{"version":1,"baseline":1,"entries":[` +
				strings.Replace(entry, "%s", `,"note":"n"`, 1) + `]}`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "golden.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadGoldenSet(path)
			if tc.wantErr && err == nil {
				t.Error("LoadGoldenSet: want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("LoadGoldenSet: %v", err)
			}
		})
	}
}

func TestLoadGoldenSetRejectsMalformedSets(t *testing.T) {
	const ok = `{"id":"e1","label":"approve","reply":"PASS","provenance":"constructed"}`
	cases := map[string]string{
		"no entries":        `{"version":1,"baseline":1,"entries":[]}`,
		"baseline above 1":  `{"version":1,"baseline":1.5,"entries":[` + ok + `]}`,
		"baseline below 0":  `{"version":1,"baseline":-0.1,"entries":[` + ok + `]}`,
		"missing id":        `{"version":1,"baseline":1,"entries":[{"label":"approve","reply":"PASS","provenance":"p"}]}`,
		"missing reply":     `{"version":1,"baseline":1,"entries":[{"id":"e1","label":"approve","provenance":"p"}]}`,
		"unknown label":     `{"version":1,"baseline":1,"entries":[{"id":"e1","label":"maybe","reply":"PASS","provenance":"p"}]}`,
		"duplicate id":      `{"version":1,"baseline":1,"entries":[` + ok + `,` + ok + `]}`,
		"not json":          `{"version":1,`,
		"version below one": `{"version":0,"baseline":1,"entries":[` + ok + `]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "golden.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadGoldenSet(path); err == nil {
				t.Error("LoadGoldenSet: want error, got nil")
			}
		})
	}
}

func TestGateEmptySetFails(t *testing.T) {
	res := Gate(GoldenSet{Version: 1, Baseline: 0}, loop.VerdictIsPass)
	if res.Pass || res.Agreement != 0 || res.N != 0 {
		t.Errorf("Gate(empty) = %+v, want N 0, agreement 0, not passing", res)
	}
}

func TestGateReportsEachDisagreement(t *testing.T) {
	set := GoldenSet{Version: 1, Baseline: 0.5, Entries: []GoldenEntry{
		{ID: "a", Label: choiceApprove, Reply: "PASS"},
		{ID: "b", Label: choiceModify, Reply: "PASS"},
	}}
	res := Gate(set, loop.VerdictIsPass)
	if res.N != 2 || res.Agreed != 1 || res.Agreement != 0.5 || !res.Pass {
		t.Errorf("Gate = %+v, want N 2, agreed 1, agreement 0.5, pass", res)
	}
	if want := []string{"b: label=modify verdict=true"}; len(res.Disagreements) != 1 || res.Disagreements[0] != want[0] {
		t.Errorf("Disagreements = %q, want %q", res.Disagreements, want)
	}
}
