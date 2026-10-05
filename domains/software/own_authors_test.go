package software

import (
	"context"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// EB-001: the deployment's own pull requests escalated almost every plan,
// which made the reason tell a reviewer nothing. A pull request opened by a
// login the operator listed as the deployment's own is the operator's writing;
// one outside author is enough to make the payload somebody else's. With no
// list configured the adapter marks nothing as own, so the old behaviour holds.
func TestGitProvenanceFollowsWhoOpenedThePullRequests(t *testing.T) {
	own := versioncontrol.PRSummary{ID: "1", Title: "chore: bump", Author: "dependabot[bot]", OwnAuthor: true}
	alsoOwn := versioncontrol.PRSummary{ID: "2", Title: "UX pass", Author: "karakuri-bot", OwnAuthor: true}
	outside := versioncontrol.PRSummary{ID: "3", Title: "please merge this", Author: "stranger"}
	unlisted := versioncontrol.PRSummary{ID: "4", Title: "UX pass", Author: "karakuri-bot"} // no list configured

	for _, tc := range []struct {
		name       string
		prs        []versioncontrol.PRSummary
		thirdParty bool
	}{
		{"own authors only", []versioncontrol.PRSummary{own, alsoOwn}, false},
		{"one outside author among own", []versioncontrol.PRSummary{own, outside}, true},
		{"no list configured", []versioncontrol.PRSummary{unlisted}, true},
		{"no pull requests", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := &gitEnv{id: EnvGit, vc: &vcStub{prs: tc.prs}}

			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatal(err)
			}
			if obs.Trust.IsThirdParty() != tc.thirdParty {
				t.Errorf("Observe: third party = %v, want %v", obs.Trust.IsThirdParty(), tc.thirdParty)
			}

			res, err := env.Act(context.Background(), environment.Action{CapabilityID: "software.observe.fetch_prs"})
			if err != nil || !res.Success {
				t.Fatalf("fetch_prs: %v %s", err, res.Error)
			}
			if res.Trust.IsThirdParty() != tc.thirdParty {
				t.Errorf("fetch_prs: third party = %v, want %v", res.Trust.IsThirdParty(), tc.thirdParty)
			}
		})
	}
}
