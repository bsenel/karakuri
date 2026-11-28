package objective

import (
	"context"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// Phase 40 step 9: an objective built from a template takes the template's
// priority when the request sets none, and the owner's own priority wins when
// the request sets one.
func TestCreateTakesTheTemplatePriorityUnlessTheOwnerSetsOne(t *testing.T) {
	svc := newTestService(t)
	svc.RegisterTemplate(coreobjective.Template{
		ID:              "tmpl.ranked",
		Priority:        40,
		SuccessCriteria: []coreobjective.Criterion{{ID: "done", Description: "it is done"}},
	})

	for _, tc := range []struct {
		name      string
		requested int
		want      int
	}{
		{"none requested", 0, 40},
		{"owner's override, lower", 5, 5},
		{"owner's override, higher", 90, 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, err := svc.Create(context.Background(), CreateRequest{
				Title: "ranked", Domain: "software", TemplateID: "tmpl.ranked", Priority: tc.requested,
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if o.Priority != tc.want {
				t.Errorf("priority = %d, want %d", o.Priority, tc.want)
			}
			saved, err := svc.Get(context.Background(), o.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if saved.Priority != tc.want {
				t.Errorf("stored priority = %d, want %d", saved.Priority, tc.want)
			}
		})
	}
}
