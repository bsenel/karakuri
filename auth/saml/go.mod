module github.com/bsenel/karakuri/auth/saml

go 1.25.0

require (
	github.com/bsenel/karakuri/auth v0.0.0
	github.com/crewjam/saml v0.5.1
)

require (
	github.com/beevik/etree v1.6.0 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/mattermost/xml-roundtrip-validator v0.1.0 // indirect
	github.com/russellhaering/goxmldsig v1.6.0 // indirect
	golang.org/x/crypto v0.33.0 // indirect
)

// Until auth/v0.1.0 is tagged, resolve the parent module from the working
// tree. The release workflow refuses to publish a module carrying a replace
// directive, so this must be removed before auth/saml is tagged.
replace github.com/bsenel/karakuri/auth => ../
