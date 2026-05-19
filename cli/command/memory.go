package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func memoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Query and manage agent memory",
	}
	cmd.AddCommand(memoryRecallCmd())
	return cmd
}

func memoryRecallCmd() *cobra.Command {
	var agentID, query, tier string
	var topK int
	cmd := &cobra.Command{
		Use:   "recall",
		Short: "Recall memory entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			tiers := []string{"episodic"}
			if tier != "" {
				tiers = []string{tier}
			}
			data, _, err := api.Post("/memory/recall", map[string]any{
				"agent_id": agentID,
				"query":    query,
				"tiers":    tiers,
				"top_k":    topK,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentID, "agent", "", "Agent ID")
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().StringVar(&tier, "tier", "", "Memory tier: working|episodic|semantic|procedural")
	cmd.Flags().IntVar(&topK, "top-k", 5, "Maximum results")
	return cmd
}
