package objective

import (
	"testing"

	"github.com/bsenel/karakuri/internal/core/capability"
)

func TestObjective_AllDomains_Dedup(t *testing.T) {
	o := Objective{Domain: "software", AdditionalDomains: []string{"healthcare", "software", "", "legal"}}
	got := o.AllDomains()
	want := []string{"software", "healthcare", "legal"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestObjective_AllDomains_EmptyPrimary(t *testing.T) {
	// Primary domain empty + additional set: union should skip the empty primary.
	o := Objective{Domain: "", AdditionalDomains: []string{"healthcare"}}
	got := o.AllDomains()
	if len(got) != 1 || got[0] != "healthcare" {
		t.Errorf("expected [healthcare], got %v", got)
	}
}

func TestObjective_AllDomains_OnlyPrimary(t *testing.T) {
	o := Objective{Domain: "software"}
	got := o.AllDomains()
	if len(got) != 1 || got[0] != "software" {
		t.Errorf("expected [software], got %v", got)
	}
}

func TestObjective_CriterionDomains(t *testing.T) {
	o := Objective{
		Domain: "software", AdditionalDomains: []string{"healthcare"},
		SuccessCriteria: []Criterion{
			{ID: "c1", Domain: "software"},
			{ID: "c2", Domain: "healthcare"},
			{ID: "c3"},                     // no domain — should not appear
			{ID: "c4", Domain: "software"}, // duplicate
		},
	}
	got := o.CriterionDomains()
	if len(got) != 2 {
		t.Fatalf("expected 2 unique criterion domains, got %v", got)
	}
	seen := map[string]bool{}
	for _, d := range got {
		seen[d] = true
	}
	if !seen["software"] || !seen["healthcare"] {
		t.Errorf("expected software+healthcare, got %v", got)
	}
}

// The second bound: a criterion verified by a discovered tool is refused
// however it was written, and only the reserved namespace is.
func TestCriterion_VerifierIsReserved(t *testing.T) {
	for _, tc := range []struct {
		verifier string
		want     bool
	}{
		{"mcp.acme_files.read_file", true},
		{"software.act.run_tests", false},
		{"mcpx.files.read", false},
		{"", false},
	} {
		c := Criterion{Verifier: capability.CapabilityID(tc.verifier)}
		if got := c.VerifierIsReserved(); got != tc.want {
			t.Errorf("VerifierIsReserved(%q) = %v, want %v", tc.verifier, got, tc.want)
		}
	}
}
