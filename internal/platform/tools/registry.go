package tools

import (
	"sync"

	"github.com/bsenel/karakuri/internal/platform/tools/design"
	"github.com/bsenel/karakuri/internal/platform/tools/messaging"
	"github.com/bsenel/karakuri/internal/platform/tools/observability"
	"github.com/bsenel/karakuri/internal/platform/tools/projectmgmt"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
	"github.com/bsenel/karakuri/internal/platform/tools/testing"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

type Registry struct {
	VC             versioncontrol.VersionControlAdapter
	ProjectMgmt    projectmgmt.ProjectManagementAdapter
	Design         design.DesignAdapter
	Testing        testing.TestingAdapter
	Messaging      messaging.MessagingAdapter
	Observability  observability.ObservabilityAdapter
	Research       research.ResearchAdapter
	mu             sync.RWMutex
	summary        map[string]bool
}

func NewRegistry() *Registry {
	r := &Registry{
		VC:            versioncontrol.NewNoOp(),
		ProjectMgmt:   projectmgmt.NewNoOp(),
		Design:        design.NewNoOp(),
		Testing:       testing.NewNoOp(),
		Messaging:     messaging.NewNoOp(),
		Observability: observability.NewNoOp(),
		Research:      research.NewHTTPScraper(),
		summary:       make(map[string]bool),
	}
	r.summary["versioncontrol"] = r.VC.Active()
	r.summary["projectmgmt"] = r.ProjectMgmt.Active()
	r.summary["design"] = r.Design.Active()
	r.summary["testing"] = r.Testing.Active()
	r.summary["messaging"] = r.Messaging.Active()
	r.summary["observability"] = r.Observability.Active()
	r.summary["research"] = r.Research.Active()
	return r
}

func (r *Registry) Summary() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]bool)
	for k, v := range r.summary {
		out[k] = v
	}
	return out
}
