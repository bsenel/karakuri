package command

import (
	"io"
	"strings"
	"testing"
)

// `krk auth login` is where "not logged in" sends a new user, so a login that
// is missing a flag names every way in and shows the command to type.
func TestLoginMissingFlagSaysWhatToType(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{
			[]string{"auth", "login"},
			[]string{"--id is required", "krk auth login --id admin --password-stdin < password.txt", "--sso", "--refresh-token"},
		},
		{
			[]string{"auth", "login", "--id", "alice"},
			[]string{"--password-stdin is required", "shell history", "krk auth login --id alice --password-stdin < password.txt"},
		},
	} {
		root := NewRoot()
		root.SetArgs(append([]string{"--api-url", "http://127.0.0.1:1"}, tc.args...))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Fatalf("krk %v succeeded, want an error", tc.args)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("krk %v error = %q, want it to mention %q", tc.args, err, want)
			}
		}
	}
}
