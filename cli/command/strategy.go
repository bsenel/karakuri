package command

import "github.com/spf13/cobra"

func strategyCmd() *cobra.Command {
	var idea, fromTicket, contextFile string
	cmd := &cobra.Command{
		Use:   "strategy",
		Short: "Run strategy pipeline",
		RunE: func(_ *cobra.Command, _ []string) error {
			input := idea
			if fromTicket != "" {
				input = "ticket:" + fromTicket
			}
			if contextFile != "" {
				input += " context:" + contextFile
			}
			return runPipeline("strategy", input, "")
		},
	}
	cmd.Flags().StringVar(&idea, "idea", "", "Product idea or concept")
	cmd.Flags().StringVar(&fromTicket, "from-ticket", "", "Source ticket provider:id")
	cmd.Flags().StringVar(&contextFile, "context", "", "Additional context file")
	return cmd
}
