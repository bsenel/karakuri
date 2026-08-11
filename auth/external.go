package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var (
	// ErrNoSubject is returned when an identity provider asserts an identity
	// with no subject. There is nothing to key a principal on, so there is
	// nothing safe to do with it.
	ErrNoSubject = errors.New("auth: external identity has no subject")

	// ErrInvalidSubject is returned when a subject carries characters that have
	// no business in an identifier — control characters, or leading/trailing
	// space that would make two visually identical principals distinct.
	ErrInvalidSubject = errors.New("auth: external identity subject is not a valid identifier")

	// ErrNoPrefix is returned when a Provisioner has no namespace prefix.
	//
	// This is fatal rather than defaulted because the prefix is what stops an
	// identity provider asserting sub=admin and taking over the local bootstrap
	// administrator. A prefix that can be forgotten is a prefix that will be.
	ErrNoPrefix = errors.New("auth: provisioner needs a namespace prefix")

	// ErrReservedPrefix is returned when a prefix would collide with the
	// reserved namespace managed bindings live in.
	ErrReservedPrefix = fmt.Errorf("auth: %q is reserved", strings.TrimSuffix(ManagedBindingPrefix, ":"))
)

// ExternalIdentity is what an identity provider asserted, normalised.
//
// Both protocols reduce to this: OIDC reads it out of ID-token claims, SAML out
// of assertion attributes. Everything downstream — role mapping, provisioning —
// works on this type alone and knows nothing about either protocol.
type ExternalIdentity struct {
	// Issuer identifies the provider that made the assertion. It is recorded on
	// the principal rather than used for lookup: a deployment federating two
	// providers should give them different prefixes.
	Issuer string

	// Subject is the provider's stable identifier for the user. It must be
	// stable across logins — an email address that changes when somebody
	// marries is not a subject, which is why OIDC has `sub`.
	Subject string

	Email string
	Name  string

	// Groups are the raw group or role names the provider asserted, before any
	// mapping. RoleMap turns them into Karakuri roles.
	Groups []string

	// Attrs are additional provider attributes to carry onto the principal,
	// where policy conditions can read them.
	Attrs map[string]string
}

// DisplayName is the friendliest name available for this identity.
//
// Principal IDs are namespaced and opaque (`oidc:8f3c…`), so without this every
// federated user reads as a UUID in `krk auth users list` and in the UI.
func (e ExternalIdentity) DisplayName() string {
	switch {
	case e.Name != "":
		return e.Name
	case e.Email != "":
		return e.Email
	default:
		return e.Subject
	}
}

// PrincipalID returns the namespaced principal ID for this identity.
//
// The prefix is not cosmetic. Local principals are named by an administrator
// ("admin", "alice"); federated ones are named by whoever controls the identity
// provider's subject field, which in some deployments is the user. Namespacing
// them means an external subject can never resolve to a local principal, so
// asserting sub=admin creates "oidc:admin" and grants nothing.
func (e ExternalIdentity) PrincipalID(prefix string) (string, error) {
	if err := ValidatePrefix(prefix); err != nil {
		return "", err
	}
	subject := e.Subject
	if strings.TrimSpace(subject) == "" {
		return "", ErrNoSubject
	}
	if subject != strings.TrimSpace(subject) {
		return "", fmt.Errorf("%w: leading or trailing space", ErrInvalidSubject)
	}
	for _, r := range subject {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: contains a control character", ErrInvalidSubject)
		}
	}
	return prefix + ":" + subject, nil
}

// ValidatePrefix reports whether a namespace prefix is usable.
func ValidatePrefix(prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return ErrNoPrefix
	}
	if prefix != strings.TrimSpace(prefix) || strings.Contains(prefix, ":") {
		return fmt.Errorf("%w: %q must be a bare word", ErrInvalidPattern, prefix)
	}
	if prefix+":" == ManagedBindingPrefix {
		return ErrReservedPrefix
	}
	return nil
}
