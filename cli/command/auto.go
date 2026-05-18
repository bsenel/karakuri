package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func autoCmd() *cobra.Command {
	var interval, scope, env string
	var validate, dryRun bool
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Run autonomous intelligence loops",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = interval
			_ = scope
			_ = env
			_ = validate
			_ = dryRun
			data, _, err := api.Post("/auto/run", nil)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&interval, "interval", "1h", "Run interval")
	cmd.Flags().StringVar(&scope, "scope", "all", "Loop scope")
	cmd.Flags().StringVar(&env, "env", "all", "Environment")
	cmd.Flags().BoolVar(&validate, "validate", false, "Validate loops")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Dry run")
	return cmd
}
