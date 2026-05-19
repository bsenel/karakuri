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
	cmd.AddCommand(domainListCmd(), domainCapabilitiesCmd())
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
