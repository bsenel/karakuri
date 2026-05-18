package command

import (
	"os"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

var (
	apiURL   string
	token    string
	executor string
	output   string
	api      *client.Client
)

func Execute() {
	if err := NewRoot().Execute(); err != nil {
		os.Exit(1)
	}
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "krk",
		Short: "Karakuri CLI — orchestrate LLM agent pipelines",
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			api = client.New(apiURL, token)
		},
	}
	root.PersistentFlags().StringVar(&apiURL, "api-url", "http://localhost:8080/api/v1", "API base URL")
	root.PersistentFlags().StringVar(&token, "token", os.Getenv("KARAKURI_TOKEN"), "Bearer token")
	root.PersistentFlags().StringVar(&executor, "executor", "local", "Executor backend")
	root.PersistentFlags().StringVar(&output, "output", "pretty", "Output format: json|pretty|quiet")

	root.AddCommand(
		strategyCmd(), discoveryCmd(), deliveryCmd(),
		reviewCmd(), researchCmd(), auditCmd(), digestCmd(), promoteCmd(),
		autoCmd(), statusCmd(), artifactsCmd(), resolveCmd(), historyCmd(), diffCmd(),
	)
	return root
}

func runPipeline(mode, input, parentSHA string) error {
	sess, err := api.CreateSession(mode, input, parentSHA)
	if err != nil {
		return err
	}
	sha, _ := sess["sha"].(string)
	if err := api.RunSession(sha); err != nil {
		return err
	}
	if err := api.WaitForCompletion(sha, 5*time.Minute); err != nil {
		return err
	}
	data, _, _ := api.Get("/sessions/" + sha)
	client.PrintOutput(data, output)
	return nil
}
