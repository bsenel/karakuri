package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// Editing the limits themselves, as opposed to asking for an exception to one.
//
// `krk quota request` asks for more for one subject and somebody approves it.
// This changes the limit for everybody, which is why it needs quota:admin and
// why it is a different verb: raising a team's ceiling and raising the whole
// deployment's are not the same decision wearing different flags.

func quotaSetCmd() *cobra.Command {
	var (
		tier      string
		cap       int
		perMinute int
		burst     int
		reason    string
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Change a limit for everybody",
		Long: `Stores a limit in the database, where it takes precedence over the value in
the configuration file.

The file still matters: it seeds a fresh database, and ` + "`krk quota unset`" + ` returns a
tier to it. What it stops being is the answer — so ` + "`krk quota config`" + ` reports
what is configured beside what is in force, and the server logs the difference
at startup.

A reason is required. This changes the limit for everybody, and one changed for
a reason nobody wrote down is one nobody can review later.

The request tier is a rate: give it --per-minute, and --burst if a page load
should be allowed to arrive at once. The others are daily caps: give them --cap.`,
		Example: `  krk quota set --tier llm-tokens --cap 5000000 --reason "team grew to twelve"
  krk quota set --tier request --per-minute 120 --burst 40 --reason "the SPA polls"
  krk quota set --tier capability --cap 2000 --reason "the nightly sweep needs headroom"`,
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]any{"reason": reason}
			switch tier {
			case "request":
				if perMinute <= 0 {
					return fmt.Errorf("--tier request is a rate; give it --per-minute")
				}
				// Cap is the bucket's capacity and rate is the sustained
				// refill, which is what "sixty a minute tolerating twenty at
				// once" means. Without --burst the two coincide.
				ceiling := burst
				if ceiling <= 0 {
					ceiling = perMinute
				}
				body["cap"] = ceiling
				body["window"] = time.Minute.String()
				body["rate"] = float64(perMinute) / 60.0
			case "capability", "llm-tokens", "adapter":
				if cap <= 0 {
					return fmt.Errorf("--tier %s is a daily cap; give it --cap", tier)
				}
				if perMinute > 0 || burst > 0 {
					return fmt.Errorf("--per-minute and --burst apply to --tier request only")
				}
				body["cap"] = cap
			default:
				return fmt.Errorf("unknown tier %q; one of adapter, capability, llm-tokens, request", tier)
			}

			data, _, err := api.Put("/quota/tiers/"+tier, body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&tier, "tier", "", "which limit: request, capability, llm-tokens or adapter (required)")
	cmd.Flags().IntVar(&cap, "cap", 0, "the new daily ceiling, for the quota tiers")
	cmd.Flags().IntVar(&perMinute, "per-minute", 0, "sustained requests per minute, for --tier request")
	cmd.Flags().IntVar(&burst, "burst", 0, "how many may arrive at once, for --tier request (default: --per-minute)")
	cmd.Flags().StringVar(&reason, "reason", "", "why (required)")
	_ = cmd.MarkFlagRequired("tier")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func quotaUnsetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unset <tier>",
		Short: "Return a limit to what configuration says",
		Long: `Drops the stored limit for a tier. The configured value applies again from
the next resolution — under a minute, and immediately on the server that handled
this command.`,
		Example: `  krk quota unset llm-tokens`,
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if _, _, err := api.Delete("/quota/tiers/" + strings.TrimSpace(args[0])); err != nil {
				return err
			}
			fmt.Printf("%s is back to what configuration says\n", args[0])
			return nil
		},
	}
	return cmd
}

func quotaTiersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tiers",
		Short: "Show the limits an operator has stored",
		Long: `Lists what has been set with ` + "`krk quota set`" + `, with who set it and why.

An empty list means every tier comes from configuration, which is the state a
fresh deployment is in.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/quota/tiers")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
