package software

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 3: the git observation says whether main is protected, read
// through the version-control adapter, so the oversight view can show it. A
// read that failed is "not known", never "protected" (AGENTS.md rule 10).
func TestGitObservationReportsMainsProtection(t *testing.T) {
	cases := []struct {
		name      string
		stub      *vcStub
		want      string
		wantError bool
	}{
		{name: "protected", stub: &vcStub{protected: true}, want: "protected"},
		{name: "not protected", stub: &vcStub{protected: false}, want: "not protected"},
		{
			name:      "forge cannot say",
			stub:      &vcStub{protectedErr: fmt.Errorf("stub: branch protection: %w", versioncontrol.ErrUnsupported)},
			want:      "not known",
			wantError: true,
		},
		{
			name:      "read failed",
			stub:      &vcStub{protectedErr: errors.New("github: GET /repos/acme/api/branches/main -> 500: oops")},
			want:      "not known",
			wantError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := &gitEnv{id: EnvGit, vc: tc.stub}

			obs, err := env.Observe(t.Context(), environment.ObservationQuery{Filter: map[string]any{"repo": "acme/api"}})
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if want := []string{"acme/api:main"}; !slices.Equal(tc.stub.protectionAsks, want) {
				t.Errorf("BranchProtected asked %v, want %v", tc.stub.protectionAsks, want)
			}
			got, _ := obs.State["main_protection"].(string)
			if got != tc.want {
				t.Errorf("main_protection = %#v, want %q (state %v)", obs.State["main_protection"], tc.want, obs.State)
			}
			reason, _ := obs.State["main_protection_error"].(string)
			if tc.wantError && reason == "" {
				t.Errorf("main_protection_error = %#v, want the reason the read failed", obs.State["main_protection_error"])
			}
			if !tc.wantError {
				if v, ok := obs.State["main_protection_error"]; ok {
					t.Errorf("main_protection_error = %v, want none when the read succeeded", v)
				}
			}
		})
	}
}

// The stub answers true alongside its error. Whatever the bool says, a failed
// read is never reported as protection the deployment does not know it has.
func TestGitObservationNeverReportsProtectedWhenTheReadFailed(t *testing.T) {
	for _, readErr := range []error{
		versioncontrol.ErrUnsupported,
		errors.New("github: GET /repos/acme/api/branches/main -> 404: Branch not found"),
	} {
		env := &gitEnv{id: EnvGit, vc: &vcStub{protected: true, protectedErr: readErr}}

		obs, err := env.Observe(t.Context(), environment.ObservationQuery{})
		if err != nil {
			t.Fatalf("observe: %v", err)
		}
		if got := obs.State["main_protection"]; got == "protected" {
			t.Errorf("main_protection = %q after the read failed with %v", got, readErr)
		}
	}
}
