package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// reportCmd groups the digest commands: what a twin's standing objectives did,
// and what they need somebody to decide.
func reportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Periodic digests of what standing objectives did",
		Long: `Digests report on a twin, not on an objective.

A twin holding nine standing objectives should produce one message a day, not
nine — so a schedule names the twin, the cadence, and where to send it.

Every digest ends with the decisions it needs from you: the checkpoints still
pending, oldest first, because the one that has been waiting three days is the
one blocking work.`,
		Example: `  krk report create --twin twin_1 --daily-at 08:00 --timezone Europe/Istanbul \
      --channel messaging --target '#eng-standup'
  krk report preview --twin twin_1
  krk report list`,
	}
	cmd.AddCommand(reportCreateCmd(), reportListCmd(), reportPreviewCmd(), reportOversightCmd(), reportSendCmd(), reportDeleteCmd())
	return cmd
}

func reportCreateCmd() *cobra.Command {
	var (
		twinID, channel, instance, target string
		every, cronExpr, dailyAt, tz      string
		window                            string
		quiet                             []string
		sendWhenEmpty                     bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Declare a digest schedule for a twin",
		Example: `  # A weekday morning brief to Slack.
  krk report create --twin twin_1 --daily-at 08:00 --timezone Europe/Istanbul \
      --channel messaging --target '#eng-standup'

  # A weekly mail, covering the whole week whenever it goes out.
  krk report create --twin twin_1 --cron "0 17 * * 5" \
      --channel email --target lead@example.com --window 168h`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cadence := map[string]any{}
			for k, v := range map[string]string{
				"every": every, "cron": cronExpr, "daily_at": dailyAt, "timezone": tz,
			} {
				if v != "" {
					cadence[k] = v
				}
			}
			if len(quiet) > 0 {
				cadence["quiet"] = quiet
			}
			data, _, err := api.Post("/reports", map[string]any{
				"twin_id":         twinID,
				"cadence":         cadence,
				"channel":         channel,
				"instance":        instance,
				"target":          target,
				"window":          window,
				"send_when_empty": sendWhenEmpty,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin to report on, by twin ID from 'krk twin list' (required)")
	cmd.Flags().StringVar(&channel, "channel", "messaging", "Adapter slot: messaging|email|projectmgmt|versioncontrol")
	cmd.Flags().StringVar(&instance, "instance", "", "Named adapter instance; empty uses the twin's binding")
	cmd.Flags().StringVar(&target, "target", "", "Where to send it — a channel, an address (required)")
	cmd.Flags().StringVar(&every, "every", "", "Send on this interval, e.g. 24h")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "Send on a five-field cron expression, in --timezone")
	cmd.Flags().StringVar(&dailyAt, "daily-at", "", "Send daily at HH:MM in --timezone")
	cmd.Flags().StringVar(&tz, "timezone", "", "IANA timezone for --cron and --daily-at (default UTC)")
	cmd.Flags().StringSliceVar(&quiet, "quiet", nil, "Blackout windows as HH:MM-HH:MM; a digest due inside one waits for the opening")
	cmd.Flags().StringVar(&window, "window", "", "How far back to look, e.g. 24h. Empty means since the last one was sent, so a missed run catches up")
	cmd.Flags().BoolVar(&sendWhenEmpty, "send-when-empty", false, "Send even when nothing happened (off: a mail that says nothing happened is one people stop reading)")
	_ = cmd.MarkFlagRequired("twin")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func reportListCmd() *cobra.Command {
	var twinID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List digest schedules",
		Example: `  # Every schedule you may see, with the IDs that send and delete take.
  krk report list

  # Only the schedules of one twin.
  krk report list --twin twin_1`,
		RunE: func(c *cobra.Command, _ []string) error {
			path := "/reports"
			if twinID != "" {
				path += "?twin_id=" + url.QueryEscape(twinID)
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			printList(c, data, "digest schedules", createWith("krk report create"))
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Show only this twin's schedules, by twin ID from 'krk twin list' (default: all twins)")
	return cmd
}

func reportPreviewCmd() *cobra.Command {
	var twinID, window string
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "See what a digest would say, without sending it",
		Long: `Assemble and render a digest without delivering it.

Worth doing before committing somebody to a daily mail: the digest is built
from records that already exist, so a preview over the last day is exactly what
tomorrow's would have looked like.`,
		Example: `  # What a digest over the last day would say.
  krk report preview --twin twin_1

  # The same over the last week.
  krk report preview --twin twin_1 --window 168h`,
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/reports/preview?twin_id=" + url.QueryEscape(twinID)
			if window != "" {
				path += "&window=" + url.QueryEscape(window)
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin to report on, by twin ID from 'krk twin list' (required)")
	cmd.Flags().StringVar(&window, "window", "24h", "How far back to look, e.g. 24h or 168h for a week")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

// reportOversightCmd is the one oversight listing: per stream, its passes and
// outcomes, pull requests waiting for a merge with the review result, and the
// refusals and blocks the digest carries (Phase 40 step 14).
func reportOversightCmd() *cobra.Command {
	var twinID, window string
	cmd := &cobra.Command{
		Use:   "oversight",
		Short: "List each stream's passes, waiting pull requests, refusals and blocks",
		Example: `  # Every stream's last day.
  krk report oversight --twin twin_1

  # The same over the last week.
  krk report oversight --twin twin_1 --window 168h`,
		RunE: func(c *cobra.Command, _ []string) error {
			data, _, err := api.Get("/reports/preview?twin_id=" + url.QueryEscape(twinID) +
				"&window=" + url.QueryEscape(window))
			if err != nil {
				return err
			}
			if output == "json" || output == "quiet" {
				client.PrintOutput(data, output)
				return nil
			}
			var d oversightView
			if err := json.Unmarshal(data, &d); err != nil {
				return fmt.Errorf("read digest: %w", err)
			}
			printOversight(c.OutOrStdout(), d)
			return nil
		},
	}
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin to report on (required)")
	cmd.Flags().StringVar(&window, "window", "24h", "How far back to look, e.g. 24h or 168h for a week")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

// oversightView is the part of the digest the listing reads. Waiting pull
// requests are listed when the API carries them.
type oversightView struct {
	Objectives []struct {
		Title       string `json:"title"`
		Status      string `json:"status"`
		Senses      int    `json:"senses"`
		Reconciles  int    `json:"reconciles"`
		Converged   int    `json:"converged"`
		Escalations int    `json:"escalations"`
		Failures    int    `json:"failures"`
		Waiting     []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Review string `json:"review"`
		} `json:"waiting_pull_requests"`
		Refusals []struct {
			Capability string `json:"capability"`
			Reason     string `json:"reason"`
		} `json:"refusals"`
		Blocks []struct {
			Kind    string `json:"kind"`
			Reason  string `json:"reason"`
			Subject string `json:"subject"`
			Count   int    `json:"count"`
			Until   string `json:"until"`
		} `json:"blocks"`
	} `json:"objectives"`
}

func countOf(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func printOversight(w io.Writer, d oversightView) {
	if len(d.Objectives) == 0 {
		fmt.Fprintln(w, "No streams in this window.")
		return
	}
	for _, o := range d.Objectives {
		fmt.Fprintf(w, "%s (%s)\n", o.Title, o.Status)
		fmt.Fprintf(w, "  %s, %s", countOf(o.Reconciles, "reconcile"), countOf(o.Senses, "sense"))
		if o.Converged > 0 {
			fmt.Fprintf(w, ", %d converged", o.Converged)
		}
		if o.Escalations > 0 {
			fmt.Fprintf(w, ", %s", countOf(o.Escalations, "escalation"))
		}
		if o.Failures > 0 {
			fmt.Fprintf(w, ", %s", countOf(o.Failures, "failure"))
		}
		fmt.Fprintln(w)
		for _, pr := range o.Waiting {
			fmt.Fprintf(w, "  waiting: #%d %s [%s]\n", pr.Number, pr.Title, pr.Review)
		}
		for _, r := range o.Refusals {
			fmt.Fprintf(w, "  refused: %s: %s\n", r.Capability, r.Reason)
		}
		for _, b := range o.Blocks {
			fmt.Fprintf(w, "  blocked: %s", b.Kind)
			if b.Subject != "" {
				fmt.Fprintf(w, " %s", b.Subject)
			}
			if b.Count > 0 {
				fmt.Fprintf(w, " x%d", b.Count)
			}
			if b.Reason != "" {
				fmt.Fprintf(w, ": %s", b.Reason)
			}
			if b.Until != "" && !strings.HasPrefix(b.Until, "0001-") {
				fmt.Fprintf(w, " (until %s)", b.Until)
			}
			fmt.Fprintln(w)
		}
	}
}

func reportSendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send <id>",
		Short: "Send a digest now, outside its cadence",
		Long: `Send the digest of one schedule now, without waiting for its cadence.

<id> is the schedule's ID, as shown by 'krk report list'.`,
		Example: `  # Find the schedule's ID, then send its digest.
  krk report list
  krk report send <id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/reports/"+args[0]+"/send", map[string]any{})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func reportDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a digest schedule",
		Long: `Delete a digest schedule, so that no further digests are sent for it.

<id> is the schedule's ID, as shown by 'krk report list'.`,
		Example: `  # Find the schedule's ID, then delete it.
  krk report list
  krk report delete <id>`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if _, _, err := api.Delete("/reports/" + args[0]); err != nil {
				return err
			}
			if output != "quiet" {
				fmt.Fprintf(c.OutOrStdout(), "digest schedule %s deleted\n", args[0])
			}
			return nil
		},
	}
}
