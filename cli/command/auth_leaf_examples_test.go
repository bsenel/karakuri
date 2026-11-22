package command

import (
	"strings"
	"testing"
)

// logout, users list, roles list and catalog printed no Example in --help.
func TestAuthLeafCommandsShowExamples(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"logout"}, "krk auth logout"},
		{[]string{"users", "list"}, "krk auth users list"},
		{[]string{"roles", "list"}, "krk auth roles list"},
		{[]string{"catalog"}, "krk auth catalog"},
	}
	for _, tc := range cases {
		cmd, _, err := authCmd().Find(tc.args)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cmd.Example, tc.want) {
			t.Errorf("%v Example = %q, want it to contain %q", tc.args, cmd.Example, tc.want)
		}
	}
}
