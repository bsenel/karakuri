package command

import (
	"net/http"
	"strings"
	"testing"
)

// --title "" and --title "   " pass MarkFlagRequired; the command refuses
// them before any request reaches the API.
func TestObjectiveCreateRefusesABlankTitle(t *testing.T) {
	for name, title := range map[string]string{"empty": "", "spaces": "   "} {
		t.Run(name, func(t *testing.T) {
			srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

			stdout, stderr, err := runKrkSplit("--api-url", apiURL, "--output", "quiet",
				"objective", "create", "--title", title, "--twin", "t_7f2a")

			if err == nil {
				t.Fatalf("--title %q exited zero; stdout %q", title, stdout)
			}
			if seen := err.Error() + stderr; !strings.Contains(seen, "--title is empty") {
				t.Errorf("the user saw %q, which does not say --title is empty", seen)
			}
			if srv.calls != 0 {
				t.Errorf("--title %q sent %d requests, want none", title, srv.calls)
			}
		})
	}
}

// A title with words in it still sends one create request carrying it.
func TestObjectiveCreateSendsANamedTitle(t *testing.T) {
	srv, apiURL := startCreateServer(t, http.StatusCreated, `{"id":"obj_1"}`)

	if _, stderr, err := runKrkSplit(createArgs(apiURL)...); err != nil {
		t.Fatalf("krk objective create: %v\n%s", err, stderr)
	}
	if srv.calls != 1 || srv.method != http.MethodPost || srv.path != "/api/v1/objectives" {
		t.Fatalf("server got %d requests, last %s %s; want one POST /api/v1/objectives",
			srv.calls, srv.method, srv.path)
	}
	if got := string(srv.body["title"]); got != `"Ship the export endpoint"` {
		t.Errorf("title sent = %s, want \"Ship the export endpoint\"", got)
	}
}
