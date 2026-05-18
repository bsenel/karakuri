package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [sha-a] [sha-b]",
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
