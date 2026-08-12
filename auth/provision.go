package auth

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ManagedBindingPrefix marks the role bindings a Provisioner owns.
//
// Reconciliation only ever touches bindings carrying it, so a grant an
// administrator made by hand survives the next login. Principal IDs starting
// with it are reserved for the same reason.
const ManagedBindingPrefix = "idp:"

// Provisioner turns an ExternalIdentity into a local principal.
//
// This is the load-bearing decision of federated identity here. Authorization
// resolves permissions from role bindings held in the Store (see
// StoreAuthorizer), so an identity that exists only as a set of claims holds no
// bindings and is denied everything. Rather than teach the authorizer a second
// source of truth, an identity provider's users are provisioned into the store
// on the way in — after which a federated principal is an ordinary principal,
// and ownership conditions, audit records and the permission surface all work
// on it unchanged.
//
// The store write is the price. It is paid once per login and only when
// something actually differs, which is why Provision compares before it writes.
type Provisioner struct {
	// Store is where principals and bindings live.
	Store Store

	// Roles maps asserted groups onto role names.
	Roles RoleMap

	// Prefix namespaces principal IDs — "oidc", "saml". Required.
	Prefix string

	// OnChange, when set, is called after a login changed anything: the
	// principal was created, or its managed roles differ from last time. It is
	// how a host application audits "who granted this person admin" when the
	// answer is "the identity provider did".
	//
	// It is not called when a login changed nothing, which is almost every
	// login.
	OnChange func(ctx context.Context, p Principal, change RoleChange)
}

// RoleChange describes what a provisioning run altered.
type RoleChange struct {
	Created bool
	Added   []string
	Removed []string

	// Unknown lists mapped roles that do not exist in the store. They are
	// skipped rather than fatal — see Provision.
	Unknown []string
}

// Empty reports whether the run changed nothing.
func (c RoleChange) Empty() bool {
	return !c.Created && len(c.Added) == 0 && len(c.Removed) == 0
}

// Validate checks the provisioner is usable and that every role the map can
// grant exists.
//
// Call it at startup. A typo in the group mapping is a configuration error, and
// the module's convention is that those fail at boot rather than at the first
// request that would have matched them — here that would be somebody's login,
// which is a bad place to discover it.
func (p *Provisioner) Validate(ctx context.Context) error {
	if p.Store == nil {
		return errors.New("auth: provisioner needs a store")
	}
	if err := ValidatePrefix(p.Prefix); err != nil {
		return err
	}
	for _, role := range p.Roles.Mentions() {
		if _, err := p.Store.GetRole(ctx, role); err != nil {
			return fmt.Errorf("role map names %q: %w", role, err)
		}
	}
	return nil
}

// Provision upserts the principal for an asserted identity and reconciles its
// role bindings to match the identity provider.
//
// Roles are reconciled rather than merged: a user removed from a group at the
// provider loses the corresponding binding on their next login. Roles the map
// names but the store does not have are skipped and reported in the returned
// change rather than failing the login — Validate is where a typo should be
// caught, and by the time somebody is logging in, refusing everybody because
// one mapping is stale is the worse failure.
func (p *Provisioner) Provision(ctx context.Context, identity ExternalIdentity) (Principal, RoleChange, error) {
	var change RoleChange

	if p.Store == nil {
		return Principal{}, change, errors.New("auth: provisioner needs a store")
	}
	// PrincipalID enforces the prefix rules, including that a prefix can never
	// be the reserved one managed bindings live under. That is the single
	// enforcement point on purpose — a second check here would be one more
	// place for the two to disagree.
	id, err := identity.PrincipalID(p.Prefix)
	if err != nil {
		return Principal{}, change, err
	}

	principal, err := p.Store.GetPrincipal(ctx, id)
	switch {
	case errors.Is(err, ErrPrincipalNotFound):
		principal = Principal{ID: id, Kind: KindUser}
		change.Created = true
	case err != nil:
		return Principal{}, change, fmt.Errorf("look up principal %q: %w", id, err)
	case principal.Disabled:
		// An administrator disabling a federated account has to outrank the
		// provider still being willing to authenticate them. Without this,
		// `krk auth users disable` would be undone by the next login.
		return Principal{}, change, fmt.Errorf("%w: %q", ErrPrincipalDisabled, id)
	}

	if desired := p.merge(principal, identity); desired != nil {
		principal = *desired
		if err := p.Store.PutPrincipal(ctx, principal); err != nil {
			return Principal{}, change, fmt.Errorf("upsert principal %q: %w", id, err)
		}
	}

	if change, err = p.reconcile(ctx, principal, identity, change); err != nil {
		return Principal{}, change, err
	}
	if p.OnChange != nil && !change.Empty() {
		p.OnChange(ctx, principal, change)
	}
	return principal, change, nil
}

