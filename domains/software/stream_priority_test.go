package software

import "testing"

// Phase 40 step 9: streams share the quota in a declared order. The templates
// declare it: delivery of approved work first, then review (step 11, not a
// template yet, so a gap is left for it), then UX, the engineering backlog and
// market discovery.
func TestStreamTemplatesDeclareTheirOrder(t *testing.T) {
	delivery := streamTemplate(t, roadmapDeliveryTemplateID).Priority
	ux := streamTemplate(t, uxImprovementTemplateID).Priority
	backlog := streamTemplate(t, engineeringBacklogTemplateID).Priority
	discovery := streamTemplate(t, marketDiscoveryTemplateID).Priority

	if discovery <= 0 {
		t.Errorf("market discovery priority = %d, want above 0: 0 reads as no priority declared", discovery)
	}
	if delivery <= ux || ux <= backlog || backlog <= discovery {
		t.Errorf("priorities delivery=%d ux=%d backlog=%d discovery=%d, want delivery > ux > backlog > discovery",
			delivery, ux, backlog, discovery)
	}
	// Room for the review stream (step 11) strictly between delivery and UX.
	if delivery-ux < 2 {
		t.Errorf("delivery=%d and ux=%d leave no priority between them for the review stream", delivery, ux)
	}
}
