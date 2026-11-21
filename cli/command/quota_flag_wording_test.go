package command

import (
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestQuotaFlagUsageMatchesSiblings(t *testing.T) {
	// Every other command's flag help starts with a capital letter; the
	// quota tree started lowercase, so its flags read as a different tool.
	var visit func(c *cobra.Command)
	visit = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" || f.Usage == "" {
				return
			}
			if unicode.IsLower([]rune(f.Usage)[0]) {
				t.Errorf("%s --%s usage %q starts lowercase", c.CommandPath(), f.Name, f.Usage)
			}
		})
		for _, sub := range c.Commands() {
			visit(sub)
		}
	}
	visit(quotaCmd())

	// "pending, approved or rejected" names the values but not what the flag
	// does with them, or what happens without it.
	usage := quotaRequestsListCmd().Flags().Lookup("status").Usage
	if !strings.Contains(usage, "Show only") || !strings.Contains(usage, "default") {
		t.Errorf("--status usage %q does not say it filters or name the default", usage)
	}
}
