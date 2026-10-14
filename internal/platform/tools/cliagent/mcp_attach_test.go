package cliagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	mcpTestToken  = "krk-delegation-token-7f3a9c1e"
	mcpTestServer = "karakuri"
	mcpTestURL    = "http://127.0.0.1:8080/api/v1/mcp"
)

// stubTrace is what a scripted binary left behind: its argv, its environment
// and, when it was handed --mcp-config, a copy of that file taken while the
// run was still in progress.
type stubTrace struct {
	dir string
}

// newStubCLI writes a script that records its argv (one per line) and its
// environment under a trace directory, copies the file named after
// --mcp-config (mode preserved), and exits with exitCode.
func newStubCLI(t *testing.T, exitCode int) (bin string, trace stubTrace) {
	t.Helper()
	traceDir := filepath.Join(t.TempDir(), "trace")
	if err := os.MkdirAll(traceDir, 0o750); err != nil {
		t.Fatalf("mkdir trace: %v", err)
	}
	script := fmt.Sprintf(`#!/bin/sh
trace=%q
: > "$trace/argv"
prev=""
for a in "$@"; do
  printf '%%s\n' "$a" >> "$trace/argv"
  if [ "$prev" = "--mcp-config" ]; then
    cp -p "$a" "$trace/mcp.json"
  fi
  prev="$a"
done
env > "$trace/env"
exit %d
`, traceDir, exitCode)
	bin = filepath.Join(t.TempDir(), "stub-cli")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if err := os.Chmod(bin, 0o700); err != nil { // #nosec G302 -- the stub must be executable
		t.Fatalf("chmod stub: %v", err)
	}
	return bin, stubTrace{dir: traceDir}
}

// ran reports whether the stub was executed at all.
func (s stubTrace) ran() bool {
	_, err := os.Stat(filepath.Join(s.dir, "argv"))
	return err == nil
}

func (s stubTrace) argv(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.dir, "argv"))
	if err != nil {
		t.Fatalf("the stub binary was not executed (no argv recorded): %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func (s stubTrace) env(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.dir, "env"))
	if err != nil {
		t.Fatalf("the stub binary recorded no environment: %v", err)
	}
	return string(b)
}

// mcpConfigPath returns the argument that followed --mcp-config.
func (s stubTrace) mcpConfigPath(t *testing.T) string {
	t.Helper()
	argv := s.argv(t)
	i := slices.Index(argv, "--mcp-config")
	if i < 0 || i+1 >= len(argv) {
		t.Fatalf("argv has no `--mcp-config <file>`: ClaudeCode does not write a per-run MCP configuration yet; argv = %q", argv)
	}
	return argv[i+1]
}

// mcpConfigCopy returns the copy of the per-run file the stub took mid-run.
func (s stubTrace) mcpConfigCopy(t *testing.T) (content []byte, mode os.FileMode) {
	t.Helper()
	path := s.mcpConfigPath(t)
	copyPath := filepath.Join(s.dir, "mcp.json")
	info, err := os.Stat(copyPath)
	if err != nil {
		t.Fatalf("--mcp-config named %q but no such file existed while the CLI ran: %v", path, err)
	}
	content, err = os.ReadFile(copyPath)
	if err != nil {
		t.Fatalf("read copied MCP configuration: %v", err)
	}
	return content, info.Mode().Perm()
}

// allowedTools returns the comma-separated list of the single --allowed-tools= argument.
func allowedTools(t *testing.T, argv []string) []string {
	t.Helper()
	var found []string
	for _, a := range argv {
		if v, ok := strings.CutPrefix(a, "--allowed-tools="); ok {
			found = append(found, v)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one --allowed-tools= argument, got %d; argv = %q", len(found), argv)
	}
	return strings.Split(found[0], ",")
}

func testAttachment(tools ...string) *MCPAttachment {
	return &MCPAttachment{ServerName: mcpTestServer, URL: mcpTestURL, Token: mcpTestToken, Tools: tools}
}

func TestClaudeCode_NoMCP_ArgvUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		want    []string
	}{
		{"no allow-list", nil, []string{"--print", "--output-format=stream-json", "--verbose", "do the thing"}},
		{"with allow-list", []string{"Read", "Edit"}, []string{"--print", "--output-format=stream-json", "--verbose", "--allowed-tools=Read,Edit", "do the thing"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin, trace := newStubCLI(t, 0)
			_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
				Prompt: "do the thing", WorktreePath: t.TempDir(), AllowedTools: tc.allowed,
			})
			if err != nil {
				t.Fatalf("Delegate with MCP nil must behave as today, got error: %v", err)
			}
			argv := trace.argv(t)
			if !slices.Equal(argv, tc.want) {
				t.Errorf("with MCP nil the command line must not change:\n got %q\nwant %q", argv, tc.want)
			}
			for _, a := range argv {
				if strings.HasPrefix(a, "--mcp-config") || strings.HasPrefix(a, "--strict-mcp-config") {
					t.Errorf("with MCP nil argv must carry no MCP flag, found %q", a)
				}
			}
		})
	}
}

