package mcp

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// The interop tests run Karakuri's real client against a server built on the
// official Python SDK. Nothing in this file fakes a server. They need an
// interpreter that has the SDK installed and the probe script (one tool,
// echo(text)), which CI has neither of, so they skip unless both are named.
const (
	sdkPythonEnv = "KARAKURI_MCP_SDK_PYTHON"
	sdkProbeEnv  = "KARAKURI_MCP_SDK_PROBE"
)

// sdkProbe returns the interpreter and the probe script, or skips.
func sdkProbe(t *testing.T) (python, probe string) {
	t.Helper()
	python, probe = os.Getenv(sdkPythonEnv), os.Getenv(sdkProbeEnv)
	if python == "" || probe == "" {
		t.Skipf("skipping SDK interop: set %s to a Python interpreter with the official MCP SDK installed and %s to the probe server script",
			sdkPythonEnv, sdkProbeEnv)
	}
	return python, probe
}

// startSDKHTTPServer runs the probe over streamable HTTP on a free port and
// returns its endpoint once it accepts connections.
func startSDKHTTPServer(t *testing.T) string {
	t.Helper()
	python, probe := sdkProbe(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	addr := l.Addr().String()
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var output bytes.Buffer
	// #nosec G204 -- a test that starts the SDK's server from the interpreter and script the operator named in the environment
	cmd := exec.Command(python, probe, "http", strconv.Itoa(port))
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s %s: %v", python, probe, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("probe server output:\n%s", output.String())
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("probe server never accepted a connection on %s: %v", addr, err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Sprintf("http://%s/mcp", addr)
}

// sdkListAndEcho lists the probe's one tool and calls it.
func sdkListAndEcho(ctx context.Context, t *testing.T, c *Client) {
	t.Helper()
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("ListTools = %+v, want the probe's one tool, echo", tools)
	}
	const text = "karakuri says hello"
	res, err := c.CallTool(ctx, "echo", map[string]any{"text": text})
	if err != nil {
		t.Fatalf("CallTool echo: %v", err)
	}
	if res.IsError || res.Text() != text {
		t.Errorf("echo returned %q (isError %v), want %q", res.Text(), res.IsError, text)
	}
}

func TestSDKInteropHTTPNegotiatesDiscover(t *testing.T) {
	url := startSDKHTTPServer(t)
	c := dial(t, Config{Transport: TransportHTTP, URL: url})
	ctx := t.Context()

	got, err := c.Negotiate(ctx)
	if err != nil {
		t.Fatalf("Negotiate against the SDK's server: %v", err)
	}
	if got.Path != PathDiscover {
		t.Errorf("Path = %q, want %q: the SDK's server has %s", got.Path, PathDiscover, MethodServerDiscover)
	}
	if got.ProtocolVersion != ModernProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", got.ProtocolVersion, ModernProtocolVersion)
	}
	if got.ServerInfo.Name != "probe" {
		t.Errorf("ServerInfo.Name = %q, want probe", got.ServerInfo.Name)
	}
	if info, version := c.ServerInfo(); info.Name != "probe" || version != ModernProtocolVersion {
		t.Errorf("recorded server %q at %q, want probe at %s", info.Name, version, ModernProtocolVersion)
	}
	sdkListAndEcho(ctx, t, c)
}

// There is no production knob that forces the handshake: Negotiate always tries
// server/discover first. The legacy path is taken here the only way the
// production API offers, by calling Initialize instead of Negotiate, against
// the same default server, which still answers 2025-06-18.
func TestSDKInteropHTTPLegacyHandshakeStillConnects(t *testing.T) {
	url := startSDKHTTPServer(t)
	c := dial(t, Config{Transport: TransportHTTP, URL: url})
	ctx := t.Context()

	got, err := c.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize against the SDK's server: %v", err)
	}
	if got.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", got.ProtocolVersion, ProtocolVersion)
	}
	if got.ServerInfo.Name != "probe" {
		t.Errorf("ServerInfo.Name = %q, want probe", got.ServerInfo.Name)
	}
	sdkListAndEcho(ctx, t, c)
}

// The probe as a child process through the real stdio transport. The asserted
// path is an observation of mcp 2.3.0, made on 2026-10-09: its stdio server
// answers server/discover at 2026-07-28 and names itself under `_meta`, as its
// HTTP server does. A later SDK that negotiates differently fails here, which
// is the point.
func TestSDKInteropStdio(t *testing.T) {
	python, probe := sdkProbe(t)
	// Close, which dial registers with t.Cleanup, ends the child process.
	c := dial(t, Config{Transport: TransportStdio, Command: python, Args: []string{probe, "stdio"}})
	ctx := t.Context()

	got, err := c.Negotiate(ctx)
	if err != nil {
		t.Fatalf("Negotiate against the SDK's stdio server: %v", err)
	}
	t.Logf("stdio negotiated path %q at %q with server %q", got.Path, got.ProtocolVersion, got.ServerInfo.Name)
	if got.Path != PathDiscover || got.ProtocolVersion != ModernProtocolVersion || got.ServerInfo.Name != "probe" {
		t.Errorf("stdio negotiated %q at %q with server %q, want %q at %s with server probe",
			got.Path, got.ProtocolVersion, got.ServerInfo.Name, PathDiscover, ModernProtocolVersion)
	}
	sdkListAndEcho(ctx, t, c)
}
