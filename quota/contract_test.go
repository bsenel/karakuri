package quota_test

import (
	"testing"

	"github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/quotatest"
)

// The reference backend must pass the contract it defines. This also keeps the
// suite honest: a case that cannot pass against an obviously correct
// implementation is a bug in the case, not in the backend under test.
//
// It lives here rather than beside the suite so that the behaviour the suite
// exercises is attributed to this package's coverage — the contract is most of
// what proves MemoryBackend correct, and a gate that could not see it would be
// measuring the wrong thing.
func TestMemoryBackendSatisfiesContract(t *testing.T) {
	t.Parallel()
	quotatest.Run(t, func(*testing.T) quota.Backend { return quota.NewMemoryBackend() })
}
