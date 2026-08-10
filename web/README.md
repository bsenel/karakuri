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

The login form exchanges an ID and password for a short-lived access token and a
refresh token, stored together in `localStorage` under `karakuri_session`.

`api/client.ts` refreshes transparently: any call made within a minute of the
access token's expiry refreshes first, and a 401 triggers one retry. Because
refresh tokens rotate on every use, concurrent calls share a single in-flight
refresh — letting each 401 refresh independently would spend the token more than
once, and the server treats a replayed refresh token as a leak and revokes the
whole session family.

SSE is the one place a token travels in a query string (`?access_token=`), since
`EventSource` cannot set headers. The server accepts that only on stream
endpoints.

On a fresh install the server creates an `admin` account and logs its password
once at startup.
