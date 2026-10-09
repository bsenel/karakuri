package command

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// exportBody is deliberately not canonical JSON and has no trailing newline: a
// CLI that decodes, re-encodes, pretty-prints or appends anything changes it.
const exportBody = `{"b":1,  "a":2}`

const (
	exportFrom = "2026-01-01T00:00:00Z"
	exportTo   = "2026-02-01T00:00:00+00:00"
)

// exportServer stands in for GET /api/v1/audit/export, answering status and
// exportBody and recording every request it saw.
type exportServer struct {
	status   int
	requests int
	path     string
	query    url.Values
}

func startExportServer(t *testing.T, status int) (*exportServer, string) {
	t.Helper()
	s := &exportServer{status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		s.path, s.query = r.URL.Path, r.URL.Query()
		if s.status != http.StatusOK {
			http.Error(w, "audit export: invalid window", s.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, exportBody)
	}))
	t.Cleanup(srv.Close)

	apiURL := srv.URL + "/api/v1"
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(apiURL, client.Session{
		AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	t.Cleanup(func() { api = nil })
	return s, apiURL
}

// runKrkSplit runs krk with stdout and stderr captured apart, which is the
// whole point of the export: stdout is the document, stderr is its digest.
func runKrkSplit(args ...string) (stdout, stderr string, err error) {
	root := NewRoot()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func exportDigest() string {
	sum := sha256.Sum256([]byte(exportBody))
	return hex.EncodeToString(sum[:])
}

// Without --out the server's bytes are stdout, whatever the output format.
func TestAuditExportWritesTheServersBytesToStdout(t *testing.T) {
	for _, format := range []string{"json", "pretty", "quiet"} {
		t.Run(format, func(t *testing.T) {
			_, apiURL := startExportServer(t, http.StatusOK)

			stdout, stderr, err := runKrkSplit("--api-url", apiURL, "--output", format,
				"audit", "export", "--from", exportFrom, "--to", exportTo)
			if err != nil {
				t.Fatalf("krk audit export: %v\n%s", err, stderr)
			}
			if stdout != exportBody {
				t.Errorf("stdout = %q, want exactly %q", stdout, exportBody)
			}
			if !strings.Contains(stderr, exportDigest()) {
				t.Errorf("stderr = %q, want the SHA-256 %s", stderr, exportDigest())
			}
		})
	}
}

// With --out the bytes go to a file only its owner can read, and stdout stays
// empty.
func TestAuditExportWritesTheServersBytesToAFile(t *testing.T) {
	_, apiURL := startExportServer(t, http.StatusOK)
	out := filepath.Join(t.TempDir(), "export.json")

	stdout, stderr, err := runKrkSplit("--api-url", apiURL, "--output", "json",
		"audit", "export", "--from", exportFrom, "--to", exportTo, "--out", out)
	if err != nil {
		t.Fatalf("krk audit export --out: %v\n%s", err, stderr)
	}
	got, err := os.ReadFile(out) // #nosec G304 -- out is a file under t.TempDir() this test named.
	if err != nil {
		t.Fatalf("read %s: %v", out, err)
	}
	if string(got) != exportBody {
		t.Errorf("file = %q, want exactly %q", got, exportBody)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat %s: %v", out, err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %o, want 600", perm)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, exportDigest()) {
		t.Errorf("stderr = %q, want the SHA-256 %s", stderr, exportDigest())
	}
}

// --from and --to are the server's from and to, as typed.
func TestAuditExportSendsTheWindowUnchanged(t *testing.T) {
	srv, apiURL := startExportServer(t, http.StatusOK)

	if _, stderr, err := runKrkSplit("--api-url", apiURL, "audit", "export", "--from", exportFrom, "--to", exportTo); err != nil {
		t.Fatalf("krk audit export: %v\n%s", err, stderr)
	}
	if srv.path != "/api/v1/audit/export" {
		t.Fatalf("path = %q, want /api/v1/audit/export", srv.path)
	}
	if !srv.query.Has("from") || !srv.query.Has("to") {
		t.Fatalf("query = %v, want both from and to", srv.query)
	}
	if got := srv.query.Get("from"); got != exportFrom {
		t.Errorf("from = %q, want %q", got, exportFrom)
	}
	if got := srv.query.Get("to"); got != exportTo {
		t.Errorf("to = %q, want %q", got, exportTo)
	}
}

// A window with one end missing is refused before the server is asked.
func TestAuditExportRequiresFromAndTo(t *testing.T) {
	cases := map[string][]string{
		"no from": {"--to", exportTo},
		"no to":   {"--from", exportFrom},
		"neither": nil,
	}
	for name, flags := range cases {
		t.Run(name, func(t *testing.T) {
			srv, apiURL := startExportServer(t, http.StatusOK)

			args := append([]string{"--api-url", apiURL, "audit", "export"}, flags...)
			stdout, _, err := runKrkSplit(args...)
			if err == nil {
				t.Errorf("krk %s succeeded, want an error", strings.Join(args, " "))
			}
			if srv.requests != 0 {
				t.Errorf("server saw %d requests, want none", srv.requests)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
		})
	}
}

// A refused window is an error, and leaves no file behind to be mistaken for
// an export.
func TestAuditExportRefusalIsAnErrorAndWritesNoFile(t *testing.T) {
	srv, apiURL := startExportServer(t, http.StatusBadRequest)
	out := filepath.Join(t.TempDir(), "export.json")

	stdout, _, err := runKrkSplit("--api-url", apiURL, "audit", "export", "--from", exportFrom, "--to", exportTo, "--out", out)
	if err == nil {
		t.Fatal("krk audit export succeeded on a 400, want an error")
	}
	if srv.requests == 0 {
		t.Error("server saw no request, want the 400 to come from it")
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Errorf("stat %s = %v, want no file", out, statErr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
}

// The subcommand takes nothing from the listing it hangs off.
func TestAuditListingIsUnchangedByTheExportSubcommand(t *testing.T) {
	srv, apiURL := startAuditServer(t)

	runKrk(t, "--api-url", apiURL, "--output", "quiet", "audit", "--kind", "escalation", "--limit", "5")

	if srv.method != http.MethodGet || srv.path != "/api/v1/audit" {
		t.Fatalf("request = %s %s, want GET /api/v1/audit", srv.method, srv.path)
	}
	if got := srv.query.Get("kind"); got != "escalation" {
		t.Errorf("kind = %q, want escalation", got)
	}
	if got := srv.query.Get("limit"); got != "5" {
		t.Errorf("limit = %q, want 5", got)
	}
}
