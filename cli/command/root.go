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
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			api = client.New(apiURL)
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
func explainArgs(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := validate(c, args); err != nil {
				return fmt.Errorf("%w\nUsage: %s\nRun '%s --help' for details", err, c.UseLine(), c.CommandPath())
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		explainArgs(sub)
	}
}
