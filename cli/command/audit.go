package command

import (
	"net/url"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func auditCmd() *cobra.Command {
	var (
		objectiveID     string
		agentID         string
		kind            string
		since           string
		limit           int
		boundsViolation bool
		violationOnly   bool
	)
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Inspect the authority-bounds audit log",
		Long: `Reads tool_events filtered by kind (execute|escalation|approval),
objective, agent, or bounds-violation status. Default is the 50 most
recent entries across all kinds.`,
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
	cmd.Flags().StringVar(&since, "since", "", "Show events on or after this RFC3339 timestamp")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max entries to return (server caps at 100 by default)")
	cmd.Flags().BoolVar(&boundsViolation, "bounds-violation", false, "Explicit tri-state filter; use --violations-only as shorthand for true")
	cmd.Flags().BoolVar(&violationOnly, "violations-only", false, "Shorthand for --bounds-violation=true")
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
