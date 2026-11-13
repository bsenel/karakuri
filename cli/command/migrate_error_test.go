package command

import (
	"strings"
	"testing"
)

// An unknown driver must name the drivers krk migrate takes and show one.
func TestMigrateUnknownDriverNamesTheSupportedOnes(t *testing.T) {
	cmd := migrateCmd()
	cmd.SetArgs([]string{"--from", "mysql:root@/karakuri", "--to", "sqlite:" + t.TempDir() + "/k.db"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("krk migrate with a mysql source returned no error")
	}
	for _, want := range []string{"--from", `driver "mysql" is not supported`, "use sqlite or postgres", "e.g. sqlite:./karakuri.db"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
