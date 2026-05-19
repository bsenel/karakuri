package command

import (
	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func domainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Inspect registered domain packs",
	}
	cmd.AddCommand(domainListCmd(), domainCapabilitiesCmd(), domainTestCmd())
	return cmd
}

func domainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered domain packs",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/domains")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func domainTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <domain-id>",
		Short: "Run conformance suite against a registered domain pack",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			data, _, err := api.Get("/domains/" + args[0] + "/conformance")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func domainCapabilitiesCmd() *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "List capabilities",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/domains/capabilities"
			if domain != "" {
				path += "?domain=" + domain
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "Filter by domain")
	return cmd
}
