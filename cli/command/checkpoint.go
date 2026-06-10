package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func checkpointCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Manage checkpoints",
	}
	cmd.AddCommand(checkpointListCmd(), checkpointGetCmd(), checkpointResolveCmd())
	return cmd
}

func checkpointListCmd() *cobra.Command {
	var twinID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending checkpoints",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/checkpoints"
			if twinID != "" {
				path += "?twin_id=" + twinID
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Filter by twin ID")
	return cmd
}

func checkpointGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a checkpoint",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/checkpoints/" + args[0])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func checkpointResolveCmd() *cobra.Command {
	var decision, note, approver string
	var removeActions, constraints []string
	var revisedConfidence float64
	cmd := &cobra.Command{
		Use:   "resolve <id>",
		Short: "Resolve a checkpoint with a decision",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			body := buildResolveBody(c, decision, note, approver, removeActions, constraints, revisedConfidence)
			data, _, err := api.Post("/checkpoints/"+args[0]+"/resolve", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&decision, "decision", "", "Decision choice (required)")
	cmd.Flags().StringVar(&note, "note", "", "Free-form rationale stored on the audit row")
	cmd.Flags().StringVar(&approver, "approver", "", "Identifier of the operator approving/rejecting (audit attribution)")
	cmd.Flags().StringSliceVar(&removeActions, "remove-action", nil, "Capability ID to drop from the draft (repeatable; only valid with --decision modify)")
	cmd.Flags().StringSliceVar(&constraints, "constraint", nil, "Constraint to feed into the revise pass (repeatable; only valid with --decision modify)")
	cmd.Flags().Float64Var(&revisedConfidence, "revised-confidence", -1, "Floor for the revised plan's confidence (only valid with --decision modify)")
	_ = cmd.MarkFlagRequired("decision")
	return cmd
}

// buildResolveBody composes the resolve/resume request body. Modifications
// is only attached when at least one --remove-action / --constraint /
// --revised-confidence flag is set (Phase 13.5).
func buildResolveBody(c *cobra.Command, decision, note, approver string, removeActions, constraints []string, revisedConfidence float64) map[string]any {
	body := map[string]any{
		"decision": decision,
		"note":     note,
		"approver": approver,
	}
	mods := map[string]any{}
	if len(removeActions) > 0 {
		mods["removed_actions"] = removeActions
	}
	if len(constraints) > 0 {
		mods["added_constraints"] = constraints
	}
	if c.Flags().Changed("revised-confidence") {
		mods["revised_confidence"] = revisedConfidence
	}
	if len(mods) > 0 {
		body["modifications"] = mods
	}
	return body
}
