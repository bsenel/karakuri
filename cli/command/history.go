package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func historyCmd() *cobra.Command {
	var mode string
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List session history",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/sessions"
			if mode != "" {
				path += "?mode=" + mode
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			_ = limit
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "Filter by mode")
	cmd.Flags().IntVar(&limit, "limit", 20, "Limit results")
	return cmd
}
