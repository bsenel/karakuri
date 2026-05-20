package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Playwright is a TestingAdapter that runs `npx playwright test --reporter=json`
// as a subprocess inside a configured project directory and parses results.
// Requires Node + Playwright to be installed locally.
type Playwright struct {
	projectDir string // cwd for npx; usually the dir containing playwright.config.ts
	npxBin     string // override "npx" if needed (e.g. "/usr/local/bin/npx")
}

func NewPlaywright(projectDir string) *Playwright {
	return &Playwright{projectDir: projectDir, npxBin: "npx"}
}

func (p *Playwright) Name() string { return "playwright" }

func (p *Playwright) Active() bool { return p.projectDir != "" }

func (p *Playwright) RunTests(ctx context.Context, path string) ([]TestResult, error) {
	if p.projectDir == "" {
		return nil, fmt.Errorf("playwright: project_dir not configured")
	}
	args := []string{"playwright", "test", "--reporter=json"}
	if path != "" {
		args = append(args, path)
	}
	cmd := exec.CommandContext(ctx, p.npxBin, args...)
	cmd.Dir = p.projectDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Playwright exits with code 1 when tests fail — that's not an adapter error;
	// the JSON reporter still emits results. So we ignore the exit code and parse stdout.
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("playwright: empty output (stderr: %s)", stderr.String())
	}

	var report playwrightReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return nil, fmt.Errorf("playwright: parse JSON output: %w; stderr: %s", err, stderr.String())
	}
	return flattenSpecs(report.Suites, ""), nil
}

// ── JSON reporter shape (subset we care about) ───────────────────────────────

type playwrightReport struct {
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSuite struct {
	Title  string            `json:"title"`
	Specs  []playwrightSpec  `json:"specs"`
	Suites []playwrightSuite `json:"suites"`
}

type playwrightSpec struct {
	Title string           `json:"title"`
	Tests []playwrightTest `json:"tests"`
}

type playwrightTest struct {
	Results []playwrightResult `json:"results"`
}

type playwrightResult struct {
	Status string `json:"status"` // "passed", "failed", "timedOut", "skipped"
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func flattenSpecs(suites []playwrightSuite, prefix string) []TestResult {
	var out []TestResult
	for _, s := range suites {
		title := s.Title
		if prefix != "" {
			title = prefix + " > " + title
		}
		for _, spec := range s.Specs {
			name := title + " > " + spec.Title
			passed := true
			var msg string
			for _, t := range spec.Tests {
				for _, r := range t.Results {
					if r.Status != "passed" {
						passed = false
					}
					if r.Error != nil {
						msg = r.Error.Message
					}
				}
			}
			out = append(out, TestResult{Name: name, Passed: passed, Output: msg})
		}
		out = append(out, flattenSpecs(s.Suites, title)...)
	}
	return out
}
