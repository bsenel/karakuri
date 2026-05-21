package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bsenel/karakuri/internal/feature/migrate"
)

// migrateCmd: `krk migrate --from <driver:dsn> --to <driver:dsn>`.
// Migration runs locally (it shells out to the storage layer, not the
// karakuri server) so a fresh Postgres can be populated before the server
// is pointed at it. Operates on a stopped server: source DB must be quiescent.
func migrateCmd() *cobra.Command {
	var from, to string
	var batch int
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate Karakuri data between database backends (SQLite ↔ PostgreSQL)",
		Long: `Copies every table from a source DSN to a target DSN. Both DSNs are
specified as "<driver>:<dsn>" pairs, e.g.

  krk migrate \
    --from "sqlite:./karakuri.db" \
    --to   "postgres:postgres://karakuri:secret@localhost:5432/karakuri?sslmode=disable"

The destination schema is created fresh (CREATE TABLE IF NOT EXISTS) before
rows are inserted; existing rows on the destination are not preserved.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			fromDriver, fromDSN, err := parseDSN(from)
			if err != nil {
				return fmt.Errorf("--from: %w", err)
			}
			toDriver, toDSN, err := parseDSN(to)
			if err != nil {
				return fmt.Errorf("--to: %w", err)
			}
			report, err := migrate.Run(context.Background(), migrate.Plan{
				FromDriver: fromDriver, FromDSN: fromDSN,
				ToDriver: toDriver, ToDSN: toDSN,
				BatchSize: batch,
			})
			if err != nil {
				return err
			}
			out, _ := json.MarshalIndent(report, "", "  ")
			_, _ = os.Stdout.Write(out)
			_, _ = os.Stdout.WriteString("\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Source DSN as <driver>:<dsn>, e.g. sqlite:./karakuri.db (required)")
	cmd.Flags().StringVar(&to, "to", "", "Target DSN as <driver>:<dsn>, e.g. postgres:postgres://… (required)")
	cmd.Flags().IntVar(&batch, "batch", 200, "Batch size for inserts")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

// parseDSN splits "<driver>:<dsn>" — the dsn part may itself contain colons
// (postgres URIs do), so we split on the first colon only.
func parseDSN(s string) (string, string, error) {
	driver, dsn, ok := strings.Cut(s, ":")
	if !ok || driver == "" || dsn == "" {
		return "", "", fmt.Errorf("expected <driver>:<dsn>, got %q", s)
	}
	return driver, dsn, nil
}
