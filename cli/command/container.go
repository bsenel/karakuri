package command

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// The tenancy tree on the command line.
//
// Every command here takes names and prints names, and never lets one reach a
// policy: names are resolved to IDs before anything is sent, because two
// organisations may each have a team called "eng" and a grant that matched on
// the word would cover both. The ID is what a binding stores, so renaming an
// organisation changes nothing about who can see what.

func orgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage organisations",
	}
	cmd.AddCommand(
		containerCreateCmd("org", "Create an organisation", false),
		containerListCmd("org", "List organisations"),
		containerRenameCmd("org", "Rename an organisation"),
		containerDeleteCmd("org", "Delete an empty organisation"),
	)
	return cmd
}

func teamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Manage teams",
	}
	cmd.AddCommand(
		containerCreateCmd("team", "Create a team inside an organisation", true),
		containerListCmd("team", "List teams"),
		containerRenameCmd("team", "Rename a team"),
		containerDeleteCmd("team", "Delete an empty team"),
		teamMoveCmd(),
	)
	return cmd
}

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects — collaboration spaces that can span organisations",
	}
	cmd.AddCommand(
		containerCreateCmd("project", "Create a project", false),
		containerListCmd("project", "List projects"),
		containerRenameCmd("project", "Rename a project"),
		containerDeleteCmd("project", "Delete an empty project"),
		projectAddCmd(),
		projectRemoveCmd(),
	)
	return cmd
}

func containerCreateCmd(kind, short string, needsOrg bool) *cobra.Command {
	var name, org string
	cmd := &cobra.Command{
		Use:   "create",
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			body := map[string]any{"kind": kind, "name": name}
			if needsOrg {
				if org == "" {
					return fmt.Errorf("--org is required: two organisations may each have a %s of the same name", kind)
				}
				parent, err := resolveContainer("org", org, "")
				if err != nil {
					return err
				}
				body["parent_id"] = parent
			} else if org != "" {
				parent, err := resolveContainer("org", org, "")
				if err != nil {
					return err
				}
				body["parent_id"] = parent
			}
			data, _, err := api.Post("/containers", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	_ = cmd.MarkFlagRequired("name")
	switch {
	case needsOrg:
		cmd.Flags().StringVar(&org, "org", "", "Organisation this belongs to")
	case kind == "org":
		cmd.Flags().StringVar(&org, "org", "", "Parent organisation, for a subsidiary")
	}
	return cmd
}

func containerListCmd(kind, short string) *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "list",
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			q := url.Values{"kind": {kind}}
			if org != "" {
				parent, err := resolveContainer("org", org, "")
				if err != nil {
					return err
				}
				q.Set("parent_id", parent)
			}
			data, _, err := api.Get("/containers?" + q.Encode())
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	if kind == "team" {
		cmd.Flags().StringVar(&org, "org", "", "Only teams directly inside this organisation")
	}
	return cmd
}

func containerRenameCmd(kind, short string) *cobra.Command {
	var org, name string
	cmd := &cobra.Command{
		Use:   "rename <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := resolveIn(kind, args[0], org)
			if err != nil {
				return err
			}
			data, _, err := api.Post("/containers/"+id+"/name", map[string]any{"name": name})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "to", "", "New display name")
	_ = cmd.MarkFlagRequired("to")
	if kind == "team" {
		cmd.Flags().StringVar(&org, "org", "", "Organisation the team is in")
	}
	return cmd
}

func containerDeleteCmd(kind, short string) *cobra.Command {
	var org string
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := resolveIn(kind, args[0], org)
			if err != nil {
				return err
			}
			// A container with children is refused by the server rather than
			// cascaded — deleting a tree would revoke access to everything
			// under it in one call.
			if _, _, err := api.Delete("/containers/" + id); err != nil {
				return err
			}
			client.PrintOutput([]byte(fmt.Sprintf("{%q:%q}", "deleted", id)), output)
			return nil
		},
	}
	if kind == "team" {
		cmd.Flags().StringVar(&org, "org", "", "Organisation the team is in")
	}
	return cmd
}

// teamMoveCmd reparents a team, which the server allows only to somebody
// holding both the old and the new organisation — otherwise moving a team would
// be a way to walk resources out of a tenant you hold into one you do not.
func teamMoveCmd() *cobra.Command {
	var fromOrg, toOrg string
	cmd := &cobra.Command{
		Use:   "move <name>",
		Short: "Move a team to another organisation",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := resolveIn("team", args[0], fromOrg)
			if err != nil {
				return err
			}
			parent, err := resolveContainer("org", toOrg, "")
			if err != nil {
				return err
			}
			data, _, err := api.Post("/containers/"+id+"/parent", map[string]any{"parent_id": parent})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&fromOrg, "org", "", "Organisation the team is currently in")
	cmd.Flags().StringVar(&toOrg, "to-org", "", "Organisation to move it to")
	_ = cmd.MarkFlagRequired("to-org")
	return cmd
}

