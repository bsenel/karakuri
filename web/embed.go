// Package web embeds the React SPA built into web/dist at the Go binary
// level. The embedded files are served at / by the API server (see
// internal/api/server.go), with /api/v1/* routed first so REST + SSE win
// over the catch-all SPA fallback.
//
// Routing rules at the HTTP layer:
//
//   - /api/v1/*           → REST + SSE handlers (mounted first)
//   - /assets/*, *.css,
//     *.js, *.svg, etc.   → served verbatim from web/dist
//   - everything else     → index.html (React Router takes over client-side)
//
// If web/dist is empty (the frontend wasn't built before `go build`), the
// handler responds with a helpful 200 OK explaining how to populate it; the
// REST surface keeps working in either case.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler serving the embedded SPA.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notBuilt()
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return notBuilt()
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/")
		if urlPath == "" {
			urlPath = "index.html"
		}
		if _, err := fs.Stat(sub, urlPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		if hasAssetExt(urlPath) {
			http.NotFound(w, r)
			return
		}
		// SPA fallback — rewrite to /, let the file server serve index.html.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func hasAssetExt(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	switch ext {
	case ".css", ".js", ".map", ".svg", ".png", ".jpg", ".jpeg", ".gif",
		".webp", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".json":
		return true
	}
	return false
}

func notBuilt() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html>
<html><head><meta charset="utf-8"><title>Karakuri</title></head>
<body style="font-family:-apple-system,sans-serif;max-width:640px;margin:48px auto;color:#222;line-height:1.5;">
  <h1>⌬ Karakuri</h1>
  <p>The browser UI hasn't been built yet. Run:</p>
  <pre style="background:#eef;padding:12px;border-radius:6px;">cd web &amp;&amp; npm install &amp;&amp; npm run build</pre>
  <p>Then rebuild the server and reload this page.</p>
  <p>The REST + SSE API is fully available at <code>/api/v1/*</code>.</p>
</body></html>`))
	})
}
