package objective

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type CreateRequest struct {
	Title             string
	Description       string
	Domain            string
	AdditionalDomains []string
	Priority          int
	MaxIterations     int // 0 = use server default at loop-start time
	TwinID            string
	TemplateID        string // optional; populates criteria/constraints if set
}

// Containers places an objective in the tenancy tree. It is an interface so
// this package depends on the one operation it needs rather than on the whole
// container service.
type Containers interface {
	InheritFrom(ctx context.Context, resourceType, resourceID, fromType, fromID string) error
}

type Service struct {
	store      storage.StorageAdapter
	templates  map[string]objective.Template
	containers Containers
}

func NewService(store storage.StorageAdapter) *Service {
	return &Service{store: store, templates: make(map[string]objective.Template)}
}

// WithContainers makes new objectives inherit their twin's containers, so a
// grant on an organisation reaches the work being done in it and not only the
// twins themselves.
func (s *Service) WithContainers(c Containers) *Service {
	s.containers = c
	return s
}

func (s *Service) RegisterTemplate(t objective.Template) {
	s.templates[t.ID] = t
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (objective.Objective, error) {
	id, _ := newID()
	o := objective.Objective{
		ID: objective.ObjectiveID(id), Title: req.Title, Description: req.Description,
		Domain: req.Domain, AdditionalDomains: req.AdditionalDomains,
		TwinID: req.TwinID, Priority: req.Priority, MaxIterations: req.MaxIterations,
		Status:    objective.StatusPending,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if req.TemplateID != "" {
		if tmpl, ok := s.templates[req.TemplateID]; ok {
			o.SuccessCriteria = tmpl.SuccessCriteria
			o.Constraints = tmpl.Constraints
		}
	}
	if err := s.store.SaveObjective(ctx, o); err != nil {
		return objective.Objective{}, fmt.Errorf("save objective: %w", err)
	}
	if s.containers != nil && req.TwinID != "" {
		// An objective belongs where its twin belongs. A failure here would
		// leave it in no container — visible only to unscoped grants — so it is
		// reported rather than swallowed, even though the objective is already
		// saved.
		if err := s.containers.InheritFrom(ctx, "objective", id, "twin", req.TwinID); err != nil {
			return o, fmt.Errorf("place objective in its twin's containers: %w", err)
		}
	}
	return o, nil
}

func (s *Service) Get(ctx context.Context, id objective.ObjectiveID) (objective.Objective, error) {
	return s.store.GetObjective(ctx, id)
}

func (s *Service) List(ctx context.Context, f storage.ObjectiveFilter) ([]objective.Objective, error) {
	return s.store.ListObjectives(ctx, f)
}

func (s *Service) UpdateStatus(ctx context.Context, id objective.ObjectiveID, status objective.ObjectiveStatus) error {
	return s.store.UpdateObjectiveStatus(ctx, id, status)
}

func (s *Service) ListTemplates() []objective.Template {
	out := make([]objective.Template, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, t)
	}
	return out
}

func newID() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
