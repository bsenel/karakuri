package command

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func verifierCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verifier",
		Short: "Inspect the verifiers that settle success criteria",
	}
	cmd.AddCommand(verifierListCmd())
	return cmd
}

func verifierListCmd() *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List verifiers and the metrics each one declares",
		Long: "List the capabilities marked verifiable, the ones that may settle a\n" +
			"success criterion, with each metric a verifier declares: its name, unit,\n" +
			"direction and the typed parameters it takes. A verifier with no metric\n" +
			"settles verified criteria by pass or fail.",
		Example: `  krk verifier list
  krk verifier list --domain software`,
		RunE: func(c *cobra.Command, _ []string) error {
			path := "/domains/capabilities"
			if domain != "" {
				path += "?domain=" + domain
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			verifiers, err := readVerifiers(data)
			if err != nil {
				return err
			}
			if output != "pretty" {
				out, err := json.Marshal(verifiers)
				if err != nil {
					return err
				}
				client.PrintOutput(out, output)
				return nil
			}
			if len(verifiers) == 0 {
				fmt.Fprintln(c.ErrOrStderr(), "No verifiers.")
				return nil
			}
			printVerifiers(c.OutOrStdout(), verifiers)
			return nil
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "Only show verifiers of this domain ID (see: krk domain list); default: all domains")
	return cmd
}

// shownVerifier is the part of a capability in GET /domains/capabilities that
// krk verifier list prints.
type shownVerifier struct {
	ID         string        `json:"id"`
	Domain     string        `json:"domain"`
	Verifiable bool          `json:"verifiable"`
	Metrics    []shownMetric `json:"metrics,omitempty"`
}

type shownMetric struct {
	Name      string       `json:"name"`
	Unit      string       `json:"unit"`
	Direction string       `json:"direction"`
	Params    []shownParam `json:"params,omitempty"`
}

type shownParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// readVerifiers keeps the capabilities the server marks verifiable; which
// ones those are is the server's to say.
func readVerifiers(data []byte) ([]shownVerifier, error) {
	var caps []shownVerifier
	if err := json.Unmarshal(data, &caps); err != nil {
		return nil, fmt.Errorf("read capabilities: %w", err)
	}
	verifiers := []shownVerifier{}
	for _, cp := range caps {
		if cp.Verifiable {
			verifiers = append(verifiers, cp)
		}
	}
	return verifiers, nil
}

func printVerifiers(w io.Writer, verifiers []shownVerifier) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, v := range verifiers {
		fmt.Fprintf(tw, "%s\n", v.ID)
		for _, m := range v.Metrics {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", m.Name, m.Unit, m.Direction, metricParams(m.Params))
		}
	}
	_ = tw.Flush()
}

func metricParams(params []shownParam) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, p.Name+": "+p.Type)
	}
	return strings.Join(parts, ", ")
}
