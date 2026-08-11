module github.com/bsenel/karakuri/auth/oidc

go 1.25.0

require (
	github.com/bsenel/karakuri/auth v0.0.0
	github.com/coreos/go-oidc/v3 v3.20.0
	golang.org/x/oauth2 v0.36.0
)

require github.com/go-jose/go-jose/v4 v4.1.4 // indirect

// Until auth/v0.1.0 is tagged, resolve the parent module from the working
// tree. The release workflow refuses to publish a module carrying a replace
// directive, so this must be removed before auth/oidc is tagged.
replace github.com/bsenel/karakuri/auth => ../
