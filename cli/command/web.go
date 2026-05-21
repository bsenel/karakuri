package command

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// webCmd runs the React dev server in web/ (npm run dev). Convenience wrapper
// so `krk web` is symmetrical with the server-side `make run-server`.
//
// Requires Node 18+ and the web/ workspace's node_modules to be present
// (`npm install` once before first use). The dev server proxies /api → the
// Karakuri API on :8080 (configured in web/vite.config.ts).
func webCmd() *cobra.Command {
	var install bool
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Launch the React dev server (web/) — proxies to :8080",
		RunE: func(_ *cobra.Command, _ []string) error {
			if install {
				if err := run("npm", "install"); err != nil {
					return err
				}
			}
			return run("npm", "run", "dev")
		},
	}
	cmd.Flags().BoolVar(&install, "install", false, "Run `npm install` before starting the dev server")
	return cmd
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = "web"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if _, err := os.Stat("web/package.json"); err != nil {
		return fmt.Errorf("web/package.json not found — run this from the Karakuri repo root")
	}
	return cmd.Run()
}
