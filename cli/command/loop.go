package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func loopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loop",
		Short: "Control the autonomous reasoning loop",
	}
	cmd.AddCommand(loopStartCmd(), loopStatusCmd(), loopResumeCmd())
	return cmd
}

func loopStartCmd() *cobra.Command {
	var twinID string
	var maxIter int
	var watchMode bool
	cmd := &cobra.Command{
		Use:   "start <objective-id>",
		Short: "Start the reasoning loop for an objective",
		Example: `  krk loop start obj_123 --twin t_7f2a

  # Stop after at most 10 iterations
  krk loop start obj_123 --twin t_7f2a --max-iter 10`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/loops", map[string]any{
				"objective_id": args[0],
				"twin_id":      twinID,
				"max_iter":     maxIter,
				"watch_mode":   watchMode,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "ID of the objective's twin (see: krk twin list)")
	cmd.Flags().IntVar(&maxIter, "max-iter", 50, "Maximum loop iterations")
	cmd.Flags().BoolVar(&watchMode, "watch", false, "Enable watch mode (loop continues on environment events)")
	return cmd
}

func loopStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status <loop-id>",
		Short:   "Get loop status",
		Example: `  krk loop status loop_123`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/loops/" + args[0] + "/status")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func loopResumeCmd() *cobra.Command {
	var decision, note, approver string
	var removeActions, constraints []string
	var revisedConfidence float64
	cmd := &cobra.Command{
		Use:   "resume <loop-id>",
		Short: "Resume a paused loop with a checkpoint decision",
		Example: `  # Let the plan go ahead as drafted
  krk loop resume loop_123 --decision approve --note "plan looks right"

  # Stop it
  krk loop resume loop_123 --decision reject --note "out of scope"`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			body := buildResolveBody(c, decision, note, approver, removeActions, constraints, revisedConfidence)
			data, _, err := api.Post("/loops/"+args[0]+"/resume", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "Decision: approve|reject|modify (required)")
	cmd.Flags().StringVar(&note, "note", "", "Free-form rationale stored on the audit row")
	cmd.Flags().StringVar(&approver, "approver", "", "Identifier of the operator approving/rejecting (audit attribution)")
	cmd.Flags().StringSliceVar(&removeActions, "remove-action", nil, "Capability ID to drop from the draft (repeatable; only valid with --decision modify)")
	cmd.Flags().StringSliceVar(&constraints, "constraint", nil, "Constraint to feed into the revise pass (repeatable; only valid with --decision modify)")
	cmd.Flags().Float64Var(&revisedConfidence, "revised-confidence", -1, "Floor for the revised plan's confidence (only valid with --decision modify)")
	_ = cmd.MarkFlagRequired("decision")
	return cmd
}
