package mcp

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// CodeHeaderMismatch is what a server answers a request whose HTTP headers
// disagree with its body: HEADER_MISMATCH in mcp_types/jsonrpc.py (the SDK,
// mcp 2.3.0).
const CodeHeaderMismatch = -32020

// IsModernRequest reports whether an HTTP request is on the 2026-07-28 path.
// The MCP-Protocol-Version header decides it and the body does not, as in the
// SDK's server (mcp/server/_streamable_http_modern.py): a request without the
// header is a 2025-06-18 one whatever its `_meta` says about itself.
func IsModernRequest(header http.Header) bool {
	return header.Get(headerProtocolVersion) != ""
}

// ClassifyInbound runs the validation ladder a server owes a 2026-07-28 request
// before anything is dispatched, and returns the first rung that failed or nil.
// It is classify_inbound_request in mcp/shared/inbound.py (the SDK, mcp 2.3.0),
// rung for rung and in its order; the codes and the message texts are the
// SDK's, not the specification page's:
//
//  1. params._meta carries the namespaced protocol version and client
//     capabilities (client info is optional), else -32602 naming what is
//     missing;
//  2. MCP-Protocol-Version equals the envelope's version, Mcp-Method equals the
//     method, and Mcp-Name equals the named param of a name-bearing method,
//     else -32020. Before rung 3, so a client that disagrees with itself is
//     told that rather than told its version is unsupported;
//  3. the version is one of supported, else -32022 with what is supported and
//     what was asked for.
//
// Whether the method exists is not a rung: dispatch answers that.
func ClassifyInbound(header http.Header, req Request, supported []string) *Error {
	var params map[string]json.RawMessage
	var meta map[string]json.RawMessage
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}
	if raw, ok := params["_meta"]; ok {
		_ = json.Unmarshal(raw, &meta)
	}
	if meta == nil {
		return &Error{
			Code: CodeInvalidParams,
			Message: fmt.Sprintf("params._meta must be an object carrying the required '%s' and '%s' envelope keys",
				metaKeyProtocolVersion, metaKeyClientCapabilities),
		}
	}
	var missing []string
	for _, key := range []string{metaKeyProtocolVersion, metaKeyClientCapabilities} {
		if _, ok := meta[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return &Error{
			Code:    CodeInvalidParams,
			Message: "params._meta is missing the required envelope key(s): " + strings.Join(missing, ", "),
		}
	}

	// A version that is not a string cannot equal a header, so it fails this
	// rung rather than the SDK's shape check after it, which only a transport
	// without headers reaches.
	var version string
	if err := json.Unmarshal(meta[metaKeyProtocolVersion], &version); err != nil || header.Get(headerProtocolVersion) != version {
		return &Error{
			Code:    CodeHeaderMismatch,
			Message: strings.ToLower(headerProtocolVersion) + " header does not match the request envelope's protocol version",
		}
	}
	if header.Get(headerMethod) != req.Method {
		return &Error{
			Code:    CodeHeaderMismatch,
			Message: strings.ToLower(headerMethod) + " header does not match the request body's method",
		}
	}
	if key, ok := nameBearingMethods[req.Method]; ok {
		// An absent or null param is not compared: the method's own handler
		// refuses a call that names nothing.
		if raw := params[key]; len(raw) > 0 && string(raw) != "null" {
			var want string
			got, sent := decodeHeaderValue(header.Values(headerName))
			if err := json.Unmarshal(raw, &want); err != nil || !sent || got != want {
				return &Error{
					Code:    CodeHeaderMismatch,
					Message: fmt.Sprintf("%s header does not match the request body's '%s' parameter", strings.ToLower(headerName), key),
				}
			}
		}
	}

	for _, v := range supported {
		if v == version {
			return nil
		}
	}
	// UnsupportedProtocolVersionErrorData in mcp_types/_types.py.
	return &Error{
		Code:    CodeUnsupportedProtocol,
		Message: "Unsupported protocol version",
		Data:    map[string]any{"supported": supported, "requested": version},
	}
}

// decodeHeaderValue is decode_header_value in mcp/shared/inbound.py, the
// inverse of encodeHeaderValue: a value is itself unless it is wrapped as
// `=?base64?...?=`. An absent header, or a wrapper that is not canonical base64
// of UTF-8, reports false so it never matches a body value by accident.
func decodeHeaderValue(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	const prefix, suffix = "=?base64?", "?="
	v := values[0]
	if len(v) < len(prefix)+len(suffix) || !strings.HasPrefix(v, prefix) || !strings.HasSuffix(v, suffix) {
		return v, true
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(v[len(prefix) : len(v)-len(suffix)])
	if err != nil || !utf8.Valid(raw) {
		return "", false
	}
	return string(raw), true
}

// ErrorHTTPStatus is the HTTP status a 2026-07-28 response carries for a
// JSON-RPC error code: ERROR_CODE_HTTP_STATUS in mcp/shared/inbound.py. One
// table for a refusal from the ladder and an error from a handler alike; a
// code it does not list, -32603 among them, travels with 200. The SDK's table
// also maps -32021 (MISSING_REQUIRED_CLIENT_CAPABILITY), which nothing here
// sends.
func ErrorHTTPStatus(code int) int {
	switch code {
	case CodeParseError, CodeInvalidRequest, CodeInvalidParams,
		CodeHeaderMismatch, CodeUnsupportedProtocol:
		return http.StatusBadRequest
	case CodeMethodNotFound:
		return http.StatusNotFound
	default:
		return http.StatusOK
	}
}
