package command

import "testing"

// --output quiet means no output: a confirmation line is held back the same
// way a response body is.
func TestQuietSilencesSuccessMessages(t *testing.T) {
	_, apiURL := startEvalServer(t)

	for _, args := range [][]string{
		{"quota", "unset", "llm-tokens"},
		{"report", "delete", "r1"},
		{"objective", "unstanding", "o1"},
	} {
		got := runKrk(t, append([]string{"--api-url", apiURL, "--output", "quiet"}, args...)...)
		if got != "" {
			t.Errorf("krk --output quiet %v printed %q, want nothing", args, got)
		}
	}
}

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
		{[]string{"objective", "unstanding", "o1"}, "objective o1 is no longer standing\n"},
	} {
		got := runKrk(t, append([]string{"--api-url", apiURL}, tc.args...)...)
		if got != tc.want {
			t.Errorf("krk %v printed %q, want %q", tc.args, got, tc.want)
		}
	}
}
