package command

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/cli/client"
)

// A refused objective is the first error a new user reads. The server's reason
// comes out as a sentence, not as a JSON body, and the error says where the
// flags are explained.
func TestObjectiveCreateRefusalIsASentence(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want, not  []string
	}{
		{
			name: "json error body",
			body: `{"error":"title is required"}`,
			want: []string{"create objective: title is required (HTTP 400)", "krk objective create --help"},
			not:  []string{`{"error"`},
		},
		{
			name: "json error body naming a field",
			body: `{"error":"a judged criterion needs evidence","field":"success_criteria[1].evidence"}`,
			want: []string{"create objective: success_criteria[1].evidence: a judged criterion needs evidence (HTTP 400)"},
			not:  []string{`{"error"`},
		},
		{
			name: "plain text body is kept whole",
			body: "twin not bound\n",
			want: []string{"create objective: HTTP 400: twin not bound\n", "krk objective create --help"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
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

			_, _, err := runKrkSplit("--api-url", apiURL, "objective", "create", "--title", "Ship it")
			if err == nil {
				t.Fatal("a 400 from POST /objectives = nil error, want one")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			for _, not := range tc.not {
				if strings.Contains(err.Error(), not) {
					t.Errorf("error %q still contains %q", err, not)
				}
			}
		})
	}
}
