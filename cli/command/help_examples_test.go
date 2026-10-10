package command

import (
	"strings"
	"testing"
)

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
