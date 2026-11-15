// Package failfixture is the module the run_tests output test runs: 60
// passing tests that each write a line, then one test that fails. In go test
// -json every passing test writes run, output and pass events, so the failure
// comes long after the first 4000 bytes of stdout.
package failfixture

import (
	"fmt"
	"testing"
)

// FailureMessage is what TestFailsLast fails with; the outer test looks for it.
const FailureMessage = "karakuri-failfixture: the widget count is 3, want 4"

func TestPassesSixtyTimes(t *testing.T) {
	for i := 1; i <= 60; i++ {
		t.Run(fmt.Sprintf("case_%02d", i), func(t *testing.T) {
			t.Logf("case %02d checked its widget and it was fine", i)
		})
	}
}

// TestFailsLast runs after the passing tests, since go test runs top-level
// tests in source order.
func TestFailsLast(t *testing.T) {
	t.Fatal(FailureMessage)
}
