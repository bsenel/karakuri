package command

import "testing"

// The list commands that came after printList say an empty list is empty in
// the same words the first four do, and stay silent on stderr otherwise.
func TestLaterEmptyListsSaySo(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"org", "list"}, "No orgs.\n"},
		{[]string{"team", "list"}, "No teams.\n"},
		{[]string{"project", "list"}, "No projects.\n"},
		{[]string{"report", "list"}, "No digest schedules.\n"},
		{[]string{"quota", "requests", "list"}, "No quota requests.\n"},
		{[]string{"domain", "list"}, "No domain packs.\n"},
		{[]string{"audit"}, "No audit entries.\n"},
		{[]string{"audit", "--violations-only"}, "No audit entries.\n"},
	} {
		for _, body := range []string{"null", "[]\n"} {
			apiURL := startListServer(t, body)
			_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL}, tc.args...)...)
			if err != nil {
				t.Fatalf("krk %v: %v", tc.args, err)
			}
			if stderr != tc.want {
				t.Errorf("krk %v on %q wrote %q to stderr, want %q", tc.args, body, stderr, tc.want)
			}
		}
		apiURL := startListServer(t, "null")
		_, stderr, err := runKrkSplit(append([]string{"--api-url", apiURL, "--output", "json"}, tc.args...)...)
		if err != nil {
			t.Fatalf("krk --output json %v: %v", tc.args, err)
		}
		if stderr != "" {
			t.Errorf("krk --output json %v wrote %q to stderr, want nothing", tc.args, stderr)
		}
	}
}
