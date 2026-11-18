package command

import (
	"strings"
	"testing"
)

// whoami and policies list had no Example, and --principal did not say that
// leaving it empty lists every policy.
func TestAuthHelpGivesExamplesAndPrincipalDefault(t *testing.T) {
	if ex := authWhoamiCmd().Example; !strings.Contains(ex, "krk auth whoami") {
		t.Errorf("whoami Example = %q, want a krk auth whoami line", ex)
	}
	policies := authPoliciesCmd()
	list, _, err := policies.Find([]string{"list"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list.Example, "--principal alice") {
		t.Errorf("policies list Example = %q, want a --principal line", list.Example)
	}
	usage := list.Flags().Lookup("principal").Usage
	if !strings.Contains(usage, "default: list every policy") {
		t.Errorf("--principal usage %q does not say what empty means", usage)
	}
}
