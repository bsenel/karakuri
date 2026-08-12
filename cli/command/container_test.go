package command

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// containerServer stands in for the API, answering /containers from a fixed
// tree. It records the last query so a test can check what the CLI asked for —
// which is the whole point of resolution: a team lookup must be qualified by
// its organisation.
type containerServer struct {
	containers []map[string]string
	lastQuery  url.Values
}

func (s *containerServer) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.lastQuery = r.URL.Query()
		var out []map[string]string
		for _, c := range s.containers {
			if kind := s.lastQuery.Get("kind"); kind != "" && c["kind"] != kind {
				continue
			}
			if name := s.lastQuery.Get("name"); name != "" && c["name"] != name {
				continue
			}
			if parent := s.lastQuery.Get("parent_id"); parent != "" && c["parent_id"] != parent {
				continue
			}
			out = append(out, c)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)

	// The client reads cached credentials from disk; give it a session of its
	// own so this run neither needs nor touches the developer's.
	t.Setenv("KARAKURI_CREDENTIALS", filepath.Join(t.TempDir(), "credentials.json"))
	if err := client.SaveSession(srv.URL, client.Session{
		AccessToken: "test-token", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	api = client.New(srv.URL)
	t.Cleanup(func() { api = nil })
}

// twoTenants is the case the design exists for: both organisations have a team
// called "eng".
func twoTenants(t *testing.T) *containerServer {
	t.Helper()
	s := &containerServer{containers: []map[string]string{
		{"id": "o_acme", "kind": "org", "name": "acme"},
		{"id": "o_globex", "kind": "org", "name": "globex"},
		{"id": "t_acme_eng", "kind": "team", "name": "eng", "parent_id": "o_acme"},
		{"id": "t_globex_eng", "kind": "team", "name": "eng", "parent_id": "o_globex"},
		{"id": "p_delta", "kind": "project", "name": "delta"},
	}}
	s.start(t)
	return s
}

func TestResolveContainerQualifiesATeamByItsOrg(t *testing.T) {
	srv := twoTenants(t)

	acme, err := resolveContainer("org", "acme", "")
	if err != nil {
		t.Fatalf("resolve org: %v", err)
	}
	if acme != "o_acme" {
		t.Fatalf("org = %q", acme)
	}

	team, err := resolveContainer("team", "eng", acme)
	if err != nil {
		t.Fatalf("resolve team: %v", err)
	}
	if team != "t_acme_eng" {
		t.Fatalf("team = %q, want acme's", team)
	}
	if got := srv.lastQuery.Get("parent_id"); got != "o_acme" {
		t.Fatalf("looked up the team with parent_id=%q — an unqualified lookup can hit the wrong tenant", got)
	}
}

// An ambiguous name is an error, never a guess: picking one silently is how a
// grant lands in the wrong tenant.
func TestResolveContainerRefusesAnAmbiguousName(t *testing.T) {
	twoTenants(t)

	_, err := resolveContainer("team", "eng", "")
	if err == nil {
		t.Fatal("an unqualified team name resolved to something")
	}
	if !strings.Contains(err.Error(), "--org") {
		t.Errorf("err = %v, want it to say how to disambiguate", err)
	}
}

func TestResolveContainerRefusesAMissingName(t *testing.T) {
	twoTenants(t)

	if _, err := resolveContainer("org", "widgets", ""); err == nil {
		t.Fatal("a name that matches nothing resolved")
	}
	if _, err := resolveContainer("org", "", ""); err == nil {
		t.Fatal("an empty name resolved")
	}
}

// containerScope is what turns --org/--team/--project on `krk auth bindings
// add` into the scope a binding stores. It must never emit a display name.
func TestContainerScope(t *testing.T) {
	twoTenants(t)

	cases := []struct {
		name               string
		org, team, project string
		want               string
		wantErrContains    string
	}{
		{name: "nothing named", want: ""},
		{name: "an org", org: "acme", want: "org:o_acme"},
		{name: "a team in acme", org: "acme", team: "eng", want: "team:t_acme_eng"},
		{name: "the same team name in globex", org: "globex", team: "eng", want: "team:t_globex_eng"},
		{name: "a project", project: "delta", want: "project:p_delta"},
		{
			name: "a team with no org", team: "eng",
			wantErrContains: "--team needs --org",
		},
		{
			name: "a project qualified by an org", project: "delta", org: "acme",
			wantErrContains: "spans organisations",
		},
		{name: "an unknown org", org: "widgets", wantErrContains: `no org called "widgets"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := containerScope(tc.org, tc.team, tc.project)
			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("scope = %q, want an error", got)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("containerScope: %v", err)
			}
			if got != tc.want {
				t.Fatalf("scope = %q, want %q", got, tc.want)
			}
		})
	}

	// The two teams called "eng" resolve to different scopes, which is the
	// property the whole design turns on.
	acme, _ := containerScope("acme", "eng", "")
	globex, _ := containerScope("globex", "eng", "")
	if acme == globex {
		t.Fatalf("both tenants' eng teams resolved to %q", acme)
	}
}
