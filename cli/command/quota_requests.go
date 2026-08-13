package command

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// Self-service limits on the command line.
//
// Asking and deciding are separate commands because they are separate
// permissions: almost everybody should be able to ask, and almost nobody to
// decide. Collapsing them into one `krk quota set` would have made the
// permission to request the permission to grant.

func quotaRequestCmd() *cobra.Command {
	var (
		tier      string
		twin      string
		principal string
		cap       int
		window    time.Duration
		until     string
		reason    string
	)
	cmd := &cobra.Command{
		Use:   "request",
		Short: "Ask for a limit to be raised",
		Long: `Records a request for an administrator to decide. Nothing changes until
somebody approves it.

The subject depends on the tier: --tier request is per principal and defaults
to you, and the twin tiers need --twin. A reason is required — a limit raised
for a reason nobody wrote down is one nobody can review later.

Without --until the raise is permanent once approved. With it, the raise lapses
on its own, which is what "we need double for launch week" actually means.`,
		Example: `  krk quota request --tier llm-tokens --twin t_7f2a --cap 5000000 --reason "launch week"
  krk quota request --tier llm-tokens --twin t_7f2a --cap 5000000 --reason "launch" --until 2026-09-01T00:00:00Z
  krk quota request --tier request --cap 600 --reason "bulk import"`,
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]any{
				"tier": tier, "twin": twin, "principal": principal,
				"cap": cap, "reason": reason,
			}
			if window > 0 {
				body["window"] = window.String()
			}
			if until != "" {
				if _, err := time.Parse(time.RFC3339, until); err != nil {
					return fmt.Errorf("--until must be RFC3339, e.g. 2026-09-01T00:00:00Z: %w", err)
				}
				body["until"] = until
			}
			data, _, err := api.Post("/quota/requests", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&tier, "tier", "", "which limit: request, capability, llm-tokens or adapter (required)")
	cmd.Flags().StringVar(&twin, "twin", "", "twin the limit applies to, for the twin tiers")
	cmd.Flags().StringVar(&principal, "principal", "", "principal the limit applies to, for --tier request (default: you)")
	cmd.Flags().IntVar(&cap, "cap", 0, "the new ceiling (required)")
	cmd.Flags().DurationVar(&window, "window", 0, "new window, for rate tiers only")
	cmd.Flags().StringVar(&until, "until", "", "when the raise should lapse, RFC3339 (default: permanent)")
	cmd.Flags().StringVar(&reason, "reason", "", "why you need it (required)")
	_ = cmd.MarkFlagRequired("tier")
	_ = cmd.MarkFlagRequired("cap")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func quotaRequestsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List, approve and reject quota requests",
	}
	cmd.AddCommand(quotaRequestsListCmd(), quotaDecideCmd("approve"), quotaDecideCmd("reject"))
	return cmd
}

func quotaRequestsListCmd() *cobra.Command {
	var (
		status string
		twin   string
		mine   bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List quota requests",
		Example: `  krk quota requests list --status pending
  krk quota requests list --mine`,
		RunE: func(_ *cobra.Command, _ []string) error {
			q := url.Values{}
			if status != "" {
				q.Set("status", status)
			}
			if twin != "" {
				q.Set("twin", twin)
			}
			if mine {
				q.Set("mine", "true")
			}
			path := "/quota/requests"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "pending, approved or rejected")
	cmd.Flags().StringVar(&twin, "twin", "", "only requests about this twin")
	cmd.Flags().BoolVar(&mine, "mine", false, "only requests you made")
	return cmd
}

// quotaDecideCmd builds `approve` and `reject`, which differ by one boolean and
// by nothing else worth duplicating.
func quotaDecideCmd(verb string) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   verb + " <request-id>",
		Short: strings.ToUpper(verb[:1]) + verb[1:] + " a quota request",
		Long: `Approving writes the override, which takes effect within the resolver's
cache window — under a minute — and immediately in the process that approved it.

You can only approve a raise for a subject you already hold. Without that rule
the permission to approve would be the permission to raise anybody's limit,
including your own in a tenant you have no claim on.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/quota/requests/"+args[0]+"/decide", map[string]any{
				"approve": verb == "approve",
				"note":    note,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	hint := "why"
	if verb == "reject" {
		hint = `why not — "no" without a reason is the least useful answer`
	}
	cmd.Flags().StringVar(&note, "note", "", hint)
	return cmd
}
