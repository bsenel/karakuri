package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func resolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve [session-sha] [checkpoint-id] [decision]",
		Short: "Resolve a checkpoint",
		Args:  cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/sessions/"+args[0]+"/checkpoints/"+args[1]+"/resolve",
				map[string]string{"decision": args[2]})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
