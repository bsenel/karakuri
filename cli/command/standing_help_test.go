package command

import (
	"strings"
	"testing"
)

// A first-time reader of krk objective standing has to know what leaving a
// cadence flag empty does and what unit a budget is in; the usage says both.
func TestObjectiveStandingHelpSaysWhatEmptyMeans(t *testing.T) {
	cmd := objectiveStandingCmd()
	requireExample(t, cmd)

	wants := map[string][]string{
		"sense":                {"15m", "empty = never sense"},
		"every":                {"1h", "not with --cron or --daily-at"},
		"resync":               {"24h", "empty = "},
		"min-interval":         {"10m", "empty = no floor"},
		"budget-daily":         {"units krk cost report totals", "0 = no ceiling"},
		"budget-per-reconcile": {"same units", "0 = uncapped", "--budget-daily bounds the day"},
	}
	for flag, phrases := range wants {
		usage := cmd.Flags().Lookup(flag).Usage
		for _, want := range phrases {
			if !strings.Contains(usage, want) {
				t.Errorf("--%s usage %q does not mention %q", flag, usage, want)
			}
		}
	}
}

// A second pause does not overwrite the first reason, and an empty one is
// stored as nothing; the --reason usage has to say both.
func TestObjectivePauseHelpSaysWhatEmptyReasonMeans(t *testing.T) {
	cmd := objectivePauseCmd()
	requireExample(t, cmd)

	usage := cmd.Flags().Lookup("reason").Usage
	for _, want := range []string{"empty = no reason recorded", "keeps its first reason"} {
		if !strings.Contains(usage, want) {
			t.Errorf("--reason usage %q does not mention %q", usage, want)
		}
	}
}
