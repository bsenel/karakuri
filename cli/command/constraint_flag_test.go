package command

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// A constraint is a sentence. --constraint must reach added_constraints as
// the operator typed it, never split at its commas.
func TestConstraintFlagKeepsCommas(t *testing.T) {
	const sentence = "open a draft PR, not a merge, and stop"
	commands := map[string]struct {
		args []string
		path string
	}{
		"checkpoint resolve": {[]string{"checkpoint", "resolve", "cp_123"}, "/api/v1/checkpoints/cp_123/resolve"},
		"loop resume":        {[]string{"loop", "resume", "loop_123"}, "/api/v1/loops/loop_123/resume"},
	}
	cases := map[string]struct {
		flags []string
		want  []string
	}{
		"one constraint with commas": {
			[]string{"--constraint", sentence},
			[]string{sentence},
		},
		"two constraints in order": {
			[]string{"--constraint", sentence, "--constraint", "touch only docs/, nothing else"},
			[]string{sentence, "touch only docs/, nothing else"},
		},
	}
	for cmdName, cmd := range commands {
		for caseName, tc := range cases {
			t.Run(cmdName+"/"+caseName, func(t *testing.T) {
				srv, apiURL := startCreateServer(t, http.StatusOK, `{}`)

				args := append([]string{"--api-url", apiURL, "--output", "quiet"}, cmd.args...)
				args = append(args, "--decision", "modify")
				args = append(args, tc.flags...)
				if _, stderr, err := runKrkSplit(args...); err != nil {
					t.Fatalf("krk %s: %v\n%s", cmdName, err, stderr)
				}
				if srv.calls != 1 || srv.method != http.MethodPost || srv.path != cmd.path {
					t.Fatalf("server got %d requests, last %s %s; want one POST %s",
						srv.calls, srv.method, srv.path, cmd.path)
				}
				var mods struct {
					AddedConstraints []string `json:"added_constraints"`
				}
				if err := json.Unmarshal(srv.body["modifications"], &mods); err != nil {
					t.Fatalf("modifications %s: %v", srv.body["modifications"], err)
				}
				if !reflect.DeepEqual(mods.AddedConstraints, tc.want) {
					t.Errorf("added_constraints = %q, want %q", mods.AddedConstraints, tc.want)
				}
			})
		}
	}
}
