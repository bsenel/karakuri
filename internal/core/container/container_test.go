package container_test

import (
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/container"
)

func TestNormalizeLabels(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"trims", []string{"  org:o_1  "}, []string{"org:o_1"}},
		{"drops empties", []string{"", "   ", "org:o_1"}, []string{"org:o_1"}},
		{
			"de-duplicates, keeping order",
			[]string{"team:t_1", "org:o_1", "team:t_1"},
			[]string{"team:t_1", "org:o_1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := container.NormalizeLabels(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("NormalizeLabels(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestKind(t *testing.T) {
	if container.Kind("squad").Valid() {
		t.Fatal("an unknown kind reported itself valid")
	}
	if !container.KindTeam.NeedsParent() {
		t.Fatal("a team with no organisation belongs to no tenant")
	}
	if container.KindOrg.NeedsParent() || container.KindProject.NeedsParent() {
		t.Fatal("orgs and projects must be creatable at the root")
	}
	// A project deliberately has no parent: giving it one would put it back
	// inside a single tenant, which is the opposite of what it is for.
	for _, kind := range []container.Kind{container.KindOrg, container.KindTeam, container.KindProject} {
		if container.KindProject.CanNestUnder(kind) {
			t.Fatalf("a project may not nest under a %s", kind)
		}
	}
}
