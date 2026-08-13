package command

import (
	"net/url"
	"strconv"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func costCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Report what was spent",
		Long: `Spend is recorded per model call and per tool call, priced from the rate table
in configuration, and attributed to the containers a resource sat in when the
call happened.

A report shows only what you may see. The filter comes from the same bindings
that decide which twins you can list, so a report is not a way around tenancy.

Nothing is priced by default: without a rate table the units are still counted
and the money reads zero, which is the honest answer rather than an invented
one.`,
	}
	cmd.AddCommand(costReportCmd())
	return cmd
}

func costReportCmd() *cobra.Command {
	var (
		since    time.Duration
		twin     string
		org      string
		team     string
		project  string
		provider string
		groupBy  string
		limit    int
	)
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Total what was spent, bucketed however you ask",
		Example: `  krk cost report --since 24h --group-by provider
  krk cost report --twin t_7f2a --since 720h --group-by day
  krk cost report --org acme --team eng --since 720h --group-by model`,
		RunE: func(_ *cobra.Command, _ []string) error {
			q := url.Values{}
			if since > 0 {
				q.Set("since", time.Now().UTC().Add(-since).Format(time.RFC3339))
			}
			if twin != "" {
				q.Set("twin", twin)
			}
			if provider != "" {
				q.Set("provider", provider)
			}
			if groupBy != "" {
				q.Set("group_by", groupBy)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}

			// A container narrows the report to one team or org. The name is
			// resolved to its ID here, as everywhere else — two organisations
			// may each have a team called "eng", and a report matching on the
			// word would total both.
			if org != "" || team != "" || project != "" {
				scope, err := containerScope(org, team, project)
				if err != nil {
					return err
				}
				q.Set("label", scope)
			}

			path := "/cost"
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
	cmd.Flags().DurationVar(&since, "since", 30*24*time.Hour, "how far back to total")
	cmd.Flags().StringVar(&twin, "twin", "", "only this twin's spend")
	cmd.Flags().StringVar(&org, "org", "", "only this organisation's spend")
	cmd.Flags().StringVar(&team, "team", "", "only this team's spend; needs --org")
	cmd.Flags().StringVar(&project, "project", "", "only this project's spend")
	cmd.Flags().StringVar(&provider, "provider", "", "only this provider's spend")
	cmd.Flags().StringVar(&groupBy, "group-by", "",
		"bucket by: day, subject, resource, provider, model, label (comma-separated)")
	cmd.Flags().IntVar(&limit, "limit", 0, "keep only the largest N buckets")
	return cmd
}
