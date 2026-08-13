package cost

import (
	"maps"
	"strings"
)

// Pricer turns consumption into money.
//
// It is an interface because pricing is where deployments differ most: a public
// rate card, a negotiated contract, a gateway that already knows. What they
// share is the shape — units in, currency out — and nothing else.
type Pricer interface {
	// Price returns what units of unitKind from provider/model cost, in whole
	// currency units. An unknown combination returns zero rather than an error:
	// a model nobody priced is still worth recording the *units* for, and a
	// ledger that refused the event would lose them.
	Price(provider, model, unitKind string, units float64) float64
}

// Rate is the price of one unit.
type Rate struct {
	Provider string
	Model    string
	UnitKind string

	// PerUnit is the cost of a single unit. Model prices are usually quoted per
	// million tokens, so this is a small number — deliberately, because doing
	// the division once at configuration time beats doing it per event and
	// wondering which of the two places rounded.
	PerUnit float64
}

// StaticPricer prices from a fixed table.
//
// It takes a Go map rather than reading a file, which is the deviation from
// this phase's plan worth stating: a price table is configuration, and parsing
// configuration is the host's job. Reading YAML here would put a dependency in
// a module whose whole point is not having any — the same reason the auth
// module implements JWT over crypto/hmac rather than taking a library.
//
// Zero value prices everything at zero, which is what a deployment that has not
// configured rates should see: units recorded, money reported as unknown rather
// than as invented.
type StaticPricer struct {
	rates map[rateKey]float64
}

type rateKey struct{ provider, model, unitKind string }

var _ Pricer = (*StaticPricer)(nil)

// NewStaticPricer builds a pricer from a rate table.
//
// A Rate with an empty Model is the provider's fallback, used when no rate
// names the model exactly. That is what makes a table usable: providers ship
// models faster than anybody updates a config file, and the alternative to a
// fallback is that a new model silently costs nothing.
func NewStaticPricer(rates []Rate) *StaticPricer {
	p := &StaticPricer{rates: make(map[rateKey]float64, len(rates))}
	for _, r := range rates {
		p.rates[rateKey{
			provider: strings.ToLower(r.Provider),
			model:    strings.ToLower(r.Model),
			unitKind: r.UnitKind,
		}] = r.PerUnit
	}
	return p
}

// Price looks up the exact model, then the provider's fallback, then gives up
// and returns zero.
func (p *StaticPricer) Price(provider, model, unitKind string, units float64) float64 {
	if p == nil || units <= 0 {
		return 0
	}
	provider, model = strings.ToLower(provider), strings.ToLower(model)

	if rate, ok := p.rates[rateKey{provider, model, unitKind}]; ok {
		return rate * units
	}
	if rate, ok := p.rates[rateKey{provider: provider, unitKind: unitKind}]; ok {
		return rate * units
	}
	return 0
}

// Rates returns the table, for an operator asking what this process thinks
// things cost.
func (p *StaticPricer) Rates() []Rate {
	if p == nil {
		return nil
	}
	out := make([]Rate, 0, len(p.rates))
	for k, v := range maps.All(p.rates) {
		out = append(out, Rate{Provider: k.provider, Model: k.model, UnitKind: k.unitKind, PerUnit: v})
	}
	return out
}

// ZeroPricer records units and no money. It is what a deployment with no rate
// table gets, so a caller never has to nil-check a Pricer.
type ZeroPricer struct{}

var _ Pricer = ZeroPricer{}

func (ZeroPricer) Price(string, string, string, float64) float64 { return 0 }
