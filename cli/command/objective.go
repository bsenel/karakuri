package command

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func objectiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objective",
		Short: "Manage objectives",
	}
	cmd.AddCommand(objectiveCreateCmd(), objectiveGetCmd(), objectiveListCmd(), objectiveTemplatesCmd())
	cmd.AddCommand(standingCmds()...)
	return cmd
}

func objectiveCreateCmd() *cobra.Command {
	var title, description, domain, twinID, templateID string
	var priority, maxIter int
	var criteriaFile string
	var criteria []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an objective",
		Example: `  krk objective create --title "Fix the flaky login test" --twin t_7f2a

  # From a template (see: krk objective templates)
  krk objective create --title "Ship the export endpoint" --twin t_7f2a \
      --template software.objective.delivery --priority 2

  # With success criteria from a file (see docs/examples/criteria.yaml)
  krk objective create --title "Ship the export endpoint" --twin t_7f2a \
      --criteria-file docs/examples/criteria.yaml

  # Add a one-off criterion after the file's
  krk objective create --title "Speed up search" --twin t_7f2a \
      --criteria-file docs/examples/criteria.yaml \
      --criterion 'id=p95;description=Search p95 latency is at most 250 ms;kind=measured;verifier=ci.latency;metric=p95_latency;op=<=;target=250;unit=ms'`,
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]any{
				"title": title, "description": description, "domain": domain,
				"twin_id": twinID, "template_id": templateID,
				"priority":       priority,
				"max_iterations": maxIter,
			}
			sc, err := successCriteria(criteriaFile, criteria)
			if err != nil {
				return err
			}
			if sc != nil {
				body["success_criteria"] = sc
			}
			data, status, err := api.Post("/objectives", body)
			if err != nil {
				return err
			}
			// The server's 400 names the field it refused (success_criteria[1].evidence),
			// so its body is the error, word for word.
			if status >= http.StatusBadRequest {
				return fmt.Errorf("create objective: HTTP %d: %s", status, strings.TrimSpace(string(data)))
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Objective title (required)")
	cmd.Flags().StringVar(&description, "description", "", "What the objective should achieve, in more detail than the title")
	cmd.Flags().StringVar(&domain, "domain", "software", "Domain")
	cmd.Flags().StringVar(&twinID, "twin", "", "ID of the twin the objective belongs to (see: krk twin list)")
	cmd.Flags().StringVar(&templateID, "template", "", "Template ID (e.g. software.objective.delivery)")
	cmd.Flags().IntVar(&priority, "priority", 0, "Priority (0=low, higher=more urgent)")
	cmd.Flags().IntVar(&maxIter, "max-iter", 0, "Max loop iterations baked into the objective (0 = use the loop-start default)")
	cmd.Flags().StringVar(&criteriaFile, "criteria-file", "", "YAML file of success criteria, in the shape of the API's success_criteria")
	cmd.Flags().StringArrayVar(&criteria, "criterion", nil, "One success criterion, id=..;kind=measured;metric=..;op=..;target=..;unit=.. (repeatable; added after --criteria-file)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func objectiveGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get <id>",
		Short:   "Get an objective by ID",
		Long:    "Get an objective by ID. krk objective list shows each objective's ID,\nand krk objective create prints the ID of the one it creates.",
		Example: `  krk objective get obj_123`,
		Args:    cobra.ExactArgs(1),
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
		Example: `  krk objective list
  krk objective list --twin t_7f2a --status active`,
		RunE: func(c *cobra.Command, _ []string) error {
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
			printList(c, data, "objectives", "krk objective create")
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Filter by twin ID (see: krk twin list)")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status (pending|active|blocked|converged|completed|failed)")
	return cmd
}

func objectiveTemplatesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "templates",
		Short: "List available objective templates",
		Long:  "List available objective templates. Pass a template's ID to\nkrk objective create --template to start an objective from it.",
		Example: `  krk objective templates
  krk objective create --title "Ship the export endpoint" --twin t_7f2a \
      --template software.objective.delivery`,
		RunE: func(c *cobra.Command, _ []string) error {
			data, _, err := api.Get("/objectives/templates")
			if err != nil {
				return err
			}
			printList(c, data, "objective templates", "")
			return nil
		},
	}
}
