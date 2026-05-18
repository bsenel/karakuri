package command

import (
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func researchCmd() *cobra.Command {
	var ticket, topic, sources, depth string
	cmd := &cobra.Command{
		Use:   "research",
		Short: "On-demand research",
		RunE: func(_ *cobra.Command, _ []string) error {
			if ticket != "" {
				topic = "ticket:" + ticket
			}
			var srcs []string
			if sources != "" {
				srcs = strings.Split(sources, ",")
			}
			data, _, err := api.Post("/research", map[string]any{
				"topic": topic, "sources": srcs, "depth": depth,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&ticket, "ticket", "", "Ticket provider:id")
	cmd.Flags().StringVar(&topic, "topic", "", "Free-text topic")
	cmd.Flags().StringVar(&sources, "sources", "", "Comma-separated sources")
	cmd.Flags().StringVar(&depth, "depth", "standard", "quick|standard|deep")
	return cmd
}
