package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func checkpointCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Manage checkpoints",
	}
	cmd.AddCommand(checkpointListCmd(), checkpointGetCmd(), checkpointResolveCmd())
	return cmd
}

func checkpointListCmd() *cobra.Command {
	var twinID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending checkpoints",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/checkpoints"
			if twinID != "" {
				path += "?twin_id=" + twinID
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Filter by twin ID")
	return cmd
}

func checkpointGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/checkpoints/" + args[0])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func checkpointResolveCmd() *cobra.Command {
	var decision string
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve a checkpoint with a decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/checkpoints/"+args[0]+"/resolve", map[string]string{
				"decision": decision,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "Decision choice (required)")
	_ = cmd.MarkFlagRequired("decision")
	return cmd
}
