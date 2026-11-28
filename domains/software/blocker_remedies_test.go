package software

import "testing"

// Phase 40 step 14b: the pack declares, on each stream template, the remedy for
// the blocker kinds this phase already knows. A conflict is step 6a.
func TestEveryStreamTemplateDeclaresTheUnmergeablePRRemedy(t *testing.T) {
	for _, id := range []string{
		marketDiscoveryTemplateID, engineeringBacklogTemplateID,
		uxImprovementTemplateID, roadmapDeliveryTemplateID,
	} {
		tpl := streamTemplate(t, id)
		if tpl.BlockerRemedies["unmergeable_pr"] == "" {
			t.Errorf("%s declares no remedy for unmergeable_pr", id)
		}
	}
}
