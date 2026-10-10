package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

// sdkClientPythonEnv names a Python interpreter that has the official MCP SDK
// installed: the variable internal/platform/tools/mcp/sdk_interop_test.go reads.
const sdkClientPythonEnv = "KARAKURI_MCP_SDK_PYTHON"

// sdkClientScript is the official SDK's client over streamable HTTP: the
// initialize handshake, tools/list, one read-only tools/call, and what came
// back as one JSON object on stdout. It is written for both mcp 2.3.0 and mcp
// 1.30.0: streamable_http_client yields (read, write) in the first and
// (read, write, get_session_id) in the second, and a result's error flag is
// is_error in one and isError in the other.
const sdkClientScript = `import asyncio
import json
import sys
from importlib.metadata import version

import mcp
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client


def report(path, negotiated, listed, called):
    is_error = getattr(called, "is_error", None)
    if is_error is None:
        is_error = getattr(called, "isError", False)
    print(json.dumps({
        "sdk_version": version("mcp"),
        "client": path,
        "protocol_version": negotiated or "",
        "tools": [tool.name for tool in listed.tools],
        "is_error": bool(is_error),
        "text": "".join(getattr(block, "text", "") for block in called.content),
    }))


# Why this test once logged that the 2.3.0 client "negotiated 2025-06-18": the
# driver forced it. ClientSession.initialize() is the legacy handshake in every
# SDK version, so server/discover was never sent and nothing of Karakuri's was
# refused. In mcp 2.3.0 the negotiating client is mcp.Client
# (mcp/client/client.py): mode defaults to "auto" and Client.__aenter__ calls
# negotiate_auto(session), which probes server/discover and falls back to
# initialize on a legacy server. mcp 1.30.0 has no mcp.Client.
async def main(url):
    if hasattr(mcp, "Client"):
        async with mcp.Client(url) as client:
            listed = await client.list_tools()
            called = await client.call_tool("objectives_list", {})
            report("Client(mode=auto)", client.protocol_version, listed, called)
        return
    async with streamable_http_client(url) as streams:
        async with ClientSession(streams[0], streams[1]) as session:
            init = await session.initialize()
            listed = await session.list_tools()
            called = await session.call_tool("objectives_list", {})
            report("ClientSession.initialize", getattr(init, "protocolVersion", ""), listed, called)


asyncio.run(main(sys.argv[1]))
`

// The official SDK's client against this repository's own server, over the
// wire. Nothing here fakes the client. CI has no SDK installed, so the test
// skips unless an interpreter that has one is named.
func TestMCPServerSpeaksToTheSDKClient(t *testing.T) {
	python := os.Getenv(sdkClientPythonEnv)
	if python == "" {
		t.Skipf("skipping SDK client interop: set %s to a Python interpreter with the official MCP SDK installed (CI has none)",
			sdkClientPythonEnv)
	}

	f := newMCPFixture(t, karakuriauth.ActionObjectiveRead)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The same injection rpc does, in place of the Authenticate middleware.
		f.h.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), auth.Principal{ID: "runtime-1"})))
	}))
	t.Cleanup(srv.Close)

	script := filepath.Join(t.TempDir(), "sdk_client.py")
	if err := os.WriteFile(script, []byte(sdkClientScript), 0o600); err != nil {
		t.Fatalf("write the client script: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	// #nosec G204 -- the interpreter is chosen by the operator through KARAKURI_MCP_SDK_PYTHON
	cmd := exec.CommandContext(ctx, python, script, srv.URL)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("the SDK's client against %s: %v\nstdout:\n%s\nstderr:\n%s", srv.URL, err, stdout.String(), stderr.String())
	}

	var got struct {
		SDKVersion      string   `json:"sdk_version"`
		Client          string   `json:"client"`
		ProtocolVersion string   `json:"protocol_version"`
		Tools           []string `json:"tools"`
		IsError         bool     `json:"is_error"`
		Text            string   `json:"text"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &got); err != nil {
		t.Fatalf("the SDK's client printed %q (%v), want one JSON object\nstderr:\n%s", stdout.String(), err, stderr.String())
	}
	t.Logf("the SDK's client (mcp %s, %s) negotiated %q and was offered %v", got.SDKVersion, got.Client, got.ProtocolVersion, got.Tools)

	// Which path was negotiated: a handshake-only SDK (mcp 1.x) stays on
	// 2025-06-18, and one with the discover probe (mcp 2.x) must end on
	// 2026-07-28 against this server as it does against its own.
	want := "2026-07-28"
	if strings.HasPrefix(got.SDKVersion, "1.") {
		want = "2025-06-18"
	}
	if got.ProtocolVersion != want {
		t.Errorf("mcp %s (%s) negotiated %q, want %q", got.SDKVersion, got.Client, got.ProtocolVersion, want)
	}

	names := map[string]bool{}
	for _, name := range got.Tools {
		names[name] = true
	}
	if len(names) != 3 || !names["objectives_list"] || !names["objective_read"] || !names["reconcile_status"] {
		t.Errorf("the SDK's client was offered %v, want exactly the objective:read tools", got.Tools)
	}
	if got.IsError || !strings.Contains(got.Text, "Keep the build green") {
		t.Errorf("objectives_list through the SDK's client = %q (isError %v)", got.Text, got.IsError)
	}
}
