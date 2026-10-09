package command

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	var (
		objectiveID     string
		agentID         string
		kind            string
		provider        string
		model           string
		template        string
		since           string
		limit           int
		boundsViolation bool
		violationOnly   bool
	)
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect the authority-bounds audit log",
		Long: `Reads tool_events filtered by kind (execute|escalation|approval),
objective, agent, or bounds-violation status, or by what produced the
decision: the provider and model that drafted the plan, and the template
the objective was created from. Each row lists its provider, model,
template_id and autonomy_rung where one was recorded. Default is the 50
most recent entries across all kinds.`,
		Example: `  krk audit
  krk audit --violations-only --limit 20
  krk audit --kind escalation --since 2026-10-01T00:00:00Z
  krk audit --objective <objective-id> --output json`,
		RunE: func(c *cobra.Command, _ []string) error {
			q := url.Values{}
			if objectiveID != "" {
				q.Set("objective_id", objectiveID)
			}
			if agentID != "" {
				q.Set("agent_id", agentID)
			}
			if kind != "" {
				q.Set("kind", kind)
			}
			if provider != "" {
				q.Set("provider", provider)
			}
			if model != "" {
				q.Set("model", model)
			}
			if template != "" {
				q.Set("template", template)
			}
			if since != "" {
				q.Set("since", since)
			}
			if limit > 0 {
				q.Set("limit", itoa(limit))
			}
			if c.Flags().Changed("bounds-violation") {
				if boundsViolation {
					q.Set("bounds_violation", "true")
				} else {
					q.Set("bounds_violation", "false")
				}
			} else if violationOnly {
				q.Set("bounds_violation", "true")
			}
			path := "/audit"
			if encoded := q.Encode(); encoded != "" {
				path += "?" + encoded
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&objectiveID, "objective", "", "Filter by objective ID")
	cmd.Flags().StringVar(&agentID, "agent", "", "Filter by agent ID")
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by event kind (execute|escalation|approval)")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by the provider that drafted the plan")
	cmd.Flags().StringVar(&model, "model", "", "Filter by the model that drafted the plan")
	cmd.Flags().StringVar(&template, "template", "", "Filter by the template the objective was created from")
	cmd.Flags().StringVar(&since, "since", "", "Show events on or after this RFC3339 timestamp")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max entries to return (server caps at 100 by default)")
	cmd.Flags().BoolVar(&boundsViolation, "bounds-violation", false, "Explicit tri-state filter; use --violations-only as shorthand for true")
	cmd.Flags().BoolVar(&violationOnly, "violations-only", false, "Shorthand for --bounds-violation=true")
	cmd.AddCommand(auditExportCmd())
	return cmd
}

func auditExportCmd() *cobra.Command {
	var from, to, out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the audit log for a closed window",
		Long: `Fetches the audit export for the window [from, to) and writes the
server's bytes exactly as received: to stdout, or to the file --out names.
The SHA-256 of those bytes is printed to stderr, so two exports of one
window can be compared. The global --output format does not apply.`,
		Example: `  krk audit export --from 2026-09-01T00:00:00Z --to 2026-10-01T00:00:00Z
  krk audit export --from 2026-09-01T00:00:00Z --to 2026-10-01T00:00:00Z --out audit-2026-09.json`,
		RunE: func(c *cobra.Command, _ []string) error {
			q := url.Values{}
			q.Set("from", from)
			q.Set("to", to)
			data, status, err := api.Get("/audit/export?" + q.Encode())
			if err != nil {
				return err
			}
			if status < 200 || status > 299 {
				return fmt.Errorf("audit export: status %d: %s", status, strings.TrimSpace(string(data)))
			}
			if out != "" {
				if err := os.WriteFile(out, data, 0o600); err != nil {
					return err
				}
			} else if _, err := c.OutOrStdout().Write(data); err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			fmt.Fprintln(c.ErrOrStderr(), hex.EncodeToString(sum[:]))
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Start of the window, inclusive (RFC3339)")
	cmd.Flags().StringVar(&to, "to", "", "End of the window, exclusive (RFC3339)")
	cmd.Flags().StringVar(&out, "out", "", "Write the export to this file instead of stdout")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
