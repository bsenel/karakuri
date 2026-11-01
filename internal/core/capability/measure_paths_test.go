package capability

import (
	"encoding/json"
	"testing"
)

// docs/roadmap.md, Phase 41 step 9: a verifier declares the paths its
// definition is read from, and a pass whose changed files touch one of them
// changed the measure. TouchesAnyGlob is that question, asked of plain paths
// and globs, so the core holds it with no pack's nouns.
func TestTouchesAnyGlob(t *testing.T) {
	cases := []struct {
		name    string
		changed []string
		globs   []string
		want    bool
	}{
		{"a double-star glob matches a nested file", []string{"internal/feature/loop/verify_test.go"}, []string{"**/*_test.go"}, true},
		{"a double-star glob matches a file at the root", []string{"main_test.go"}, []string{"**/*_test.go"}, true},
		{"a double-star in the middle matches any depth", []string{"web/src/components/deep/Panel.test.tsx"}, []string{"web/src/**/*.test.tsx"}, true},
		{"a double-star in the middle matches zero directories", []string{"web/src/App.test.tsx"}, []string{"web/src/**/*.test.tsx"}, true},
		{"an exact path matches itself", []string{"README.md", ".golangci.yml"}, []string{".golangci.yml"}, true},
		{"one changed file among many is enough", []string{"a.go", "b.go", "pkg/c_test.go"}, []string{"**/*_test.go"}, true},
		{"no changed file matches", []string{"internal/feature/loop/verify.go", "docs/roadmap.md"}, []string{"**/*_test.go", ".golangci.yml"}, false},
		{"an exact path does not match a longer one", []string{"sub/.golangci.yml"}, []string{".golangci.yml"}, false},
		{"a single star does not cross a directory", []string{"web/src/components/Panel.test.tsx"}, []string{"web/src/*.test.tsx"}, false},
		{"no globs declared touch nothing", []string{"pkg/c_test.go"}, nil, false},
		{"no changed files touch nothing", nil, []string{"**/*_test.go"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TouchesAnyGlob(tc.changed, tc.globs); got != tc.want {
				t.Errorf("TouchesAnyGlob(%q, %q) = %v, want %v", tc.changed, tc.globs, got, tc.want)
			}
		})
	}
}

// docs/roadmap.md, Phase 41 step 9: the declaration is on the wire as
// measure_paths beside metrics, and a capability that declares none writes
// nothing.
func TestMeasurePathsAreOnTheWire(t *testing.T) {
	declared, err := jsonOf(Capability{ID: "x.verify.t", MeasurePaths: []string{"**/*_test.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := declared["measure_paths"].([]any); !ok || len(got) != 1 || got[0] != "**/*_test.go" {
		t.Errorf("measure_paths = %#v, want [\"**/*_test.go\"]", declared["measure_paths"])
	}
	none, err := jsonOf(Capability{ID: "x.verify.t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := none["measure_paths"]; ok {
		t.Error("a capability with no measure paths writes measure_paths")
	}
}

func jsonOf(c Capability) (map[string]any, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(raw, &out)
}
