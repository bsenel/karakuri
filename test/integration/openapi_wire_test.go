package integration_test

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
)

// specHost stands in for whatever address a test server listens on, so one
// router built from the spec matches every test server.
const specHost = "http://karakuri.test"

var (
	specRouterOnce sync.Once
	specRouter     routers.Router
	specRouterErr  error
)

// loadSpecRouter parses docs/openapi.yaml once for the whole package.
func loadSpecRouter() (routers.Router, error) {
	specRouterOnce.Do(func() {
		doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
		if err != nil {
			specRouterErr = err
			return
		}
		if err := doc.Validate(context.Background()); err != nil {
			specRouterErr = err
			return
		}
		doc.Servers = openapi3.Servers{{URL: specHost + apiPrefix}}
		specRouter, specRouterErr = legacy.NewRouter(doc)
	})
	return specRouter, specRouterErr
}

// validateResponses wraps the API so every JSON answer a test receives is
// checked against docs/openapi.yaml: its status must be one the operation
// documents, and its body must match that response's schema. The static check
// covers the Go types; this covers what the handlers actually write, the maps
// they build by hand among them. A streamed or binary answer is passed through.
func validateResponses(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	router, err := loadSpecRouter()
	if err != nil {
		t.Fatalf("load docs/openapi.yaml: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An event stream is never complete, so it cannot be buffered and
		// checked; it goes straight through.
		if strings.HasSuffix(r.URL.Path, "/events") || strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			next.ServeHTTP(w, r)
			return
		}
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)

		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())

		if !strings.HasPrefix(r.URL.Path, apiPrefix+"/") {
			return
		}
		mediaType, _, _ := mime.ParseMediaType(rec.Header().Get("Content-Type"))
		if mediaType != "application/json" {
			return
		}
		match := r.Clone(context.Background())
		match.URL.Scheme, match.URL.Host = "http", strings.TrimPrefix(specHost, "http://")
		match.Host = match.URL.Host
		route, pathParams, err := router.FindRoute(match)
		if err != nil {
			t.Errorf("%s %s answered %d but matches no operation in docs/openapi.yaml: %v", r.Method, r.URL.Path, rec.Code, err)
			return
		}
		input := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{Request: match, PathParams: pathParams, Route: route},
			Status:                 rec.Code,
			Header:                 rec.Header(),
			Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
			Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
		}
		if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
			t.Errorf("%s %s answered %d with a body docs/openapi.yaml does not describe: %v\nbody: %.300s",
				r.Method, route.Path, rec.Code, err, rec.Body.String())
		}
	})
}
