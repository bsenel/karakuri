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
// a pipe sees what it saw before. next says how to get past the empty list:
// the command that makes the thing, or, when no command makes it directly,
// what makes it appear. Empty next means there is nothing useful to add.
func printList(c *cobra.Command, data []byte, noun, next string) {
	client.PrintOutput(data, output)
	if output == "json" || output == "quiet" {
		return
	}
	if body := string(bytes.TrimSpace(data)); body == "null" || body == "[]" {
		if next == "" {
			fmt.Fprintf(c.ErrOrStderr(), "No %s.\n", noun)
			return
		}
		fmt.Fprintf(c.ErrOrStderr(), "No %s. %s\n", noun, next)
	}
}

// createWith is the next step for a list whose rows a command makes.
func createWith(command string) string {
	return "Create one with: " + command
}

// fromLoop is the next step for a list whose rows a running loop writes.
const fromLoop = "A running loop writes them; start one with: krk loop start <objective-id>"
