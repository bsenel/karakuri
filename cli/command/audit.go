package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	var env, service, since, threshold string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Environment audit",
		RunE: func(_ *cobra.Command, _ []string) error {
			sess, err := api.CreateSession("autonomous", "env-audit:"+env+":"+service, "")
			if err != nil {
				return err
			}
			sha, _ := sess["sha"].(string)
			data, _, _ := api.Get("/sessions/" + sha)
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&env, "env", "all", "dev|staging|prod|all")
	cmd.Flags().StringVar(&service, "service", "", "Service name")
	cmd.Flags().StringVar(&since, "since", "24h", "Duration")
	cmd.Flags().StringVar(&threshold, "threshold", "high", "Severity threshold")
	return cmd
}
