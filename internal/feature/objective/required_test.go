package objective

import (
	"context"
	"errors"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

// Phase 41 Step 1: an objective with no required criterion can never
// complete, so it is refused when it is created rather than after it has spent
// every iteration it is given.
func TestCreateRejectsAnObjectiveWithNoRequiredCriterion(t *testing.T) {
	no := false
	svc := newTestService(t)
	svc.RegisterTemplate(coreobjective.Template{ID: "tmpl.empty"})
	svc.RegisterTemplate(coreobjective.Template{ID: "tmpl.all-optional", SuccessCriteria: []coreobjective.Criterion{
		{ID: "tidy", Description: "lint is clean", Required: &no},
		{ID: "fast", Description: "the bench holds", Required: &no},
	}})

	for name, req := range map[string]CreateRequest{
		"no template":           {Title: "plain", Domain: "software"},
		"unknown template":      {Title: "plain", Domain: "software", TemplateID: "nope"},
		"template with none":    {Title: "plain", Domain: "software", TemplateID: "tmpl.empty"},
		"template all optional": {Title: "plain", Domain: "software", TemplateID: "tmpl.all-optional"},
	} {
		if _, err := svc.Create(context.Background(), req); !errors.Is(err, ErrNoRequiredCriterion) {
			t.Errorf("%s: err = %v, want ErrNoRequiredCriterion", name, err)
		}
	}

	// Refused, not saved and then failed: nothing is left behind to be run.
	saved, err := svc.List(context.Background(), storage.ObjectiveFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(saved) != 0 {
		t.Errorf("%d rejected objectives were saved anyway", len(saved))
	}
}

// One required criterion is enough, whether it says so or says nothing.
func TestCreateAcceptsAnObjectiveWithARequiredCriterion(t *testing.T) {
	no, yes := false, true
	svc := newTestService(t)
	svc.RegisterTemplate(coreobjective.Template{ID: "tmpl.default", SuccessCriteria: []coreobjective.Criterion{
		{ID: "tested", Description: "the tests pass"},
	}})
	svc.RegisterTemplate(coreobjective.Template{ID: "tmpl.mixed", SuccessCriteria: []coreobjective.Criterion{
		{ID: "tidy", Description: "lint is clean", Required: &no},
		{ID: "tested", Description: "the tests pass", Required: &yes},
	}})

	for _, id := range []string{"tmpl.default", "tmpl.mixed"} {
		if _, err := svc.Create(context.Background(), CreateRequest{
			Title: "work", Domain: "software", TemplateID: id,
		}); err != nil {
			t.Errorf("%s: create: %v", id, err)
		}
	}
}
