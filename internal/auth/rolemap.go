package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bsenel/karakuri/auth"
	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/container"
)

// ContainerLookup is the slice of the container service the role map needs:
// enough to turn a name an operator wrote into the ID a binding carries.
type ContainerLookup interface {
	List(ctx context.Context, f container.Filter) ([]container.Container, error)
	Closure(ctx context.Context, id string) ([]string, error)
}

// BuildRoleMap resolves the configured group mapping into scoped role grants.
//
// Names become IDs here and nowhere else. A binding must never carry a display
// name — two organisations may each have a team called "Engineering", and a
// grant that matched on the name would cover both — so the one place a name is
// permitted is configuration written by a human, and it is resolved once, at
// boot, where a typo is something an operator is reading output about rather
// than something a user discovers by not being able to see their twins.
func BuildRoleMap(ctx context.Context, cfg config.AuthRoleMapConfig, containers ContainerLookup) (auth.RoleMap, error) {
	out := auth.RoleMap{}
	for group, grants := range cfg.Groups {
		resolved, err := resolveGrants(ctx, grants, containers)
		if err != nil {
			return auth.RoleMap{}, fmt.Errorf("group %q: %w", group, err)
		}
		if out.Groups == nil {
			out.Groups = map[string][]auth.RoleGrant{}
		}
		out.Groups[group] = resolved
	}
	resolved, err := resolveGrants(ctx, cfg.Default, containers)
	if err != nil {
		return auth.RoleMap{}, fmt.Errorf("default: %w", err)
	}
	out.Default = resolved
	return out, nil
}

func resolveGrants(ctx context.Context, grants []config.AuthRoleGrantConfig, containers ContainerLookup) ([]auth.RoleGrant, error) {
	if len(grants) == 0 {
		return nil, nil
	}
	out := make([]auth.RoleGrant, 0, len(grants))
	for _, g := range grants {
		role := strings.TrimSpace(g.Role)
		if role == "" {
			return nil, fmt.Errorf("a grant has no role")
		}
		scope, err := resolveScope(ctx, g, containers)
		if err != nil {
			return nil, fmt.Errorf("role %q: %w", role, err)
		}
		out = append(out, auth.RoleGrant{Role: role, Scope: scope})
	}
	return out, nil
}

// resolveScope turns the container named on a grant into its label, or "*" when
// the grant names none.
func resolveScope(ctx context.Context, g config.AuthRoleGrantConfig, containers ContainerLookup) (string, error) {
	org := strings.TrimSpace(g.Org)
	team := strings.TrimSpace(g.Team)
	project := strings.TrimSpace(g.Project)

	switch {
	case org == "" && team == "" && project == "":
		// The bare form, and what every mapping meant before Phase 17.
		return "*", nil
	case project != "" && (org != "" || team != ""):
		// A project spans organisations by design. Qualifying one with an org
		// would be describing something that does not exist, and quietly
		// ignoring half the grant is worse than refusing it.
		return "", fmt.Errorf("a project grant cannot also name an org or a team")
	case team != "" && org == "":
		return "", fmt.Errorf("team %q needs its org: two organisations may each have a team of that name", team)
	}

	if containers == nil {
		return "", fmt.Errorf("no container tree is configured, so %q cannot be resolved", firstNonEmpty(project, team, org))
	}

	if project != "" {
		c, err := findUnique(ctx, containers, container.KindProject, project, "")
		if err != nil {
			return "", err
		}
		return c.Label(), nil
	}

	orgContainer, err := findUnique(ctx, containers, container.KindOrg, org, "")
	if err != nil {
		return "", err
	}
	if team == "" {
		return orgContainer.Label(), nil
	}

	// Teams may nest, so the team is looked up by name anywhere and then
	// filtered to the ones actually inside this org. That is what makes
	// "acme/eng" and "globex/eng" resolve to different labels rather than
	// colliding.
	return findTeam(ctx, containers, org, orgContainer, team)
}

func findTeam(ctx context.Context, containers ContainerLookup, orgName string, org container.Container, team string) (string, error) {
	candidates, err := containers.List(ctx, container.Filter{Kind: container.KindTeam, Name: team})
	if err != nil {
		return "", fmt.Errorf("look up team %q: %w", team, err)
	}
	var matches []container.Container
	for _, c := range candidates {
		closure, err := containers.Closure(ctx, c.ID)
		if err != nil {
			return "", fmt.Errorf("resolve team %q: %w", team, err)
		}
		if slices.Contains(closure, org.Label()) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no team %q in org %q", team, orgName)
	case 1:
		return matches[0].Label(), nil
	default:
		// Legal in the tree — two sub-teams of different parents may share a
		// name — but not addressable from configuration, so say so rather than
		// picking one.
		return "", fmt.Errorf("org %q has %d teams called %q; rename one or grant the org", orgName, len(matches), team)
	}
}

func findUnique(ctx context.Context, containers ContainerLookup, kind container.Kind, name, parentID string) (container.Container, error) {
	found, err := containers.List(ctx, container.Filter{Kind: kind, Name: name, ParentID: parentID})
	if err != nil {
		return container.Container{}, fmt.Errorf("look up %s %q: %w", kind, name, err)
	}
	switch len(found) {
	case 0:
		return container.Container{}, fmt.Errorf("no %s called %q", kind, name)
	case 1:
		return found[0], nil
	default:
		return container.Container{}, fmt.Errorf("%d %ss are called %q; names are unique per parent, so this one is ambiguous", len(found), kind, name)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
