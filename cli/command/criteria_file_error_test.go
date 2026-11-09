package command

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A --criteria-file that is missing or not a YAML list is refused before any
// request, and the error says what to check and names the example that parses.
func TestObjectiveCreateCriteriaFileErrorsSayWhatToDo(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := map[string]struct {
		path string
		want []string
	}{
		"missing":    {filepath.Join(dir, "nope.yaml"), []string{"no such file", "Check the path", exampleCriteriaFile}},
		"bad yaml":   {write("bad.yaml", "id: [unclosed\n"), []string{"did not find expected", "YAML list of criteria", "- id:", exampleCriteriaFile}},
		"not a list": {write("map.yaml", "id: p95\n"), []string{"cannot unmarshal", "YAML list of criteria", exampleCriteriaFile}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

			_, stderr, err := runKrkSplit(createArgs(apiURL, "--criteria-file", tc.path)...)

			if err == nil {
				t.Fatal("krk objective create exited zero with a bad --criteria-file")
			}
			seen := err.Error() + stderr
			for _, want := range tc.want {
				if !strings.Contains(seen, want) {
					t.Errorf("the user saw %q, which lacks %q", seen, want)
				}
			}
			if srv.calls != 0 {
				t.Errorf("a bad --criteria-file sent %d requests, want none", srv.calls)
			}
		})
	}
}
