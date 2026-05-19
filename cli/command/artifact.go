package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func artifactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage artifacts",
	}
	cmd.AddCommand(artifactListCmd(), artifactGetCmd(), artifactDiffCmd())
	return cmd
}

func artifactListCmd() *cobra.Command {
	var objectiveID, agentID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List artifacts",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/artifacts"
			sep := "?"
			if objectiveID != "" {
				path += sep + "objective_id=" + objectiveID
				sep = "&"
			}
			if agentID != "" {
				path += sep + "agent_id=" + agentID
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&objectiveID, "objective", "", "Filter by objective ID")
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent ID")
	return cmd
}

func artifactGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <sha>",
		Short: "Get artifact content by SHA",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/artifacts/" + args[0])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func artifactDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <sha-a> <sha-b>",
		Short: "Diff two artifacts",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/artifacts/" + args[0] + "/diff/" + args[1])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
