package command

import (
	"fmt"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func autoCmd() *cobra.Command {
	var domain, kind string
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Create a watcher twin and start watch mode",
		RunE: func(_ *cobra.Command, _ []string) error {
			twinData, _, err := api.Post("/twins", map[string]string{
				"name": "watcher", "kind": kind, "domain": domain,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(twinData, output)
			fmt.Println("Watcher twin created. Use 'krk loop start <objective-id>' to begin autonomous operation.")
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "software", "Domain for the watcher twin")
	cmd.Flags().StringVar(&kind, "kind", "team", "Twin kind")
	return cmd
}
