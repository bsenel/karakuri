package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/api/handler"
	corecheckpoint "github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/objective"
	featurecp "github.com/bsenel/karakuri/internal/feature/checkpoint"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// Phase 35 step 4: a reviewer reads when a checkpoint will lapse from the
// routes they already use, and the specification says so.

// checkpointStore is the two reads the checkpoint routes make.
type checkpointStore struct {
	storage.StorageAdapter
	checkpoints []corecheckpoint.Checkpoint
}

func (s *checkpointStore) GetCheckpoint(_ context.Context, id string) (corecheckpoint.Checkpoint, error) {
	for _, c := range s.checkpoints {
		if c.ID == id {
			return c, nil
		}
	}
	return corecheckpoint.Checkpoint{}, errors.New("checkpoint not found")
}

func (s *checkpointStore) ListPendingCheckpoints(_ context.Context, twinID string) ([]corecheckpoint.Checkpoint, error) {
	out := []corecheckpoint.Checkpoint{}
	for _, c := range s.checkpoints {
		if c.Status == corecheckpoint.StatusPending && (twinID == "" || c.TwinID == twinID) {
			out = append(out, c)
		}
	}
	return out, nil
}

var checkpointExpiry = time.Date(2026, 10, 10, 12, 30, 0, 0, time.UTC)

func pendingCheckpoint(id string, expiresAt *time.Time) corecheckpoint.Checkpoint {
	return corecheckpoint.Checkpoint{
		ID: id, ObjectiveID: objective.ObjectiveID("obj-" + id), TwinID: "twin-a",
		Summary: "summary " + id, Options: []string{"approve", "reject", "modify"},
		Status:    corecheckpoint.StatusPending,
		CreatedAt: checkpointExpiry.Add(-time.Hour),
		ExpiresAt: expiresAt,
	}
}

// checkpointRoutes serves one checkpoint stored with an expiry and one stored
// without, on the paths the server mounts.
func checkpointRoutes() http.Handler {
	expiry := checkpointExpiry
	store := &checkpointStore{checkpoints: []corecheckpoint.Checkpoint{
		pendingCheckpoint("cp-expiring", &expiry),
		pendingCheckpoint("cp-waiting", nil),
	}}
	h := &handler.CheckpointHandler{Checkpoints: featurecp.NewService(store, event.NewHub())}
	r := chi.NewRouter()
	r.Get("/api/v1/checkpoints", h.ListPending)
	r.Get("/api/v1/checkpoints/{id}", h.Get)
	return r
}

// assertExpiresAt checks one checkpoint body: the instant as RFC 3339 when it
// was stored with one, and no key at all when it was not.
func assertExpiresAt(t *testing.T, where string, body map[string]any) {
	t.Helper()
	id, _ := body["id"].(string)
	raw, present := body["expires_at"]
	switch id {
	case "cp-expiring":
		if !present {
			t.Errorf("%s: checkpoint %q has no expires_at: %v", where, id, body)
			return
		}
		text, _ := raw.(string)
		got, err := time.Parse(time.RFC3339, text)
		if err != nil {
			t.Errorf("%s: expires_at = %v, want an RFC 3339 instant: %v", where, raw, err)
			return
		}
		if !got.Equal(checkpointExpiry) {
			t.Errorf("%s: expires_at = %s, want %s", where, text, checkpointExpiry.Format(time.RFC3339))
		}
	case "cp-waiting":
		if present {
			t.Errorf("%s: checkpoint %q carries expires_at = %v, want the key omitted", where, id, raw)
		}
	default:
		t.Errorf("%s: a checkpoint nobody stored: %v", where, body["id"])
	}
}

func TestGetCheckpointCarriesExpiresAtOnlyWhenSet(t *testing.T) {
	routes := checkpointRoutes()
	for _, id := range []string{"cp-expiring", "cp-waiting"} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/checkpoints/"+id, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200: %s", id, rec.Code, rec.Body)
		}
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: response is not a JSON object: %v: %s", id, err, rec.Body)
		}
		if got["id"] != id {
			t.Fatalf("read %q, got checkpoint %v", id, got["id"])
		}
		assertExpiresAt(t, "GET /checkpoints/"+id, got)
	}
}

func TestListPendingCheckpointsCarriesExpiresAtOnlyWhenSet(t *testing.T) {
	rec := httptest.NewRecorder()
	checkpointRoutes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/checkpoints?twin_id=twin-a", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("response is not a JSON array: %v: %s", err, rec.Body)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d checkpoints, want 2: %s", len(listed), rec.Body)
	}
	for _, cp := range listed {
		assertExpiresAt(t, "GET /checkpoints", cp)
	}
}

// The specification is what a client author reads instead of the handler: the
// checkpoint schema names expires_at as an optional date-time.
func TestOpenAPICheckpointSchemaDeclaresExpiresAt(t *testing.T) {
	const specPath = "../../../docs/openapi.yaml"
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read the specification: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string `yaml:"required"`
				Properties map[string]struct {
					Type   string `yaml:"type"`
					Format string `yaml:"format"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("%s is not YAML: %v", specPath, err)
	}
	schema, ok := spec.Components.Schemas["Checkpoint"]
	if !ok {
		t.Fatalf("%s has no components.schemas.Checkpoint", specPath)
	}
	prop, ok := schema.Properties["expires_at"]
	if !ok {
		t.Fatalf("Checkpoint schema has no property expires_at")
	}
	if prop.Type != "string" || prop.Format != "date-time" {
		t.Errorf("expires_at is type %q format %q, want string date-time", prop.Type, prop.Format)
	}
	if slices.Contains(schema.Required, "expires_at") {
		t.Errorf("expires_at is listed as required; a checkpoint without a limit omits it")
	}
}
