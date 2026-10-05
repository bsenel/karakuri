package command

import (
	"bytes"
	"fmt"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// printList prints a list response. An empty list reaches the CLI as a bare
// null or [], which reads like a failure; in the pretty format a line on
// stderr says there is nothing to show. Stdout is the same in every format, so
// a pipe sees what it saw before.
func printList(c *cobra.Command, data []byte, noun string) {
	client.PrintOutput(data, output)
	if output == "json" || output == "quiet" {
		return
	}
	if body := string(bytes.TrimSpace(data)); body == "null" || body == "[]" {
		fmt.Fprintf(c.ErrOrStderr(), "No %s.\n", noun)
	}
}
