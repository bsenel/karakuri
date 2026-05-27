package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func objectiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objective",
		Short: "Manage objectives",
	}
	cmd.AddCommand(objectiveCreateCmd(), objectiveGetCmd(), objectiveListCmd(), objectiveTemplatesCmd())
	return cmd
}

func objectiveCreateCmd() *cobra.Command {
	var title, description, domain, twinID, templateID string
	var priority, maxIter int
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an objective",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Post("/objectives", map[string]any{
				"title": title, "description": description, "domain": domain,
				"twin_id": twinID, "template_id": templateID,
				"priority":       priority,
				"max_iterations": maxIter,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Objective title (required)")
	cmd.Flags().StringVar(&description, "description", "", "Description")
	cmd.Flags().StringVar(&domain, "domain", "software", "Domain")
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin ID")
	cmd.Flags().StringVar(&templateID, "template", "", "Template ID (e.g. software.objective.delivery)")
	cmd.Flags().IntVar(&priority, "priority", 0, "Priority (0=low, higher=more urgent)")
	cmd.Flags().IntVar(&maxIter, "max-iter", 0, "Max loop iterations baked into the objective (0 = use the loop-start default)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func objectiveGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an objective by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/objectives/" + args[0])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func objectiveListCmd() *cobra.Command {
	var twinID, status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List objectives",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/objectives"
			sep := "?"
			if twinID != "" {
				path += sep + "twin_id=" + twinID
				sep = "&"
			}
			if status != "" {
				path += sep + "status=" + status
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
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (pending|active|completed|failed)")
	return cmd
}

func objectiveTemplatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "List available objective templates",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/objectives/templates")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
