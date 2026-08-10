module github.com/bsenel/karakuri/auth/sql

go 1.25.0

require (
	github.com/bsenel/karakuri/auth v0.0.0
	modernc.org/sqlite v1.56.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Until auth/v0.1.0 is tagged, resolve the parent module from the working
// tree. The release workflow refuses to publish a module carrying a replace
// directive, so this must be removed before auth/sql is tagged.
replace github.com/bsenel/karakuri/auth => ../
