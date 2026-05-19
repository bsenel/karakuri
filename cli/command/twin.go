package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func twinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "twin",
		Short: "Manage digital twins",
	}
	cmd.AddCommand(twinCreateCmd(), twinGetCmd(), twinListCmd())
	return cmd
}

func twinCreateCmd() *cobra.Command {
	var name, kind, domain string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a digital twin",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Post("/twins", map[string]string{
				"name": name, "kind": kind, "domain": domain,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Twin name (required)")
	cmd.Flags().StringVar(&kind, "kind", "team", "Twin kind: person|team|organization")
	cmd.Flags().StringVar(&domain, "domain", "software", "Domain")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func twinGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a twin by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/twins/" + args[0])
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func twinListCmd() *cobra.Command {
	var kind, domain string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List digital twins",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/twins"
			sep := "?"
			if kind != "" {
				path += sep + "kind=" + kind
				sep = "&"
			}
			if domain != "" {
				path += sep + "domain=" + domain
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Filter by kind")
	cmd.Flags().StringVar(&domain, "domain", "", "Filter by domain")
	return cmd
}
