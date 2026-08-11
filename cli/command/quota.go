package command

import (
	"net/url"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func quotaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Inspect rate limits and quotas, and reset a twin's counters",
		Long: `Karakuri enforces four limits: a per-principal request rate, a per-capability
daily cap, a per-twin daily model budget, and a per-adapter daily cap.

Reading them needs quota:read, which every role has. Resetting somebody's
counters needs quota:admin, because it is an operator override rather than an
ordinary operation.`,
	}
	cmd.AddCommand(quotaConfigCmd(), quotaShowCmd(), quotaResetCmd())
	return cmd
}

func quotaConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show the limits this server is enforcing",
		Long: `Reports what the server is actually enforcing, which is not always what the
config file you are looking at says — environment overrides and defaults both
apply.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/quota")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func quotaShowCmd() *cobra.Command {
	var twin string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a twin's current usage",
		Long: `Reports how much of each daily tier a twin has consumed, and when it resets.

Reading usage does not consume any of it.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/quota/usage?twin=" + url.QueryEscape(twin))
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twin, "twin", "", "twin ID (required)")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

func quotaResetCmd() *cobra.Command {
	var twin, capability string
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Clear a twin's counters for the current period (admin)",
		Long: `Clears the current period only. Resetting today cannot hand back yesterday's
budget, and tomorrow starts clean either way.

Without --capability this clears the twin-wide tiers; with one it clears that
capability's daily count.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Post("/quota/reset", map[string]string{
				"twin":       twin,
				"capability": capability,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&twin, "twin", "", "twin ID (required)")
	cmd.Flags().StringVar(&capability, "capability", "", "reset one capability's daily count instead of the twin-wide tiers")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}
