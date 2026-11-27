package software

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13: the delivery work reading is the git observation's
// unanswered-comment policy (step 12a). A comment after the last commit from
// an account Karakuri and the owner share is the owner's unless Karakuri
// recorded posting it; counting Karakuri's own would start a pass that answers
// itself.

type workForge struct {
	*commentForge
	roadmap string
}

func (f *workForge) FileAt(context.Context, string, string, string) (string, error) {
	return f.roadmap, nil
}

func buildRegisteredDeliveryWork(t *testing.T, s storage.StorageAdapter, forge versioncontrol.VersionControlAdapter) environment.Environment {
	t.Helper()
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", forge)
	pack := New().WithOwnAuthors(postedAuthors)
	pack.tools = reg
	pack.WithStorage(s)
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != "software.env.delivery_work" {
			continue
		}
		env, err := fac.Build(environment.BuildContext{AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}
	t.Fatal("no delivery_work factory")
	return nil
}

func TestRegisteredDeliveryWorkIgnoresCommentsKarakuriPosted(t *testing.T) {
	const commentID = 55
	cases := map[string]struct {
		record   bool
		wantWork bool
	}{
		"Karakuri recorded posting it":      {record: true, wantWork: false},
		"no record of it, so it is owner's": {record: false, wantWork: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			forge := &workForge{
				commentForge: rcForge(rcBranch, rcComment(commentID, "shared-acct", rcLast.Add(time.Hour))),
				roadmap:      roadmapNothingPlanned,
			}
			s := newPostedStore(t)
			if tc.record {
				saveActionEvent(t, s, "software.act.write_code", map[string]any{postedKey: []int64{commentID}})
			}
			env := buildRegisteredDeliveryWork(t, s, forge)
			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if got, _ := obs.State["has_work"].(bool); got != tc.wantWork {
				t.Errorf("has_work = %v, want %v (reasons %v)", got, tc.wantWork, obs.State["reasons"])
			}
		})
	}
}
