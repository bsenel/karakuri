package auth

import "errors"

var (
	// ErrNoPrincipal is returned when a request reaches an authorization check
	// without an authenticated principal in its context.
	ErrNoPrincipal = errors.New("auth: no principal in context")

	// ErrPrincipalNotFound is returned by a Store when the requested principal
	// does not exist.
	ErrPrincipalNotFound = errors.New("auth: principal not found")

	// ErrPrincipalDisabled is returned when a principal exists but has been
	// disabled. Disabled principals are denied before any policy is consulted.
	ErrPrincipalDisabled = errors.New("auth: principal disabled")

	// ErrRoleNotFound is returned by a Store when the requested role does not
	// exist, and by role resolution when a binding or Inherits entry names a
	// role that was never registered.
	ErrRoleNotFound = errors.New("auth: role not found")

	// ErrRoleCycle is returned when role inheritance forms a cycle.
	ErrRoleCycle = errors.New("auth: role inheritance cycle")

	// ErrBindingNotFound is returned by a Store when the requested role binding
	// does not exist.
	ErrBindingNotFound = errors.New("auth: role binding not found")

	// ErrSystemRole is returned when a caller tries to modify or delete a role
	// flagged System. Built-in roles are immutable so an operator cannot lock
	// themselves out by editing "admin".
	ErrSystemRole = errors.New("auth: system roles are immutable")

	// ErrUnknownAction is returned when a policy names an action that is not in
	// the permission catalog. Nothing is implicit: an action nobody registered
	// cannot be granted.
	ErrUnknownAction = errors.New("auth: action not in catalog")

	// ErrInvalidPattern is returned when an action, resource or scope pattern is
	// malformed.
	ErrInvalidPattern = errors.New("auth: invalid pattern")

	// ErrForbidden is the sentinel a caller can match on when an authorization
	// check denies a request.
	ErrForbidden = errors.New("auth: forbidden")
)
