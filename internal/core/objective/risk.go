package objective

// RiskClass is how a pack author regards one of their own templates: how much
// it matters if work done under it goes wrong.
//
// These are the deployment's own words about a template, not a legal category.
// Whether a deployment is high-risk in any regulatory sense depends on who runs
// it, for whom and to what end, and is somebody else's determination; a value
// here neither makes that determination nor stands in for it.
type RiskClass string

const (
	// RiskUnclassified is the zero value: the pack author has not said how they regard this template.
	RiskUnclassified RiskClass = ""
	// RiskRoutine means the author regards the work as everyday and a mistake as cheap to notice and undo.
	RiskRoutine RiskClass = "routine"
	// RiskConsequential means the author regards a mistake as costly or slow to undo, though not harmful to a person.
	RiskConsequential RiskClass = "consequential"
	// RiskHigh means the author regards a mistake as able to harm a person's health, safety, rights or livelihood.
	RiskHigh RiskClass = "high"
)

// Valid reports whether r is one of the declared classes, the zero value included.
func (r RiskClass) Valid() bool {
	switch r {
	case RiskUnclassified, RiskRoutine, RiskConsequential, RiskHigh:
		return true
	}
	return false
}

// String returns the class as written, or "unclassified" for the zero value.
func (r RiskClass) String() string {
	if r == RiskUnclassified {
		return "unclassified"
	}
	return string(r)
}
