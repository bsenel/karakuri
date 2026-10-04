package command

import "testing"

// A command that changes something and has no body to print says what it
// changed, in the words the rest of the command uses for it.
func TestSuccessMessagesNameWhatChanged(t *testing.T) {
	_, apiURL := startEvalServer(t)

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"quota", "unset", "llm-tokens"}, "limit for tier llm-tokens is back to what configuration says\n"},
		{[]string{"report", "delete", "r1"}, "digest schedule r1 deleted\n"},
	} {
		got := runKrk(t, append([]string{"--api-url", apiURL}, tc.args...)...)
		if got != tc.want {
			t.Errorf("krk %v printed %q, want %q", tc.args, got, tc.want)
		}
	}
}
