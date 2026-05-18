package command

import "github.com/spf13/cobra"

func discoveryCmd() *cobra.Command {
	var feature, fromStrategy, fromTicket, contextFile string
	cmd := &cobra.Command{
		Use:   "discovery",
		Short: "Run discovery pipeline",
		RunE: func(_ *cobra.Command, _ []string) error {
			input := feature
			if fromTicket != "" {
				input = "ticket:" + fromTicket
			}
			return runPipeline("discovery", input, fromStrategy)
		},
	}
	cmd.Flags().StringVar(&feature, "feature", "", "Feature name")
	cmd.Flags().StringVar(&fromStrategy, "from-strategy", "", "Parent strategy session SHA")
	cmd.Flags().StringVar(&fromTicket, "from-ticket", "", "Source ticket")
	cmd.Flags().StringVar(&contextFile, "context", "", "Context file")
	return cmd
}
