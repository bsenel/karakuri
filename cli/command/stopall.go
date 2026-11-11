package command

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// stopAllCmd stops every running loop and pauses every standing objective the
// caller may stop (Phase 37 step 4), in one POST /stop-all.
func stopAllCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "stop-all",
		Short: "Stop every running loop and pause every standing objective",
		Long: `Stop every running loop and pause every standing objective you may stop, in one
call. The answer lists the loops stopped, the objectives paused, and every run
it did not reach and why (for example, one you are not permitted to stop).

A stopped loop stays stopped. A paused objective stays paused until a person
resumes it, one objective at a time, with krk objective resume <objective-id>.`,
		Example: `  krk stop-all

  # Record why everything was stopped
  krk stop-all --reason "pushing to the wrong remote"`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, status, err := api.Post("/stop-all", map[string]any{"reason": reason})
			if err != nil {
				return err
			}
			// A refusal must not read as "nothing was running": its body is the error.
			if status >= http.StatusBadRequest {
				return fmt.Errorf("stop-all: HTTP %d: %s", status, strings.TrimSpace(string(data)))
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Why everything is stopped, kept on the record of each run (default: stopped by <you>)")
	return cmd
}
