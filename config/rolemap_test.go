package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Both forms have to parse, because the bare one is what every configuration
// file written before Phase 17 uses and upgrading must not require editing it.
func TestAuthRoleGrantConfigAcceptsBothForms(t *testing.T) {
	const doc = `
groups:
  karakuri-admins: [admin]
  acme-engineers:
    - {role: operator, org: acme, team: eng}
    - viewer
  delta-collaborators:
    - {role: contributor, project: delta}
default: [viewer]
`
	var got AuthRoleMapConfig
	if err := yaml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The bare form means the role over everything, exactly as it did before.
	if want := (AuthRoleGrantConfig{Role: "admin"}); got.Groups["karakuri-admins"][0] != want {
		t.Errorf("bare form = %+v, want %+v", got.Groups["karakuri-admins"][0], want)
	}
	if want := (AuthRoleGrantConfig{Role: "viewer"}); got.Default[0] != want {
		t.Errorf("default = %+v, want %+v", got.Default[0], want)
	}

	acme := got.Groups["acme-engineers"]
	if len(acme) != 2 {
		t.Fatalf("acme-engineers = %+v, want two grants", acme)
	}
	if want := (AuthRoleGrantConfig{Role: "operator", Org: "acme", Team: "eng"}); acme[0] != want {
		t.Errorf("mapping form = %+v, want %+v", acme[0], want)
	}
	// The two forms mix inside one list.
	if want := (AuthRoleGrantConfig{Role: "viewer"}); acme[1] != want {
		t.Errorf("mixed list = %+v, want %+v", acme[1], want)
	}

	if want := (AuthRoleGrantConfig{Role: "contributor", Project: "delta"}); got.Groups["delta-collaborators"][0] != want {
		t.Errorf("project form = %+v, want %+v", got.Groups["delta-collaborators"][0], want)
	}
}

func TestAuthRoleGrantConfigRejectsGarbage(t *testing.T) {
	var got AuthRoleMapConfig
	if err := yaml.Unmarshal([]byte("groups:\n  eng: [[a, b]]\n"), &got); err == nil {
		t.Fatal("a sequence parsed as a grant")
	}
}
