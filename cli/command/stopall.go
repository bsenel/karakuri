package command

import "github.com/spf13/cobra"

// stopAllCmd stops every running loop and pauses every standing objective the
// caller may stop (Phase 37 step 4). Not implemented yet: it sends nothing.
func stopAllCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "stop-all",
		Short: "Stop every running loop and pause every standing objective",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Why everything is stopped")
	return cmd
}
