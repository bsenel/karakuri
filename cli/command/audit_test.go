package command

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// auditServer stands in for the API, answering GET /api/v1/audit with an empty
// log and recording what the CLI asked for.
type auditServer struct {
	path, method string
	query        url.Values
}

func startAuditServer(t *testing.T) (*auditServer, string) {
	t.Helper()
	s := &auditServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.path, s.method, s.query = r.URL.Path, r.Method, r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
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

// Each flag is sent as the query parameter the server filters on, and only the
// flags that were given are sent: a parameter present and empty is a filter on
// the empty value to anything that reads it strictly.
func TestAuditSendsTheProvenanceFilters(t *testing.T) {
	cases := map[string]struct {
		args []string
		want map[string]string
	}{
		"provider": {
			args: []string{"--provider", "anthropic"},
			want: map[string]string{"provider": "anthropic"},
		},
		"model": {
			args: []string{"--model", "model-a"},
			want: map[string]string{"model": "model-a"},
		},
		"template": {
			args: []string{"--template", "tmpl-green-build"},
			want: map[string]string{"template": "tmpl-green-build"},
		},
		"all three": {
			args: []string{"--provider", "anthropic", "--model", "model-a", "--template", "tmpl-green-build"},
			want: map[string]string{"provider": "anthropic", "model": "model-a", "template": "tmpl-green-build"},
		},
		"none": {want: map[string]string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, apiURL := startAuditServer(t)

			args := append([]string{"--api-url", apiURL, "--output", "quiet", "audit"}, tc.args...)
			runKrk(t, args...)

			if srv.method != http.MethodGet || srv.path != "/api/v1/audit" {
				t.Fatalf("request = %s %s, want GET /api/v1/audit", srv.method, srv.path)
			}
			for _, param := range []string{"provider", "model", "template"} {
				want, sent := tc.want[param]
				if got := srv.query.Get(param); got != want {
					t.Errorf("%s = %q, want %q", param, got, want)
				}
				if !sent && srv.query.Has(param) {
					t.Errorf("%s was sent without its flag being given", param)
				}
			}
		})
	}
}
