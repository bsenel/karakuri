# Karakuri Web

React + TypeScript browser UI for Karakuri. Consumes the existing REST + SSE
endpoints; no backend changes required.

## Requirements

- Node 18 or later

## Develop

```bash
npm install
npm run dev      # http://localhost:5173 ; proxies /api → http://localhost:8080
```

The dev server expects a Karakuri server running on `localhost:8080`. Start it
with `make build && ./bin/server` (or `make docker-up`) before running the UI.

## Build

```bash
npm run build    # → web/dist
```

The Go server embeds `web/dist/` via `embed.FS` at `cmd/server/`, so once
`web/dist/` is present, the server binary serves the UI from `/` while keeping
`/api/v1/*` as the REST surface and `/api/v1/*/events` as SSE. SPA routes fall
back to `index.html`.

## Auth

The login modal stores the bearer token in `localStorage` under the
`karakuri_token` key. Token value must match `auth.token` in the server config
(or the `KARAKURI_AUTH_TOKEN` env var). An empty server token disables auth
checks; the UI still works without filling the modal.
