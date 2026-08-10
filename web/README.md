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

The browser never holds a token. `login()` asks the server for cookie mode, and
the access and refresh tokens come back as **httpOnly, SameSite=Strict cookies**
this code cannot read — which is the point: anything reachable from JavaScript
is readable by injected script, and a stolen refresh token is a persistent
session.

What follows from that:

- Every request sets `credentials: 'same-origin'` so the cookies ride along.
  Without it `fetch` omits them and every call 401s.
- There is no expiry to check, because the token is invisible here. Refresh is
  reactive: a 401 triggers one refresh and one retry.
- Refresh tokens rotate, so concurrent 401s share a single in-flight refresh.
  Letting each refresh independently would spend the token more than once, and
  the server treats a replayed refresh token as a leak and revokes the whole
  session family.
- SSE carries no token in the URL. `EventSource` cannot set headers — the usual
  reason SSE ends up with `?access_token=` — but it does send cookies, so the
  stream authenticates the same way everything else does. URLs leak into access
  logs, proxy logs and Referer headers; cookies do not.

CSRF is handled by `SameSite=Strict`: the browser will not attach these cookies
to any request initiated from another site, which works because the SPA is
served from the same origin as the API.

API clients (`krk`, CI) are unaffected and still use bearer tokens.

On a fresh install the server creates an `admin` account using
`KARAKURI_AUTH_BOOTSTRAP_PASSWORD`.
