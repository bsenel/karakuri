package command

import (
	"fmt"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// standingCmd groups the commands for objectives Karakuri holds rather than
// finishes: declare one, see what its control loop has been doing, reconcile it
// now, stop it, start it again.
//
// They hang off `krk objective` rather than forming a top-level noun because a
// standing objective is an objective — the same row, with a mode on it — and a
// separate `krk standing` would suggest a second kind of thing to keep track of.
func standingCmds() []*cobra.Command {
	return []*cobra.Command{
		objectiveStandingCmd(),
		objectiveUnstandingCmd(),
		objectiveReconcileCmd(),
		objectiveReconcileStatusCmd(),
		objectivePauseCmd(),
		objectiveResumeCmd(),
	}
}

func objectiveStandingCmd() *cobra.Command {
	var (
		sense, every, cronExpr, dailyAt, timezone string
		resync, minInterval                       string
		quiet                                     []string
		autonomy, ceiling                         string
		promoteAfter                              int
		demoteOnFailure                           bool
		budgetDaily, budgetPerReconcile           float64
	)
	cmd := &cobra.Command{
		Use:   "standing <id>",
		Short: "Declare an objective standing: held at its desired state on a cadence",
		Long: `Declare an objective standing.

A one-shot objective converges once and stops. A standing objective is a
desired state Karakuri holds: it senses cheaply on --sense, reconciles when
something drifted or when --every / --cron / --daily-at comes due, and escalates
whatever exceeds the autonomy it has earned.

Sensing costs adapter calls and no model call, so --sense can be minutes where
--every is hours. Quiet windows and --min-interval hold back the expensive tier
only; sensing runs through the night, which is how the morning reconcile knows
what happened.

Autonomy is a ladder — sense, propose, act_with_notice, act — and --ceiling is
the rung it may never pass however well it behaves.

A budget is separate from its twin's allowance and answers a different worry:
an objective reconciling hourly is the one whose appetite nobody has calibrated
yet. Running out of money is not a failure and needs no operator — sensing
continues, the circuit breaker never sees it, earned autonomy survives, and the
daily ceiling clears itself at UTC midnight.`,
		Example: `  # Watch a repository and propose fixes, never acting on its own
  krk objective standing obj_123 --sense 15m --autonomy propose

  # A weekday morning review that may act, having earned its way up to it
  krk objective standing obj_123 \
      --cron "0 8 * * 1-5" --timezone Europe/Istanbul \
      --sense 1h --resync 24h \
      --autonomy propose --ceiling act_with_notice --promote-after 5 \
      --quiet 22:00-07:00

  # Capped: at most 5.00 a day, and no single pass may spend more than 1.00
  krk objective standing obj_123 --every 1h \
      --budget-daily 5.00 --budget-per-reconcile 1.00`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cadence := map[string]any{}
			for key, val := range map[string]string{
				"sense":        sense,
				"every":        every,
				"cron":         cronExpr,
				"daily_at":     dailyAt,
				"timezone":     timezone,
				"resync":       resync,
				"min_interval": minInterval,
			} {
				if val != "" {
					cadence[key] = val
				}
			}
			if len(quiet) > 0 {
				cadence["quiet"] = quiet
			}

			body := map[string]any{}
			if len(cadence) > 0 {
				body["cadence"] = cadence
			}
			if autonomy != "" || ceiling != "" || promoteAfter > 0 || demoteOnFailure {
				a := map[string]any{}
				if autonomy != "" {
					a["level"] = autonomy
				}
				if ceiling != "" {
					a["ceiling"] = ceiling
				}
				if promoteAfter > 0 {
					a["promote_after"] = promoteAfter
				}
				if demoteOnFailure {
					a["demote_on_failure"] = true
				}
				body["autonomy"] = a
			}

			if budgetDaily > 0 || budgetPerReconcile > 0 {
				b := map[string]any{}
				if budgetDaily > 0 {
					b["daily"] = budgetDaily
				}
				if budgetPerReconcile > 0 {
					b["per_reconcile"] = budgetPerReconcile
				}
				body["budget"] = b
			}

			data, _, err := api.Put("/objectives/"+args[0]+"/standing", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&sense, "sense", "", "How often to check cheaply for drift, e.g. 15m (no model call)")
	cmd.Flags().StringVar(&every, "every", "", "Reconcile unconditionally on this interval, e.g. 1h")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "Reconcile on a five-field cron expression, evaluated in --timezone")
	cmd.Flags().StringVar(&dailyAt, "daily-at", "", "Reconcile daily at HH:MM in --timezone")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA timezone for --cron and --daily-at (default UTC)")
	cmd.Flags().StringVar(&resync, "resync", "", "Reconcile at least this often even with no drift, e.g. 24h")
	cmd.Flags().StringVar(&minInterval, "min-interval", "", "Floor between reconciles whatever asked for one, e.g. 10m")
	cmd.Flags().StringSliceVar(&quiet, "quiet", nil, "Blackout windows as HH:MM-HH:MM; work is deferred to the opening, never dropped")
	cmd.Flags().StringVar(&autonomy, "autonomy", "", "Starting level: sense|propose|act_with_notice|act (default propose)")
	cmd.Flags().StringVar(&ceiling, "ceiling", "", "Highest level it may ever earn (default: its starting level)")
	cmd.Flags().IntVar(&promoteAfter, "promote-after", 0, "Consecutive clean reconciles that earn one rung (0 = never promote)")
	cmd.Flags().BoolVar(&demoteOnFailure, "demote-on-failure", false, "Also demote on a failed reconcile, not only on a rejected checkpoint")
	cmd.Flags().Float64Var(&budgetDaily, "budget-daily", 0, "Most this objective may spend per UTC day, separately from its twin's allowance (0 = no ceiling of its own)")
	cmd.Flags().Float64Var(&budgetPerReconcile, "budget-per-reconcile", 0, "Most one reconcile pass may spend (0 = uncapped). Bounds one bad pass; --budget-daily bounds the month")
	return cmd
}

func objectiveUnstandingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unstanding <id>",
		Short: "Stop holding an objective; it becomes one-shot again",
		Long: `Return a standing objective to a one-shot one.

The objective and its history survive. Only the supervision stops: its control
loop is dropped, and nothing runs again unless somebody starts a loop.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if _, _, err := api.Delete("/objectives/" + args[0] + "/standing"); err != nil {
				return err
			}
			fmt.Printf("objective %s is no longer standing\n", args[0])
			return nil
		},
	}
}

func objectiveReconcileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile <id>",
		Short: "Reconcile a standing objective now, outside its cadence",
		Long: `Ask for a reconcile now.

It still goes through the lease and the concurrency bound: "now" means as soon
as this replica has a slot and nobody else holds the objective, not in addition
to whatever is already running.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/objectives/"+args[0]+"/reconcile", map[string]any{})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func objectiveReconcileStatusCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "reconcile-status <id>",
		Short: "Show a standing objective's control loop and recent passes",
		Long: `Show what the supervisor has been doing with an objective.

The history includes the cheap sense-only passes, which are the majority and
the point: "checked forty-eight times today and spent nothing" is the evidence
the two-tier split is working.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := fmt.Sprintf("/objectives/%s/reconcile?limit=%d", args[0], limit)
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "How many recent passes to show")
	return cmd
}

func objectivePauseCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "pause <id>",
		Short: "Stop a standing objective",
		Long: `Take a standing objective out of rotation.

The pause survives a restart, which is the point: a supervisor that forgot it
had been told to stop would put the objective straight back to work.

Give a reason. An objective stopped for a reason nobody wrote down is one
nobody can decide to restart.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/objectives/"+args[0]+"/pause", map[string]any{
				"reason": strings.TrimSpace(reason),
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Why it is being stopped, recorded on the control loop")
	return cmd
}

func objectiveResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Put a paused standing objective back into rotation",
		Long: `Resume a standing objective.

This clears the failure count and the stall streak that stopped it. Clearing
them is the point rather than a convenience: resuming says somebody has looked
at why it broke, and leaving the counters at their ceiling would trip the
breaker again on the next stumble.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Post("/objectives/"+args[0]+"/resume", map[string]any{})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}
