package command

import (
	"strings"
	"testing"
)

// Each refusal of the container flags names the fix, not only the rule.
func TestContainerScopeRefusalsSayWhatToDo(t *testing.T) {
	twoTenants(t)

	cases := []struct {
		name               string
		org, team, project string
		want               []string
	}{
		{name: "a team with no org", team: "eng", want: []string{"--team eng --org acme", "krk org list"}},
		{name: "a project qualified by an org", project: "delta", org: "acme", want: []string{"--project delta"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := containerScope(tc.org, tc.team, tc.project)
			if err == nil {
				t.Fatal("want an error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("err = %v, want it to contain %q", err, w)
				}
			}
		})
	}

	_, err := resolveContainer("team", "eng", "")
	if err == nil || !strings.Contains(err.Error(), "krk team list") {
		t.Errorf("err = %v, want it to point at krk team list", err)
	}
}