// merge returns the principal as the identity says it should be, or nil when it
// already says that. Skipping a no-op write keeps a login to reads on the
// overwhelmingly common path where nothing about the user changed.
func (p *Provisioner) merge(current Principal, identity ExternalIdentity) *Principal {
	next := current.Clone()
	next.Name = identity.DisplayName()
	next.Kind = KindUser

	attrs := map[string]string{}
	maps.Copy(attrs, identity.Attrs)
	if identity.Email != "" {
		attrs["email"] = identity.Email
	}
	if identity.Issuer != "" {
		attrs["issuer"] = identity.Issuer
	}
	if len(attrs) == 0 {
		attrs = nil
	}
	next.Attrs = attrs

	if next.ID == current.ID && next.Name == current.Name && next.Kind == current.Kind &&
		maps.Equal(next.Attrs, current.Attrs) {
		return nil
	}
	return &next
}

// reconcile brings the principal's managed bindings in line with the mapped
// grants, leaving every unmanaged binding alone.
//
// The unit of reconciliation is (role, scope), not role. Somebody who is an
// operator in two teams holds two bindings, and losing one group at the
// provider has to remove one of them and keep the other — keying on the role
// alone would take both away or neither.
func (p *Provisioner) reconcile(ctx context.Context, principal Principal, identity ExternalIdentity, change RoleChange) (RoleChange, error) {
	desired := p.Roles.Grants(identity.Groups)

	existing, err := p.Store.ListBindings(ctx, principal.ID)
	if err != nil {
		return change, fmt.Errorf("list bindings for %q: %w", principal.ID, err)
	}
	// Keyed on the binding's own fields rather than on its ID, so a binding
	// written before scopes existed is recognised as the grant it represents
	// and is left alone instead of being deleted and rewritten under a new ID.
	managed := map[RoleGrant]RoleBinding{}
	for _, b := range existing {
		if strings.HasPrefix(b.ID, ManagedBindingPrefix) {
			managed[RoleGrant{Role: b.Role, Scope: b.EffectiveScope()}] = b
		}
	}

	for _, grant := range desired {
		if _, ok := managed[grant]; ok {
			continue
		}
		if _, err := p.Store.GetRole(ctx, grant.Role); err != nil {
			if errors.Is(err, ErrRoleNotFound) {
				change.Unknown = append(change.Unknown, grant.Role)
				continue
			}
			return change, fmt.Errorf("look up role %q: %w", grant.Role, err)
		}
		if err := p.Store.PutBinding(ctx, RoleBinding{
			ID:          managedBindingID(principal.ID, grant),
			PrincipalID: principal.ID,
			Role:        grant.Role,
			Scope:       grant.Scope,
		}); err != nil {
			return change, fmt.Errorf("bind %q to %q: %w", principal.ID, grant.Role, err)
		}
		change.Added = append(change.Added, describeGrant(grant))
	}

	for grant, binding := range managed {
		if slices.Contains(desired, grant) {
			continue
		}
		if err := p.Store.DeleteBinding(ctx, binding.ID); err != nil && !errors.Is(err, ErrBindingNotFound) {
			return change, fmt.Errorf("unbind %q from %q: %w", principal.ID, grant.Role, err)
		}
		change.Removed = append(change.Removed, describeGrant(grant))
	}

	slices.Sort(change.Added)
	slices.Sort(change.Removed)
	slices.Sort(change.Unknown)
	change.Unknown = slices.Compact(change.Unknown)
	return change, nil
}

// managedBindingID is deterministic so reconciliation is idempotent and needs
// no extra column to record provenance: the ID itself carries it.
//
// The scope is part of it because a principal can hold one role at several
// scopes, and two bindings cannot share an ID.
func managedBindingID(principalID string, grant RoleGrant) string {
	return ManagedBindingPrefix + principalID + ":" + grant.Role + ":" + grant.EffectiveScope()
}

// describeGrant renders a grant for RoleChange, which an operator reads in an
// audit record. An unscoped grant renders as the bare role so the common case
// stays as legible as it was before scopes existed.
func describeGrant(grant RoleGrant) string {
	if scope := grant.EffectiveScope(); scope != "*" {
		return grant.Role + "@" + scope
	}
	return grant.Role
}
