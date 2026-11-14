package software

import (
	"bufio"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
)

// settleOutsideTheWork marks a verification command whose result says nothing
// about the work (Phase 44 step 4). A command killed at its time limit is
// inconclusive: timeout, and lists the go test packages that finished before
// the limit, read from stdout. A command the shell could not find (exit 127)
// or could not execute (exit 126) is a tool this machine does not have, so it
// is inconclusive: unsupported (AGENTS.md rule 10). Any other result, a pass
// or a red suite, is left as the shell reported it.
func settleOutsideTheWork(res *environment.ActionResult, cmd, stdout string) {
	switch {
	case res.StateDelta["status"] == "timeout":
		finished := finishedGoPackages(stdout)
		res.StateDelta["state"] = string(objective.CriterionInconclusive)
		res.StateDelta["reason"] = string(objective.ReasonTimeout)
		res.StateDelta["finished_packages"] = finished
		res.Error = fmt.Sprintf("%s; %d package(s) finished before the limit, the run says nothing about the rest",
			res.Error, len(finished))
	case res.StateDelta["status"] == "exit_nonzero" && isMissingToolExit(res.StateDelta["exit_code"]):
		code := res.StateDelta["exit_code"]
		res.StateDelta["state"] = string(objective.CriterionInconclusive)
		res.StateDelta["reason"] = string(objective.ReasonUnsupported)
		res.Error = fmt.Sprintf("the command %q could not run on this machine (exit %v: %s)",
			truncate(cmd, 200), code, missingToolDetail(code, res.StateDelta["stderr"]))
	}
}

func isMissingToolExit(code any) bool {
	c, ok := code.(int)
	return ok && (c == 127 || c == 126)
}

// missingToolDetail says what the exit code means and, when the shell said
// which program it was, quotes that line: in a command like "cd web && npm
// test" the program that is missing is not the first word.
func missingToolDetail(code, stderr any) string {
	what := "a program it names is not installed"
	if code == 126 {
		what = "a program it names is not executable"
	}
	s, _ := stderr.(string)
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return what + ": " + truncate(last, 200)
	}
	return what
}

// goTestPackageLine is a go test summary line for one package: "ok  \tpkg\t0.1s",
// "ok  \tpkg\t(cached)", "FAIL\tpkg\t0.2s" or "?   \tpkg\t[no test files]". The
// bare closing "FAIL" and a test's "--- FAIL: TestX" are not.
var goTestPackageLine = regexp.MustCompile(`^(ok|FAIL|\?)\s+(\S+)\s`)

// finishedGoPackages lists the packages a go test run finished, with their
// result, from test2json lines (go test -json) or the plain summary lines. A
// package still running when the run stopped is not listed. The list is never
// nil: an empty one means nothing finished, which a reader must tell apart
// from a result nobody read.
func finishedGoPackages(stdout string) []map[string]any {
	results := map[string]string{}
	var order []string
	record := func(pkg, result string) {
		if _, seen := results[pkg]; !seen {
			order = append(order, pkg)
		}
		results[pkg] = result
	}
	sc := bufio.NewScanner(strings.NewReader(stdout))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "{") {
			var ev struct{ Action, Package, Test string }
			if json.Unmarshal([]byte(line), &ev) == nil && ev.Package != "" && ev.Test == "" &&
				(ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip") {
				record(ev.Package, ev.Action)
			}
			continue
		}
		if m := goTestPackageLine.FindStringSubmatch(line + "\n"); m != nil {
			record(m[2], map[string]string{"ok": "pass", "FAIL": "fail", "?": "skip"}[m[1]])
		}
	}
	finished := make([]map[string]any, 0, len(order))
	for _, pkg := range order {
		finished = append(finished, map[string]any{"package": pkg, "result": results[pkg]})
	}
	return finished
}