func TestClaudeCode_MCP_WritesPerRunConfigAndRemovesIt(t *testing.T) {
	for _, exitCode := range []int{0, 1} {
		t.Run(fmt.Sprintf("stub exits %d", exitCode), func(t *testing.T) {
			bin, trace := newStubCLI(t, exitCode)
			worktree := t.TempDir()
			_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
				Prompt: "read the audit log", WorktreePath: worktree, MCP: testAttachment("audit_list"),
			})
			if exitCode == 0 && err != nil {
				t.Fatalf("Delegate with an MCP attachment failed on a clean run: %v", err)
			}
			if exitCode != 0 && err == nil {
				t.Fatalf("Delegate must still report exit %d as an error when an MCP server is attached", exitCode)
			}

			argv := trace.argv(t)
			path := trace.mcpConfigPath(t)
			if !slices.Contains(argv, "--strict-mcp-config") {
				t.Errorf("argv lacks --strict-mcp-config: without it the CLI also loads the operator's own MCP servers; argv = %q", argv)
			}

			content, mode := trace.mcpConfigCopy(t)
			var got struct {
				MCPServers map[string]struct {
					Type    string            `json:"type"`
					URL     string            `json:"url"`
					Headers map[string]string `json:"headers"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(content, &got); err != nil {
				t.Fatalf("the per-run MCP configuration is not JSON: %v\n%s", err, content)
			}
			if len(got.MCPServers) != 1 {
				t.Errorf("mcpServers must hold exactly the attached server, got %d entries: %s", len(got.MCPServers), content)
			}
			srv, ok := got.MCPServers[mcpTestServer]
			if !ok {
				t.Fatalf("mcpServers has no entry named %q: %s", mcpTestServer, content)
			}
			if srv.Type != "http" {
				t.Errorf("mcpServers.%s.type = %q, want \"http\"", mcpTestServer, srv.Type)
			}
			if srv.URL != mcpTestURL {
				t.Errorf("mcpServers.%s.url = %q, want %q", mcpTestServer, srv.URL, mcpTestURL)
			}
			if want := "Bearer " + mcpTestToken; srv.Headers["Authorization"] != want {
				t.Errorf("mcpServers.%s.headers.Authorization = %q, want %q", mcpTestServer, srv.Headers["Authorization"], want)
			}
			if mode != 0o600 {
				t.Errorf("the per-run MCP configuration holds a credential and must be mode 0600, got %04o", mode)
			}

			if !filepath.IsAbs(path) {
				t.Errorf("--mcp-config path %q is relative, so it resolves inside the worktree the agent edits", path)
			}
			if isUnder(path, worktree) {
				t.Errorf("the per-run MCP configuration %q is under WorktreePath %q: the agent could commit the credential", path, worktree)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the per-run MCP configuration %q must be removed once Delegate returns (stub exit %d); stat err = %v", path, exitCode, err)
			}
		})
	}
}

// isUnder reports whether path lies inside dir, with symlinks in either resolved.
func isUnder(path, dir string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	parent := filepath.Join(resolve(filepath.Dir(path)), filepath.Base(path))
	rel, err := filepath.Rel(resolve(dir), parent)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestClaudeCode_MCP_AllowListNamesOnlyRequestedTools(t *testing.T) {
	bin, trace := newStubCLI(t, 0)
	_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
		Prompt: "report the cost", WorktreePath: t.TempDir(),
		AllowedTools: []string{"Read"},
		MCP:          testAttachment("audit_list", "cost_report"),
	})
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	got := allowedTools(t, trace.argv(t))
	want := []string{"Read", "mcp__" + mcpTestServer + "__audit_list", "mcp__" + mcpTestServer + "__cost_report"}
	if !slices.Equal(got, want) {
		t.Errorf("the allow-list must be the built-in tools plus mcp__<ServerName>__<tool> for each requested tool:\n got %q\nwant %q", got, want)
	}
	for _, tool := range got {
		if strings.HasSuffix(tool, "__checkpoints_list") || tool == "mcp__"+mcpTestServer {
			t.Errorf("the allow-list grants %q, which the action did not ask for", tool)
		}
	}
}

func TestClaudeCode_MCP_NoToolsIsAnError(t *testing.T) {
	bin, trace := newStubCLI(t, 0)
	c := NewClaudeCode(bin)
	in := DelegateInput{Prompt: "anything", WorktreePath: t.TempDir(), MCP: testAttachment()}

	if _, err := c.Delegate(context.Background(), in); err == nil {
		t.Error("Delegate with an MCP attachment that names no tool must return an error: it attaches nothing")
	}
	stream, err := c.Stream(context.Background(), in)
	if err == nil {
		var chunkErr error
		for chunk := range stream {
			if chunk.Kind == "error" && chunk.Err != nil {
				chunkErr = chunk.Err
			}
		}
		if chunkErr == nil {
			t.Error("Stream with an MCP attachment that names no tool must return or emit an error")
		}
	}
	if trace.ran() {
		t.Error("the CLI was executed although the MCP attachment names no tool")
	}
}

func TestClaudeCode_MCP_TokenOnlyInTheFile(t *testing.T) {
	bin, trace := newStubCLI(t, 0)
	_, err := NewClaudeCode(bin).Delegate(context.Background(), DelegateInput{
		Prompt: "read the audit log", WorktreePath: t.TempDir(), MCP: testAttachment("audit_list"),
	})
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	for i, a := range trace.argv(t) {
		if strings.Contains(a, mcpTestToken) {
			t.Errorf("argv[%d] carries the delegation token; a command line is readable by every process on the host", i)
		}
	}
	if strings.Contains(trace.env(t), mcpTestToken) {
		t.Error("the CLI's environment carries the delegation token; it must reach the CLI only through the per-run file")
	}
	content, _ := trace.mcpConfigCopy(t)
	if !strings.Contains(string(content), mcpTestToken) {
		t.Errorf("the per-run MCP configuration does not carry the token, so the CLI has no credential at all: %s", content)
	}
}

func TestAdaptersWithoutPerRunMCP(t *testing.T) {
	const prompt = "do the thing"
	cases := []struct {
		name     string
		adapter  func(bin string) CLIAgentAdapter
		wantArgv []string // nil: the adapter runs no binary
	}{
		{"cursor_cli", func(bin string) CLIAgentAdapter { return NewCursorCLI(bin) }, []string{"--print", "--output-format=stream-json", prompt}},
		{"gemini_cli", func(bin string) CLIAgentAdapter { return NewGeminiCLI(bin) }, []string{"--prompt", prompt}},
		{"copilot_cli", func(bin string) CLIAgentAdapter { return NewCopilotCLI(bin) }, []string{"copilot", "suggest", prompt}},
		{"noop", func(string) CLIAgentAdapter { return NewNoOp() }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/MCP set", func(t *testing.T) {
			bin, trace := newStubCLI(t, 0)
			a := tc.adapter(bin)
			_, err := a.Delegate(context.Background(), DelegateInput{
				Prompt: prompt, WorktreePath: t.TempDir(), MCP: testAttachment("audit_list"),
			})
			if err == nil {
				t.Fatalf("%s cannot attach an MCP server for one run; Delegate must say so instead of running without the tools", a.Name())
			}
			if !strings.Contains(err.Error(), a.Name()) {
				t.Errorf("the error must name the adapter %q, got: %v", a.Name(), err)
			}
			if !strings.Contains(err.Error(), "cannot attach an MCP server") {
				t.Errorf("the error must say the adapter \"cannot attach an MCP server\" for one run, got: %v", err)
			}
			if trace.ran() {
				t.Errorf("%s executed its binary although it had to refuse the MCP attachment", a.Name())
			}
		})
		t.Run(tc.name+"/MCP nil", func(t *testing.T) {
			bin, trace := newStubCLI(t, 0)
			a := tc.adapter(bin)
			out, err := a.Delegate(context.Background(), DelegateInput{Prompt: prompt, WorktreePath: t.TempDir()})
			if err != nil {
				t.Fatalf("%s with MCP nil must behave as today, got error: %v", a.Name(), err)
			}
			if tc.wantArgv == nil {
				if out.Summary != "no-op: no CLI agent configured" {
					t.Errorf("noop summary changed: %q", out.Summary)
				}
				return
			}
			if argv := trace.argv(t); !slices.Equal(argv, tc.wantArgv) {
				t.Errorf("%s with MCP nil must not change its command line:\n got %q\nwant %q", a.Name(), argv, tc.wantArgv)
			}
		})
	}
}
