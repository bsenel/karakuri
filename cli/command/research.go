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
		Short: "Run a research query",
		Args:  cobra.ExactArgs(1),
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
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin ID")
	cmd.Flags().StringVar(&objectiveID, "objective", "", "Objective ID")
	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID")
	cmd.Flags().StringVar(&sources, "sources", "", "Comma-separated source adapters")
	cmd.Flags().StringVar(&depth, "depth", "standard", "Research depth: quick|standard|deep")
	return cmd
}
