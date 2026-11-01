package software

import "errors"

// capFlakyTests counts the tests of params.packages that both passed and
// failed over params.runs runs of go test -json in params.workdir
// (docs/roadmap.md, Phase 41 step 10, flaky half).
const capFlakyTests = "software.verify.flaky_tests"

// flakyTest names one test go test ran: its package and its full name, a
// subtest's included ("TestT/case").
type flakyTest struct {
	pkg  string
	test string
}

// flakyTestsOf takes the go test -json event streams of N runs and returns the
// tests that passed in at least one run and failed in at least one other.
//
// Scaffolding only: the tests in flaky_tests_test.go specify it.
func flakyTestsOf(runs []string) ([]flakyTest, error) {
	return nil, errors.New("flakyTestsOf is not implemented (phase 41 step 10)")
}
