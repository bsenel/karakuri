package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func objectiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "objective",
		Short: "Manage objectives",
	}
	cmd.AddCommand(objectiveCreateCmd(), objectiveGetCmd(), objectiveListCmd(), objectiveTemplatesCmd(), objectiveCriteriaCmd())
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

func objectiveCriteriaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "criteria <id>",
		Short: "Show an objective's success criteria and their state",
		Long: "Show an objective's success criteria and the state the server settled\n" +
			"each one in. Required criteria are the gate; required: false criteria are\n" +
			"printed apart as progress. krk objective list shows each objective's ID.",
		Example: `  krk objective criteria obj_123`,
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			data, _, err := api.Get("/objectives/" + args[0])
			if err != nil {
				return fmt.Errorf("objective %s: %w", args[0], err)
			}
			if output != "pretty" {
				client.PrintOutput(data, output)
				return nil
			}
			return printCriteria(c.OutOrStdout(), data)
		},
	}
}

// shownCriterion is the part of a criterion in GET /objectives/{id} that
// krk objective criteria prints.
type shownCriterion struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Metric      string   `json:"metric"`
	Op          string   `json:"op"`
	Target      *float64 `json:"target"`
	Unit        string   `json:"unit"`
	Required    *bool    `json:"required"`
	State       string   `json:"state"`
	Reason      string   `json:"reason"`
}

// printCriteria writes the gate, then the progress. State and reason are the
// server's; a criterion with no state has not been through a verify step.
func printCriteria(w io.Writer, data []byte) error {
	var obj struct {
		ID       string           `json:"id"`
		Title    string           `json:"title"`
		Criteria []shownCriterion `json:"success_criteria"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("read objective: %w", err)
	}
	var gate, progress []shownCriterion
	for _, cr := range obj.Criteria {
		// The API omits required when it is true.
		if cr.Required == nil || *cr.Required {
			gate = append(gate, cr)
		} else {
			progress = append(progress, cr)
		}
	}
	fmt.Fprintf(w, "%s  %s\n\nRequired:\n", obj.ID, obj.Title)
	writeCriteria(w, gate, true)
	if len(progress) > 0 {
		fmt.Fprintln(w, "\nProgress (required: false, not part of the gate):")
		writeCriteria(w, progress, false)
	}
	return nil
}

func writeCriteria(w io.Writer, criteria []shownCriterion, gate bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, cr := range criteria {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", cr.ID, cr.Kind, criterionState(cr, gate), comparison(cr), cr.Description)
	}
	_ = tw.Flush()
}

// criterionState is the server's state; an optional criterion not met yet is
// progress still to make, so it does not read as a failure.
func criterionState(cr shownCriterion, gate bool) string {
	switch {
	case cr.State == "":
		return "inconclusive: not_run"
	case cr.State == "inconclusive" && cr.Reason != "":
		return "inconclusive: " + cr.Reason
	case cr.State == "not_met" && !gate:
		return "not yet met"
	}
	return cr.State
}

// comparison is "metric op target unit" for a measured criterion.
func comparison(cr shownCriterion) string {
	if cr.Metric == "" || cr.Target == nil {
		return ""
	}
	target := strconv.FormatFloat(*cr.Target, 'f', -1, 64)
	return strings.TrimSpace(cr.Metric + " " + cr.Op + " " + target + " " + cr.Unit)
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
