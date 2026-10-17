package command

import (
	"io"
	"strings"
	"testing"
)

// An empty pipe (an unset $PW, an empty file) is not a missing flag, so the
// error says what was read and shows how a password gets onto stdin.
func TestLoginEmptyStdinSaysHowToPipe(t *testing.T) {
	for _, in := range []string{"", "\n", "\r\n"} {
		root := NewRoot()
		root.SetArgs([]string{"--api-url", "http://127.0.0.1:1", "auth", "login", "--id", "alice", "--password-stdin"})
		root.SetIn(strings.NewReader(in))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Fatalf("login with stdin %q succeeded, want an error", in)
		}
		for _, want := range []string{"no password on stdin", "was empty", "--password-stdin < password.txt", "not empty"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("login with stdin %q error = %q, want it to mention %q", in, err, want)
			}
		}
	}
}

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

// --password-stdin with nothing piped in says how to pipe the password, for
// both the login and the user it creates.
func TestPasswordStdinEmptySaysHowToPipe(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "login", "--id", "alice", "--password-stdin"},
		{"auth", "users", "add", "--id", "bob", "--password-stdin"},
	} {
		root := NewRoot()
		root.SetArgs(append([]string{"--api-url", "http://127.0.0.1:1"}, args...))
		root.SetIn(strings.NewReader(""))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil {
			t.Fatalf("krk %v succeeded, want an error", args)
		}
		for _, want := range []string{"no password on stdin", "was empty", "--password-stdin < password.txt", `echo "$PW" |`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("krk %v error = %q, want it to mention %q", args, err, want)
			}
		}
	}
}

// Creating a principal or a binding with a flag missing gets the same help as
// a login does: the command to type, not only the name of the flag.
func TestAuthAddMissingFlagSaysWhatToType(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{
			[]string{"auth", "users", "add"},
			[]string{"--id is required", "krk auth users add --id alice --roles operator --password-stdin", "--service-account"},
		},
		{
			[]string{"auth", "users", "add", "--id", "bob"},
			[]string{"--password-stdin is required", "shell history", "krk auth users add --id bob --password-stdin", "--service-account"},
		},
		{
			[]string{"auth", "bindings", "add", "--principal", "alice"},
			[]string{"--principal and --role are required", "krk auth bindings add --principal alice --role operator", "krk auth users list"},
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
