package command

import (
	"strings"
	"testing"
)

// A DSN without its driver prefix is the first thing a new user gets wrong;
// the error has to show the shape to type, not only name it.
func TestParseDSNErrorShowsExample(t *testing.T) {
	for _, in := range []string{"karakuri.db", ":./karakuri.db", "sqlite:"} {
		_, _, err := parseDSN(in)
		if err == nil {
			t.Fatalf("parseDSN(%q) = nil error, want one", in)
		}
		for _, want := range []string{`"` + in + `"`, "<driver>:<dsn>", "sqlite:./karakuri.db", "postgres:postgres://"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("parseDSN(%q) error %q does not contain %q", in, err, want)
			}
		}
	}
	if _, _, err := parseDSN("postgres:postgres://u:p@h:5432/db"); err != nil {
		t.Errorf("parseDSN(valid) = %v, want nil", err)
	}
}

// A name that matches nothing is usually a typo; the error names the command
// that lists the names that do exist.
func TestResolveContainerMissingNamePointsAtList(t *testing.T) {
	twoTenants(t)

	for _, kind := range []string{"org", "team", "project"} {
		_, err := resolveContainer(kind, "widgets", "")
		if err == nil {
			t.Fatalf("resolveContainer(%s, widgets) = nil error, want one", kind)
		}
		for _, want := range []string{"no " + kind + ` called "widgets"`, "krk " + kind + " list"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("resolveContainer(%s, widgets) error %q does not contain %q", kind, err, want)
			}
		}
	}
}

func TestTwinBindingsSetErrorShowsExample(t *testing.T) {
	cmd := twinBindingsCmd()
	cmd.SetArgs([]string{"t_7f2a", "--set", "versioncontrol"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil {
		t.Fatal("bindings --set versioncontrol = nil error, want one")
	}
	for _, want := range []string{`"versioncontrol"`, "slot=instance", "--set versioncontrol=acme_github", "krk twin bindings t_7f2a"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
