# `auth/saml` — SAML 2.0 authentication

Overrides parent rules for `auth/saml/` only. This directory is a **separate Go module**
(`github.com/bsenel/karakuri/auth/saml`), a submodule of `auth` in the sense of ADR 007 —
its own `go.mod` so a consumer of `auth` does not pull `crewjam/saml` unless they speak
SAML.

## Hard rules

1. **No Karakuri imports**, same as the parent. The only dependency on this repo is
   `github.com/bsenel/karakuri/auth`.
2. **No protocol reimplementation.** Signature validation, audience and time-window checks,
   canonicalisation and the XML itself are `crewjam/saml`'s job. Hand-rolling any of it is a
   way to get signature verification subtly wrong, which fails open.
3. **Nothing here decides what anybody may do.** The assertion becomes an
   `auth.ExternalIdentity`; roles come from the host's `RoleMap`.

## Conventions

- **There is no `TokenResolver`, and that is deliberate.** A SAML assertion is a one-time
  login artifact delivered by browser POST — single-use, bound to one recipient URL, valid
  for minutes. It is not a credential a client can present per request. The host mints its
  own session after the ACS handler, and that session authenticates everything afterwards.
  If someone asks for a machine-to-machine SAML path, the answer is `auth/oidc`.
- **The flow cookie is `SameSite=None; Secure`.** This is the detail that decides whether
  login works at all: the identity provider returns the assertion as a cross-site form
  POST, and browsers do not attach `Lax` cookies to cross-site POSTs — only to top-level
  GET navigations. A `Lax` cookie never arrives, correlation fails, and every login is
  rejected as unsolicited. `InsecureAllowHTTP` falls back to `Lax` because browsers reject
  `None` without `Secure`; that is a development-only compromise.
- **Attribute names are matched against both `Name` and `FriendlyName`.** ADFS sends a URI
  in `Name` and nothing friendly, Okta sends a bare word, Shibboleth sends both. Do not add
  a per-vendor branch — add a default and let configuration do the rest.
- **`AllowIDPInitiated` is off by default.** With it on there is no request of ours to
  correlate against, so the guarantee that a response answers a request we actually sent is
  gone. Some deployments need it; it should stay a decision an operator makes.
- **Failures say why.** `crewjam.InvalidResponseError.Error()` is deliberately vague
  ("Authentication failed") and the real cause hides in `PrivateErr`. The ACS handler
  unwraps it before reporting, because a host that logs an authentication failure wants the
  reason.

## Testing

`go test ./... -race` from this directory. Coverage is gated at 90% in CI
(`.github/workflows/ci.yml`, job `modules`).

Tests drive a **real `crewjam.IdentityProvider`** (`stub_test.go`), not a fake. SAML's whole
surface is signed XML, so a hand-written response would prove only that this package can
parse whatever the test happened to write. One thing to know when reading them: the
assertion arrives inside an HTML form, so `html/template` has escaped it — notably `+` as
`&#43;`, which is common in base64. A browser unescapes before submitting; the tests do the
same.

## Releasing

`.github/workflows/release-auth.yml` already matches `auth/*/v*.*.*` and derives the module
directory from the tag, so `auth/saml/v0.1.0` needs no workflow of its own. Drop the
`replace` directive before tagging.
