package command

import (
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func researchCmd() *cobra.Command {
	var twinID, objectiveID, agentID, sources, depth string
	cmd := &cobra.Command{
		Use:   "research <topic>",
		Short: "Research a topic and save the findings as an artifact",
		Example: `  krk research "rate limiting strategies for public APIs"

  # Attach the findings to an objective (see: krk objective list)
  krk research "OAuth 2.1 changes" --objective obj_123 --twin t_7f2a --depth deep`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var srcs []string
			if sources != "" {
				srcs = strings.Split(sources, ",")
			}
			data, _, err := api.Post("/research", map[string]any{
				"twin_id":      twinID,
				"objective_id": objectiveID,
				"agent_id":     agentID,
				"topic":        args[0],
				"sources":      srcs,
				"depth":        depth,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin ID (see: krk twin list)")
	cmd.Flags().StringVar(&objectiveID, "objective", "", "Objective the findings artifact is attached to (see: krk objective list)")
	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID recorded on the findings artifact")
	cmd.Flags().StringVar(&sources, "sources", "", "Comma-separated source adapters; the server uses http-scraper when empty")
	cmd.Flags().StringVar(&depth, "depth", "standard", "Research depth: quick|standard|deep")
	return cmd
}
