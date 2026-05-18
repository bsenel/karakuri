package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func promoteCmd() *cobra.Command {
	var fromAudit, fromSlack, fromResearch, via string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote autonomous artifacts to pipeline mode",
		RunE: func(_ *cobra.Command, _ []string) error {
			from := fromAudit
			if fromSlack != "" {
				from = fromSlack
			}
			if fromResearch != "" {
				from = fromResearch
			}
			data, _, err := api.Post("/sessions/"+from+"/promote", map[string]any{
				"via": via, "dry_run": dryRun,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromAudit, "from-audit", "", "Source audit session SHA")
	cmd.Flags().StringVar(&fromSlack, "from-slack", "", "Source slack digest SHA")
	cmd.Flags().StringVar(&fromResearch, "from-research", "", "Source research SHA")
	cmd.Flags().StringVar(&via, "via", "strategy", "strategy|discovery|delivery")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Dry run")
	return cmd
}
