package testing

import "context"

type TestResult struct {
	Name    string
	Passed  bool
	Output  string
}

type TestingAdapter interface {
	Name() string
	RunTests(ctx context.Context, path string) ([]TestResult, error)
	Active() bool
}
