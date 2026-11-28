package command

import "testing"

// An empty policy list names the command that grants a role, as the other
// empty lists name the command that makes their rows. With --principal the
// hint names that principal, so it can be copied as is.
func TestAuthPoliciesListEmptyNamesTheGrantCommand(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"auth", "policies", "list"}, "No policies. Grant one with: krk auth bindings add --principal <id> --role <role>\n"},
		{[]string{"auth", "policies", "list", "--principal", "alice"}, "No grants reach principal \"alice\". Grant one with: krk auth bindings add --principal alice --role <role>\n"},
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
