package command

import "github.com/spf13/cobra"

func deliveryCmd() *cobra.Command {
	var task, fromDiscovery, fromTicket, contextFile string
	cmd := &cobra.Command{
		Use:   "delivery",
		Short: "Run delivery pipeline with worktree isolation",
		RunE: func(_ *cobra.Command, _ []string) error {
			input := task
			if fromTicket != "" {
				input = "ticket:" + fromTicket
			}
			return runPipeline("delivery", input, fromDiscovery)
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "Delivery task description")
	cmd.Flags().StringVar(&fromDiscovery, "from-discovery", "", "Parent discovery session SHA")
	cmd.Flags().StringVar(&fromTicket, "from-ticket", "", "Source ticket")
	cmd.Flags().StringVar(&contextFile, "context", "", "Context file")
	return cmd
}
