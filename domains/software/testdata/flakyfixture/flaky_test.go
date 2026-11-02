// Package flakyfixture is the module the flaky-test verifier's test runs over
// and over: one test fails exactly one run in three, the other always passes.
//
// The flaky test is deterministic. It counts its runs in the file named by
// KARAKURI_FLAKYFIXTURE_COUNTER, which the outer test sets with t.Setenv to a
// file in its own t.TempDir(), and fails on the second of every three.
package flakyfixture

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
)

const counterEnv = "KARAKURI_FLAKYFIXTURE_COUNTER"

func TestFailsOneRunInThree(t *testing.T) {
	path := os.Getenv(counterEnv)
	if path == "" {
		t.Fatalf("%s is not set: the outer test names the counter file", counterEnv)
	}
	n := 0
	b, err := os.ReadFile(path) // #nosec G304 -- the path is a file in the outer test's own t.TempDir()
	switch {
	case err == nil:
		n, err = strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			t.Fatalf("counter file %s holds %q, not a run count", path, b)
		}
	case !errors.Is(err, fs.ErrNotExist):
		t.Fatalf("read counter file: %v", err)
	}
	n++
	if err := os.WriteFile(path, []byte(strconv.Itoa(n)), 0o600); err != nil {
		t.Fatalf("write counter file: %v", err)
	}
	if n%3 == 2 {
		t.Fatalf("run %d: the second of every three runs fails", n)
	}
}

func TestAlwaysPasses(t *testing.T) {}
