package command

import (
	"fmt"
	"os"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

var (
	apiURL string
	output string
	api    *client.Client
)

func Execute() {
	if err := NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "krk",
		Short: "Karakuri CLI — autonomous agent platform",
		// A rejected API call is not a usage mistake. Dumping the flag list
		// after "forbidden: no policy grants twin:bind" buries the one line
		// that explains what happened.
		SilenceUsage: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			// An unknown format used to fall through to pretty, so a typo
			// such as "--output jsno" fed indented JSON to a script.
			switch output {
			case "json", "pretty", "quiet":
			default:
				return fmt.Errorf("invalid --output %q: use json, pretty or quiet", output)
			}
			api = client.New(apiURL)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080/api/v1", "API base URL")
	root.PersistentFlags().StringVar(&output, "output", "pretty", "Output format: json|pretty|quiet")

	root.AddCommand(
		twinCmd(),
		objectiveCmd(),
		reportCmd(),
		loopCmd(),
		checkpointCmd(),
		memoryCmd(),
		artifactCmd(),
		domainCmd(),
		researchCmd(),
		autoCmd(),
		migrateCmd(),
		webCmd(),
		auditCmd(),
		authCmd(),
		quotaCmd(),
		orgCmd(),
		teamCmd(),
		projectCmd(),
		costCmd(),
		evalCmd(),
	)
	explainArgs(root)
	return root
}

// explainArgs makes a wrong argument count say what was expected. Cobra's own
// "accepts 1 arg(s), received 0" names the problem but not the argument, and
// SilenceUsage keeps the usage line from following it.
//
// A missing required flag gets the same two lines. Cobra checks those after the
// arguments and reports only `required flag(s) "reason" not set`, which names
// the flag but not what it takes or where the examples are.
//
// A command group with no Args of its own keeps it nil: cobra runs its
// `unknown command "bogus" for "krk"` check only while Args is nil.
func explainArgs(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		explainArgs(sub)
	}
	validate := cmd.Args
	if validate == nil && cmd.HasSubCommands() {
		return
	}
	cmd.Args = func(c *cobra.Command, args []string) error {
		if validate != nil {
			if err := validate(c, args); err != nil {
				return fmt.Errorf("%w\nUsage: %s\nRun '%s --help' for details", err, c.UseLine(), c.CommandPath())
			}
		}
		if err := c.ValidateRequiredFlags(); err != nil {
			return fmt.Errorf("%w\nUsage: %s\nRun '%s --help' for the flags and examples", err, c.UseLine(), c.CommandPath())
		}
		return nil
	}
}
