package cost_test

import (
	"testing"

	"github.com/bsenel/karakuri/quota/cost"
)

func TestStaticPricer(t *testing.T) {
	p := cost.NewStaticPricer([]cost.Rate{
		{Provider: "anthropic", Model: "opus", UnitKind: cost.UnitTokens, PerUnit: 0.000015},
		{Provider: "anthropic", UnitKind: cost.UnitTokens, PerUnit: 0.000003},
		{Provider: "github", UnitKind: cost.UnitCalls, PerUnit: 0.01},
	})

	if got := p.Price("anthropic", "opus", cost.UnitTokens, 1_000_000); got != 15 {
		t.Errorf("opus = %v, want 15", got)
	}
	// A model nobody named falls back to the provider's rate. Providers ship
	// models faster than anybody updates a table, and the alternative is that a
	// new one silently costs nothing.
	if got := p.Price("anthropic", "brand-new", cost.UnitTokens, 1_000_000); got != 3 {
		t.Errorf("fallback = %v, want the provider rate", got)
	}
	// Case does not matter: a provider named "Anthropic" in a config file is
	// the same provider.
	if got := p.Price("Anthropic", "OPUS", cost.UnitTokens, 1_000_000); got != 15 {
		t.Errorf("case-insensitive lookup = %v, want 15", got)
	}
	if got := p.Price("github", "", cost.UnitCalls, 10); got != 0.1 {
		t.Errorf("calls = %v, want 0.1", got)
	}

	// Unknown provider, unknown unit, and no units at all all cost nothing
	// rather than erroring — the units are still recorded.
	for name, got := range map[string]float64{
		"unknown provider": p.Price("nobody", "m", cost.UnitTokens, 100),
		"unknown unit":     p.Price("anthropic", "opus", "widgets", 100),
		"no units":         p.Price("anthropic", "opus", cost.UnitTokens, 0),
		"negative units":   p.Price("anthropic", "opus", cost.UnitTokens, -5),
	} {
		if got != 0 {
			t.Errorf("%s = %v, want 0", name, got)
		}
	}

	if len(p.Rates()) != 3 {
		t.Errorf("Rates = %+v, want the three configured", p.Rates())
	}
}

// A deployment that configured no rates records units and invents no money.
func TestZeroPricers(t *testing.T) {
	if got := (cost.ZeroPricer{}).Price("anthropic", "opus", cost.UnitTokens, 1_000_000); got != 0 {
		t.Errorf("ZeroPricer = %v, want 0", got)
	}
	empty := cost.NewStaticPricer(nil)
	if got := empty.Price("anthropic", "opus", cost.UnitTokens, 1_000_000); got != 0 {
		t.Errorf("empty table = %v, want 0", got)
	}
	if got := empty.Rates(); len(got) != 0 {
		t.Errorf("Rates = %+v, want none", got)
	}

	// A nil pricer is usable, so a caller never has to nil-check.
	var nilPricer *cost.StaticPricer
	if got := nilPricer.Price("anthropic", "opus", cost.UnitTokens, 100); got != 0 {
		t.Errorf("nil pricer = %v, want 0", got)
	}
	if got := nilPricer.Rates(); got != nil {
		t.Errorf("nil pricer Rates = %+v, want nil", got)
	}
}
