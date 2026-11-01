package coverfixture

import "testing"

// Only the non-negative branch runs: two of Sign's four statements.
func TestSignOfAPositive(t *testing.T) {
	if got := Sign(3); got != 1 {
		t.Fatalf("Sign(3) = %d, want 1", got)
	}
}
