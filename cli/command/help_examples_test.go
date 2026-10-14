package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReportHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{reportCreateCmd(), reportListCmd(), reportPreviewCmd(), reportSendCmd(), reportDeleteCmd()} {
		requireExample(t, cmd)
	}

	// send and delete take an <id> that nothing else in their help explains.
	for _, cmd := range []*cobra.Command{reportSendCmd(), reportDeleteCmd()} {
		if !strings.Contains(cmd.Long, "krk report list") {
			t.Errorf("%s Long %q does not say where <id> comes from", cmd.Name(), cmd.Long)
		}
	}

	// preview refuses to run without --twin, so its example must show it.
	if example := reportPreviewCmd().Example; !strings.Contains(example, "--twin") {
		t.Errorf("preview Example %q does not show the required --twin", example)
	}
	usage := reportListCmd().Flags().Lookup("twin").Usage
	if !strings.Contains(usage, "krk twin list") || !strings.Contains(usage, "default") {
		t.Errorf("--twin usage %q does not say where an ID comes from or name the default", usage)
	}
}

func TestAuditHelp(t *testing.T) {
	requireExample(t, auditCmd())
	requireExample(t, auditExportCmd())

	// export refuses to run without --from and --to, so its example must
	// show both.
	example := auditExportCmd().Example
	if !strings.Contains(example, "--from") || !strings.Contains(example, "--to") {
		t.Errorf("audit export Example %q does not show the required --from and --to", example)
	}
}

func TestDomainHelp(t *testing.T) {
	requireExample(t, domainListCmd())
	requireExample(t, domainTestCmd())
	requireExample(t, domainCapabilitiesCmd())

	// "List capabilities" does not say whose; "Filter by domain" does not say
	// what value to pass or what happens without it.
	if short := domainCapabilitiesCmd().Short; short == "List capabilities" {
		t.Errorf("capabilities Short %q does not say what is listed", short)
	}
	usage := domainCapabilitiesCmd().Flags().Lookup("domain").Usage
	if !strings.Contains(usage, "krk domain list") || !strings.Contains(usage, "default") {
		t.Errorf("--domain usage %q does not say where an ID comes from or name the default", usage)
	}
}

func TestStandingHelp(t *testing.T) {
	for _, cmd := range standingCmds() {
		requireExample(t, cmd)
	}

	// pause asks for a reason in its Long, so its example must show the flag;
	// reconcile-status has one flag and the example should show it too.
	if example := objectivePauseCmd().Example; !strings.Contains(example, "--reason") {
		t.Errorf("pause Example %q does not show --reason", example)
	}
	if example := objectiveReconcileStatusCmd().Example; !strings.Contains(example, "--limit") {
		t.Errorf("reconcile-status Example %q does not show --limit", example)
	}
}

func TestContainerHelp(t *testing.T) {
	for _, parent := range []*cobra.Command{orgCmd(), teamCmd(), projectCmd()} {
		for _, cmd := range parent.Commands() {
			requireExample(t, cmd)
			if !strings.Contains(cmd.Example, "krk "+parent.Name()+" "+cmd.Name()) {
				t.Errorf("%s %s Example %q does not run the command it documents", parent.Name(), cmd.Name(), cmd.Example)
			}
		}
	}

	// A team name is only unique inside its organisation, so every team
	// example names one.
	for _, cmd := range teamCmd().Commands() {
		if !strings.Contains(cmd.Example, "--org") {
			t.Errorf("team %s Example %q does not show --org", cmd.Name(), cmd.Example)
		}
	}

	// "Twin id" does not say where the ID comes from.
	for _, cmd := range []*cobra.Command{projectAddCmd(), projectRemoveCmd()} {
		if usage := cmd.Flags().Lookup("twin").Usage; !strings.Contains(usage, "krk twin list") {
			t.Errorf("%s --twin usage %q does not say where an ID comes from", cmd.Name(), usage)
		}
	}
}
