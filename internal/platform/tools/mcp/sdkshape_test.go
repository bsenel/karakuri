package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// The wire names of revision 2026-07-28 as the official Python SDK (mcp 2.3.0)
// declares them: mcp_types/_types.py for the `_meta` keys, mcp/shared/inbound.py
// for the headers and the name-bearing methods, mcp_types/jsonrpc.py for the
// code. They are spelled out here rather than taken from protocol.go so the
// fakes hold the client to the SDK's names and not to its own.
const (
	sdkMetaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	sdkMetaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	sdkMetaClientInfo         = "io.modelcontextprotocol/clientInfo"
	sdkMetaServerInfo         = "io.modelcontextprotocol/serverInfo"

	sdkHeaderProtocolVersion = "MCP-Protocol-Version"
	sdkHeaderMethod          = "Mcp-Method"
	sdkHeaderName            = "Mcp-Name"

	sdkCodeHeaderMismatch = -32020
)

// sdkNameBearing is NAME_BEARING_METHODS: the method, and the param whose value
// the Mcp-Name header repeats.
var sdkNameBearing = map[string]string{
	"tools/call":     "name",
	"prompts/get":    "name",
	"resources/read": "uri",
}

// sdkDecodeHeader is decode_header_value: a value is itself unless it is
// wrapped as `=?base64?...?=`, and a wrapper that does not decode matches
// nothing.
func sdkDecodeHeader(v string) (string, bool) {
	const prefix, suffix = "=?base64?", "?="
	if len(v) < len(prefix)+len(suffix) || !strings.HasPrefix(v, prefix) || !strings.HasSuffix(v, suffix) {
		return v, true
	}
	raw, err := base64.StdEncoding.DecodeString(v[len(prefix) : len(v)-len(suffix)])
	if err != nil || !utf8.Valid(raw) {
		return "", false
	}
	return string(raw), true
}

// sdkLadder is classify_inbound_request: the checks a 2026-07-28 server runs on
// a request before it dispatches it, in the SDK's order, first failure wins.
// headers is nil over stdio, which has none and skips that rung.
func sdkLadder(req Request, headers http.Header) *Error {
	var params map[string]json.RawMessage
	_ = json.Unmarshal(req.Params, &params)
	var meta map[string]json.RawMessage
	if json.Unmarshal(params["_meta"], &meta) != nil || meta == nil {
		return &Error{Code: CodeInvalidParams, Message: fmt.Sprintf(
			"params._meta must be an object carrying the required '%s' and '%s' envelope keys",
			sdkMetaProtocolVersion, sdkMetaClientCapabilities)}
	}
	var missing []string
	for _, key := range []string{sdkMetaProtocolVersion, sdkMetaClientCapabilities} {
		if _, ok := meta[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return &Error{Code: CodeInvalidParams, Message: "params._meta is missing the required envelope key(s): " + strings.Join(missing, ", ")}
	}
	var version string
	isString := json.Unmarshal(meta[sdkMetaProtocolVersion], &version) == nil

	if headers != nil {
		if got := headers.Values(sdkHeaderProtocolVersion); len(got) == 0 || !isString || got[0] != version {
			return &Error{Code: sdkCodeHeaderMismatch, Message: "mcp-protocol-version header does not match the request envelope's protocol version"}
		}
		if headers.Get(sdkHeaderMethod) != req.Method {
			return &Error{Code: sdkCodeHeaderMismatch, Message: "mcp-method header does not match the request body's method"}
		}
		if key, ok := sdkNameBearing[req.Method]; ok && len(params[key]) > 0 {
			var want string
			_ = json.Unmarshal(params[key], &want)
			if got, ok := sdkDecodeHeader(headers.Get(sdkHeaderName)); !ok || got != want {
				return &Error{Code: sdkCodeHeaderMismatch, Message: fmt.Sprintf("mcp-name header does not match the request body's '%s' parameter", key)}
			}
		}
	}

	if !isString {
		return &Error{Code: CodeInvalidParams, Message: "the protocol-version envelope value must be a string"}
	}
	if version != ModernProtocolVersion {
		return &Error{
			Code:    CodeUnsupportedProtocol,
			Message: "Unsupported protocol version",
			Data:    map[string]any{"supported": []string{ModernProtocolVersion}, "requested": version},
		}
	}
	return nil
}

// sdkDiscoverResult is a server/discover result in the SDK's shape: the server
// names itself under `_meta`, and there is no top-level serverInfo.
func sdkDiscoverResult(name string) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"resultType":        "complete",
		"supportedVersions": []string{ModernProtocolVersion},
		"capabilities":      map[string]any{"tools": map[string]any{"listChanged": false}},
		"cacheScope":        "private",
		"ttlMs":             0,
		"_meta":             map[string]any{sdkMetaServerInfo: Info{Name: name, Version: "1"}},
	})
	return out
}

// unsupportedInitialize is what a modern-only server answers the handshake
// with.
func unsupportedInitialize(req Request) *Error {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(req.Params, &p)
	return &Error{
		Code:    CodeUnsupportedProtocol,
		Message: "Unsupported protocol version",
		Data:    map[string]any{"supported": []string{ModernProtocolVersion}, "requested": p.ProtocolVersion},
	}
}

// modernHandle answers one request as a modern-only server does once the
// transport has let it through. headers is nil over stdio.
func modernHandle(req Request, headers http.Header, omitResultType bool) (Response, bool) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}
	if req.Method == MethodInitialize {
		resp.Error = unsupportedInitialize(req)
		return resp, true
	}
	if refused := sdkLadder(req, headers); refused != nil {
		resp.Error = refused
		return resp, true
	}
	if req.Method == MethodServerDiscover {
		resp.Result = sdkDiscoverResult("modern-fs")
		return resp, true
	}
	if result, asks := inputRequiredResult(req); asks {
		resp.Result = result
		return resp, true
	}
	resp, ok := fakeHandle(req)
	if !ok {
		return Response{}, false
	}
	if resp.Error == nil && !omitResultType {
		var result map[string]json.RawMessage
		_ = json.Unmarshal(resp.Result, &result)
		result["resultType"], _ = json.Marshal("complete")
		resp.Result, _ = json.Marshal(result)
	}
	return resp, true
}
