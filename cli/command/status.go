package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [session-sha]",
		Short: "Get session status",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/sessions/" + args[0] + "/status")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
