package command

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A refused event stream must carry the server's reason and say what to do
// next, not only a status code.
func TestStreamSSEErrorShowsReasonAndNextStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "objective obj-1 not found", http.StatusNotFound)
	}))
	defer srv.Close()

	err := streamSSE(srv.URL+"/objectives/obj-1/events", "")
	if err == nil {
		t.Fatal("expected an error for a 404 event stream")
	}
	for _, want := range []string{"status 404", "objective obj-1 not found", "krk objective list"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestStreamSSEUnreachableNamesFlag(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	err := streamSSE(url+"/objectives/obj-1/events", "")
	if err == nil {
		t.Fatal("expected an error for a closed server")
	}
	for _, want := range []string{"cannot connect to the event stream", "--api-url"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}
