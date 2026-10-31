package environment

import "testing"

// A verifier's number travels typed on its result, with the trust of
// whoever wrote it, so nothing downstream scrapes it out of StateDelta or
// prose (docs/roadmap.md, Phase 41 decision 2, 'Measurements').
func TestActionResultCarriesTypedMeasurements(t *testing.T) {
	result := ActionResult{
		Success: true,
		Measurements: []Measurement{
			{Name: "share", Value: 0.81, Unit: "ratio", Source: "pack.verify.share"},
			{Name: "share", Value: 0.95, Unit: "ratio", Source: "a comment on the pull request", Trust: TrustThirdParty},
		},
	}

	if len(result.Measurements) != 2 {
		t.Fatalf("measurements = %+v", result.Measurements)
	}
	own, read := result.Measurements[0], result.Measurements[1]
	if own.Name != "share" || own.Value != 0.81 || own.Unit != "ratio" || own.Source != "pack.verify.share" {
		t.Errorf("measurement = %+v", own)
	}
	// A value the deployment's own verifier computed is the zero, trusted
	// value; a number read out of somebody else's text says so.
	if own.Trust.IsThirdParty() {
		t.Error("a verifier's own measurement reads as third-party")
	}
	if !read.Trust.IsThirdParty() {
		t.Error("a figure read from somebody else's text does not read as third-party")
	}
}
