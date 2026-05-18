package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func reviewCmd() *cobra.Command {
	var pr, repo, style string
	var all bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "On-demand PR review",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Post("/research", map[string]string{
				"topic": "pr-review:" + pr + " repo:" + repo + " style:" + style + " all:" + boolStr(all),
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&pr, "pr", "", "PR provider:id")
	cmd.Flags().BoolVar(&all, "all", false, "Review all open PRs")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository owner/repo")
	cmd.Flags().StringVar(&style, "style", "both", "comment|summary|both")
	return cmd
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
