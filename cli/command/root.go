package command

import (
	"os"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

var (
	apiURL string
	token  string
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
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			api = client.New(apiURL, token)
		},
	}
	root.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080/api/v1", "API base URL")
	root.PersistentFlags().StringVar(&token, "token", os.Getenv("KARAKURI_TOKEN"), "Bearer token")
	root.PersistentFlags().StringVar(&output, "output", "pretty", "Output format: json|pretty|quiet")

	root.AddCommand(
		twinCmd(),
		objectiveCmd(),
		loopCmd(),
		checkpointCmd(),
		memoryCmd(),
		artifactCmd(),
		domainCmd(),
		researchCmd(),
		autoCmd(),
		migrateCmd(),
		webCmd(),
	)
	return root
}
