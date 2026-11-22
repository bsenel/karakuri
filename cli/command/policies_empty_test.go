package command

import "testing"

// An empty policy list names the command that grants a role, as the other
// empty lists name the command that makes their rows.
func TestAuthPoliciesListEmptyNamesTheGrantCommand(t *testing.T) {
	hint := "Grant one with: krk auth bindings add --principal <id> --role <role>\n"
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"auth", "policies", "list"}, "No policies. " + hint},
		{[]string{"auth", "policies", "list", "--principal", "alice"}, "No grants reach principal \"alice\". " + hint},
	}
	for _, tc := range cases {
		apiURL := startListServer(t, `[]`)
		_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL}, tc.args...)...)
		if err != nil {
			t.Fatalf("krk %v: %v", tc.args, err)
		}
		if stderr != tc.want {
			t.Errorf("krk %v wrote %q to stderr, want %q", tc.args, stderr, tc.want)
		}
	}

	apiURL := startListServer(t, `[{"id":"p1"}]`)
	_, stderr, err := runKrkSplit("--api-url", apiURL, "auth", "policies", "list")
	if err != nil {
		t.Fatal(err)
	}
	if stderr != "" {
		t.Errorf("a non-empty list wrote %q to stderr, want nothing", stderr)
	}
}
