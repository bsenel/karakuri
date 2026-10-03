package integration_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	domainsw "github.com/bsenel/karakuri/domains/software"
	"github.com/bsenel/karakuri/internal/feature/audit"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// A window that closed long before any test runs.
const (
	auditExportFrom = "2026-01-01T00:00:00Z"
	auditExportTo   = "2026-02-01T00:00:00Z"
)

func auditExportURL(base string) string {
	q := url.Values{"from": {auditExportFrom}, "to": {auditExportTo}}
	return base + "/api/v1/audit/export?" + q.Encode()
}

func getBody(t *testing.T, token, target string) (int, []byte) {
	t.Helper()
	resp := doJSON(t, token, http.MethodGet, target, nil)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	return resp.StatusCode, body
}

// The export sits behind audit:read, the action the listing demands.
func TestAuditExportRequiresAuditRead(t *testing.T) {
	base, admin, cleanup := startServer(t)
	defer cleanup()

	viewer := createUser(t, base, admin, "export-viewer", "viewer")
	if status, body := getBody(t, viewer, auditExportURL(base)); status != http.StatusForbidden {
		t.Errorf("viewer: status = %d, want 403 (body %q)", status, body)
	}
	auditor := createUser(t, base, admin, "export-auditor", "auditor")
	if status, body := getBody(t, auditor, auditExportURL(base)); status != http.StatusOK {
		t.Errorf("auditor: status = %d, want 200 (body %q)", status, body)
	}
}

// Whoever can read the listing can read the export, with no further scope,
// and whoever cannot read one cannot read the other.
func TestAuditExportIsScopedAsTheListing(t *testing.T) {
	base, admin, cleanup := startServer(t)
	defer cleanup()

	for role, want := range map[string]int{"auditor": http.StatusOK, "viewer": http.StatusForbidden} {
		token := createUser(t, base, admin, "scope-"+role, role)
		list, _ := getBody(t, token, base+"/api/v1/audit")
		export, body := getBody(t, token, auditExportURL(base))
		if list != want {
			t.Errorf("%s: GET /audit = %d, want %d", role, list, want)
		}
		if export != list {
			t.Errorf("%s: GET /audit/export = %d, GET /audit = %d, want the same (body %q)", role, export, list, body)
		}
	}
}

// "export" is a route of its own, not an audit event ID.
func TestAuditExportIsNotServedAsAnEventID(t *testing.T) {
	base, admin, cleanup := startServer(t)
	defer cleanup()

	status, body := getBody(t, admin, auditExportURL(base))
	if strings.Contains(string(body), "no such audit event") {
		t.Fatalf("GET /audit/export was answered by /audit/{id}: %d %q", status, body)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 (body %q)", status, body)
	}
}

// The app the normal constructor builds serves the export from a real
// audit.Exporter over its store, under the retention it was configured with.
func TestAuditExportIsWiredToTheExporter(t *testing.T) {
	var dsn string
	var retention config.AuditRetentionConfig
	base, admin, cleanup := startServerWith(t, func(cfg *config.Config) {
		dsn, retention = cfg.Database.DSN, cfg.Audit.Retention
	})
	defer cleanup()

	// A second handle on the server's database, to seed it and to ask the
	// exporter directly.
	gormDB, err := platformdb.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	store := storage.NewGORMStorage(gormDB)
	ctx := context.Background()
	if err := store.SaveToolEvent(ctx, storage.ToolEvent{
		ID: "ev-export-1", ObjectiveID: "obj-export", Kind: storage.ToolEventExecute,
		Capability: "code.review", Success: true,
		CreatedAt: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed tool event: %v", err)
	}

	// The shipped default: never prune, at the declared floor.
	keep := audit.Retention{FloorDays: retention.FloorDays, Days: retention.Days}
	if keep.FloorDays == 0 {
		keep.FloorDays = 183
	}
	from, _ := time.Parse(time.RFC3339, auditExportFrom)
	to, _ := time.Parse(time.RFC3339, auditExportTo)
	want, err := audit.NewExporter(store, keep, domainsw.New().ObjectiveTemplates()).Export(ctx, from, to, time.Now())
	if err != nil {
		t.Fatalf("export directly: %v", err)
	}
	if !bytes.Contains(want, []byte("ev-export-1")) {
		t.Fatalf("direct export does not hold the seeded event: %s", want)
	}

	status, first := getBody(t, admin, auditExportURL(base))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", status, first)
	}
	if !bytes.Equal(first, want) {
		t.Errorf("served export differs from the exporter's bytes\n got: %s\nwant: %s", first, want)
	}
	_, second := getBody(t, admin, auditExportURL(base))
	if !bytes.Equal(first, second) {
		t.Errorf("two requests for one window differ\n first: %s\nsecond: %s", first, second)
	}
}
