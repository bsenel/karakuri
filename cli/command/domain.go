package command

import (
	"fmt"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func domainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Inspect registered domain packs",
		Example: `  krk domain list
  krk domain capabilities`,
	}
	cmd.AddCommand(domainListCmd(), domainCapabilitiesCmd(), domainTestCmd())
	return cmd
}

func domainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered domain packs",
		Example: `  krk domain list
  krk domain list --output json`,
		RunE: func(c *cobra.Command, _ []string) error {
			data, _, err := api.Get("/domains")
			if err != nil {
				return err
			}
			printList(c, data, "domain packs", "")
			return nil
		},
	}
}

func domainTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <domain-id>",
		Short: "Run conformance suite against a registered domain pack",
		Example: `  krk domain list            # find the domain ID
  krk domain test software`,
		Args: cobra.ExactArgs(1),
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

// unknownDomainHint is the next step when --domain matches nothing, which is
// usually a mistyped domain ID.
const unknownDomainHint = "Check the domain ID: krk domain list shows the ones that exist"

func domainCapabilitiesCmd() *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "List the capabilities domain packs provide",
		Example: `  krk domain capabilities
  krk domain capabilities --domain software`,
		RunE: func(c *cobra.Command, _ []string) error {
			path := "/domains/capabilities"
			if domain != "" {
				path += "?domain=" + domain
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			if domain != "" {
				printList(c, data, fmt.Sprintf("capabilities in domain %q", domain), unknownDomainHint)
				return nil
			}
			printList(c, data, "capabilities", "")
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "Only show capabilities of this domain ID (see: krk domain list); default: all domains")
	return cmd
}
