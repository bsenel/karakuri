package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func digestCmd() *cobra.Command {
	var since, scope, repo, channel string
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Generate digest",
		RunE: func(_ *cobra.Command, _ []string) error {
			sess, err := api.CreateSession("autonomous", "digest:"+scope+":"+since, "")
			if err != nil {
				return err
			}
			sha, _ := sess["sha"].(string)
			_, _, _ = api.Post("/auto/run", nil)
			data, _, _ := api.Get("/sessions/" + sha)
			_, _ = repo, channel
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "24h", "Duration")
	cmd.Flags().StringVar(&scope, "scope", "all", "commits|prs|slack|all")
	cmd.Flags().StringVar(&repo, "repo", "", "Repository")
	cmd.Flags().StringVar(&channel, "channel", "", "Slack channel")
	return cmd
}
