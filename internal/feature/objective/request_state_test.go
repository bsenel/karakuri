package objective

import (
	"encoding/json"
	"strings"
	"testing"

	coreobjective "github.com/bsenel/karakuri/internal/core/objective"
)

// A request cannot create a criterion that reads as already settled: verify
// settles every criterion on each pass, and until the first one the API and
// the web panel would otherwise show whatever state the request sent.
func TestCreateDropsARequestState(t *testing.T) {
	c := relativeLatency(func(c *coreobjective.Criterion) {
		c.State = coreobjective.CriterionMet
		c.Reason = coreobjective.ReasonNoData
	})
	stored := createOne(t, c)
	if stored.State != "" || stored.Reason != "" {
		t.Errorf("stored state = %q, reason = %q; want both empty until verify settles them", stored.State, stored.Reason)
	}
	out, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"met":false`) {
		t.Errorf("the stored criterion reads as met: %s", out)
	}
}
