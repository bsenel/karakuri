package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func artifactsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "artifacts [session-sha]",
		Short: "List session artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/sessions/" + args[0] + "/artifacts")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
