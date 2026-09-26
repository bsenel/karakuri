package mcp

import (
	"context"
	"fmt"
)

// Transport kinds, as they appear in config and in /health.
const (
	TransportStdio = "stdio"
	TransportHTTP  = "http"
)

// transport carries one JSON-RPC message to a server and returns its reply.
//
// Two implementations, and the interface is the smallest thing they share: a
// subprocess talking newline-delimited JSON over a pipe, and an HTTP endpoint
// answering a POST with either a JSON body or an SSE stream. Both are
// request/response from the client's side, which is all this client needs —
// server-initiated requests (sampling, elicitation) are not implemented, and
// pretending the interface supported them would be a method nothing answers.
//
// Send is expected to be called one call at a time; Client holds a mutex around
// it rather than each transport growing its own.
type transport interface {
	// Kind is the transport name reported in /health.
	Kind() string
	// Send writes a request and returns the response that matches its ID.
	Send(ctx context.Context, req Request) (*Response, error)
	// Notify writes a notification, which by definition has no reply.
	Notify(ctx context.Context, req Request) error
	// Close releases the subprocess or the idle connections.
	Close() error
}

// newTransport builds the transport a Config asks for.
//
// An unknown kind is an error rather than a silent default. Defaulting to stdio
// would run a command for a config that meant to reach a URL, and defaulting to
// HTTP would POST to an empty address; both fail later, with a message about the
// wrong thing.
func newTransport(cfg Config) (transport, error) {
	switch cfg.Transport {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport needs a command")
		}
		return newStdioTransport(cfg)
	case TransportHTTP:
		if cfg.URL == "" {
			return nil, fmt.Errorf("http transport needs a url")
		}
		return newHTTPTransport(cfg), nil
	case "":
		return nil, fmt.Errorf("no transport declared (want %q or %q)", TransportStdio, TransportHTTP)
	default:
		return nil, fmt.Errorf("unknown transport %q (want %q or %q)", cfg.Transport, TransportStdio, TransportHTTP)
	}
}