// projectAddCmd puts a resource in a project, which is how sharing across
// organisations works: the resource keeps its own team and org and gains the
// project as one more label, so both tenants reach it without either losing
// anything.
func projectAddCmd() *cobra.Command {
	var project, twin string
	cmd := &cobra.Command{
		Use:   "add-twin",
		Short: "Share a twin into a project",
		RunE: func(_ *cobra.Command, _ []string) error {
			return setTwinContainers(project, twin, true)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project to share into")
	cmd.Flags().StringVar(&twin, "twin", "", "Twin id")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

func projectRemoveCmd() *cobra.Command {
	var project, twin string
	cmd := &cobra.Command{
		Use:   "remove-twin",
		Short: "Stop sharing a twin into a project",
		RunE: func(_ *cobra.Command, _ []string) error {
			return setTwinContainers(project, twin, false)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "Project to remove it from")
	cmd.Flags().StringVar(&twin, "twin", "", "Twin id")
	_ = cmd.MarkFlagRequired("project")
	_ = cmd.MarkFlagRequired("twin")
	return cmd
}

// setTwinContainers adds or removes one container from a twin's membership.
//
// The API replaces the whole set rather than patching it, because a closure has
// to be recomputed either way — so the current membership is read first and the
// change applied to it here.
func setTwinContainers(project, twin string, add bool) error {
	projectID, err := resolveContainer("project", project, "")
	if err != nil {
		return err
	}
	current, err := twinContainerIDs(twin)
	if err != nil {
		return err
	}

	next := make([]string, 0, len(current)+1)
	for _, id := range current {
		if id != projectID {
			next = append(next, id)
		}
	}
	if add {
		next = append(next, projectID)
	}

	data, _, err := api.Post("/containers/resources", map[string]any{
		"resource_type": "twin",
		"resource_id":   twin,
		"container_ids": next,
	})
	if err != nil {
		return err
	}
	client.PrintOutput(data, output)
	return nil
}

// twinContainerIDs reads the containers a twin was placed in.
//
// The API returns the flattened closure, so an inherited org label comes back
// alongside the team that was actually declared. Sending the closure back would
// pin the twin to an organisation it only reached through its team, and it
// would stop following that team on a move — so only labels naming a container
// the caller can address are kept, and the server recomputes the rest.
func twinContainerIDs(twin string) ([]string, error) {
	q := url.Values{"resource_type": {"twin"}, "resource_id": {twin}}
	data, _, err := api.Get("/containers/resources?" + q.Encode())
	if err != nil {
		return nil, err
	}
	var body struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("read twin containers: %w", err)
	}
	ids := make([]string, 0, len(body.Scopes))
	for _, label := range body.Scopes {
		if _, id, ok := strings.Cut(label, ":"); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// resolveIn resolves a container by name, optionally inside a named
// organisation.
func resolveIn(kind, name, org string) (string, error) {
	parent := ""
	if org != "" {
		id, err := resolveContainer("org", org, "")
		if err != nil {
			return "", err
		}
		parent = id
	}
	return resolveContainer(kind, name, parent)
}

// resolveContainer turns a display name into the ID a policy carries.
//
// An ambiguous name is an error rather than a guess. Names are unique among
// siblings and nowhere else, so "the team called eng" is only a question with
// one answer once you say which organisation — and picking one silently is how
// a grant lands in the wrong tenant.
func resolveContainer(kind, name, parentID string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("no %s name given", kind)
	}
	q := url.Values{"kind": {kind}, "name": {name}}
	if parentID != "" {
		q.Set("parent_id", parentID)
	}
	data, _, err := api.Get("/containers?" + q.Encode())
	if err != nil {
		return "", err
	}
	var found []struct {
		ID       string `json:"id"`
		ParentID string `json:"parent_id"`
	}
	if err := json.Unmarshal(data, &found); err != nil {
		return "", fmt.Errorf("read %s list: %w", kind, err)
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no %s called %q", kind, name)
	case 1:
		return found[0].ID, nil
	default:
		hint := ""
		if kind == "team" {
			hint = " — name the organisation with --org"
		}
		return "", fmt.Errorf("%d %ss are called %q%s", len(found), kind, name, hint)
	}
}

// containerScope turns the container flags on `krk auth bindings add` into the
// scope a binding stores, or "" when none were given.
//
// A team always needs its organisation. That is not a convenience check: two
// organisations may each have a team called "eng", so "the team called eng" has
// no single answer, and resolving it by guessing is how a grant lands in the
// wrong tenant.
func containerScope(org, team, project string) (string, error) {
	switch {
	case org == "" && team == "" && project == "":
		return "", nil
	case project != "" && (org != "" || team != ""):
		return "", fmt.Errorf("a project spans organisations, so it cannot be qualified by --org or --team")
	case team != "" && org == "":
		return "", fmt.Errorf("--team needs --org: two organisations may each have a team called %q", team)
	}

	if project != "" {
		id, err := resolveContainer("project", project, "")
		return "project:" + id, err
	}
	orgID, err := resolveContainer("org", org, "")
	if err != nil {
		return "", err
	}
	if team == "" {
		return "org:" + orgID, nil
	}
	teamID, err := resolveContainer("team", team, orgID)
	if err != nil {
		return "", err
	}
	return "team:" + teamID, nil
}
