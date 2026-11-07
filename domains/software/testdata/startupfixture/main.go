// Command startupfixture is the CLI whose start-up time the cli_startup_ms
// verifier's tests measure. It sleeps for -sleep before it exits, so a test
// knows a floor for the wall time, and exits with -exit, so a test can tell
// whether the verifier applied params.args. -help exits 0, as the flag
// package does by default.
package main

import (
	"flag"
	"os"
	"time"
)

func main() {
	sleep := flag.Duration("sleep", 0, "how long to sleep before exiting")
	code := flag.Int("exit", 0, "the exit status")
	flag.Parse()
	time.Sleep(*sleep)
	os.Exit(*code)
}
