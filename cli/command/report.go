package command

import (
	"fmt"
	"net/url"

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
	}
	cmd.AddCommand(reportCreateCmd(), reportListCmd(), reportPreviewCmd(), reportSendCmd(), reportDeleteCmd())
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
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin to report on (required)")
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
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/reports"
			if twinID != "" {
				path += "?twin_id=" + url.QueryEscape(twinID)
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

func reportPreviewCmd() *cobra.Command {
	var twinID, window string
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "See what a digest would say, without sending it",
		Long: `Assemble and render a digest without delivering it.

Worth doing before committing somebody to a daily mail: the digest is built
from records that already exist, so a preview over the last day is exactly what
tomorrow's would have looked like.`,
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
	cmd.Flags().StringVar(&twinID, "twin", "", "Twin to report on (required)")
	cmd.Flags().StringVar(&window, "window", "24h", "How far back to look")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

func reportSendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "send <id>",
		Short: "Send a digest now, outside its cadence",
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if _, _, err := api.Delete("/reports/" + args[0]); err != nil {
				return err
			}
			fmt.Printf("report schedule %s deleted\n", args[0])
			return nil
		},
	}
}
