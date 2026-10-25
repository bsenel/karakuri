package integration_test

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// apiPrefix is where the server mounts the API; the spec's server URL.
const apiPrefix = "/api/v1"

// docs/openapi.yaml is the API contract (AGENTS.md rule 6). It had shrunk to
// two paths while the server mounted 87 routes, because nothing compared the
// two. This walks the router the server really builds and fails on a route the
// spec does not list, and on a listed route the server no longer mounts.
func TestOpenAPIDocumentsEveryRoute(t *testing.T) {
	app, _, removeDB := newTestApp(t, nil)
	defer removeDB()

	mounted := map[string]bool{}
	err := chi.Walk(app.Router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		// The SPA's catch-all serves the web app, not the API.
		if !strings.HasPrefix(route, apiPrefix+"/") {
			return nil
		}
		mounted[strings.ToLower(method)+" "+strings.TrimPrefix(route, apiPrefix)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	documented := specOperations(t)

	var missing, stale []string
	for op := range mounted {
		if !documented[op] {
			missing = append(missing, op)
		}
	}
	for op := range documented {
		if !mounted[op] {
			stale = append(stale, op)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	for _, op := range missing {
		t.Errorf("route %q is mounted but not in docs/openapi.yaml", op)
	}
	for _, op := range stale {
		t.Errorf("docs/openapi.yaml lists %q but the server does not mount it", op)
	}
	if len(mounted) == 0 {
		t.Fatal("walked no API routes; the check would pass vacuously")
	}
}

// specOperations reads "method path" pairs from docs/openapi.yaml.
func specOperations(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	ops := map[string]bool{}
	for path, item := range spec.Paths {
		for method := range item {
			switch method {
			case "get", "put", "post", "delete", "patch", "head", "options", "trace":
				ops[method+" "+path] = true
			}
		}
	}
	return ops
}
