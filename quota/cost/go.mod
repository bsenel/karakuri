module github.com/bsenel/karakuri/quota/cost

go 1.25.0

require github.com/bsenel/karakuri/quota v0.0.0

// Until quota/v0.1.0 is tagged, resolve the parent module from the working
// tree. The release workflow refuses to publish a module carrying a replace
// directive, so this must be removed before quota/cost is tagged.
replace github.com/bsenel/karakuri/quota => ../
