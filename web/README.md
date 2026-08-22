# Karakuri Web

React + TypeScript browser UI for Karakuri. Consumes the existing REST + SSE
endpoints; no backend changes required.

## Requirements

- Node `^20.19.0 || ^22.12.0 || >=24.0.0` — the range vite 8 and vitest 4
  require between them (see `engines` in `package.json`)

## Develop

```bash
npm install
npm run dev      # http://localhost:5173 ; proxies /api → http://localhost:8080
```

The dev server expects a Karakuri server running on `localhost:8080`. Start it
with `make build && ./bin/server` (or `make docker-up`) before running the UI.

## Test

```bash
npm run test          # Vitest — hooks, permission gating, data transforms
npm run test:watch
npm run typecheck
npm run test:e2e      # Playwright — needs a Karakuri server on :8080
```

The e2e suite runs against its own server config, `e2e/karakuri.e2e.yaml`, which
differs from the shipped one in two ways: everything writable is under `/tmp`,
and the request rate limit is raised. The raised limit is worth knowing about
rather than hiding — a browser is a bursty client in a way `krk` is not, one page
load here is four or five requests, and the suite drives a dozen loads in under a
minute as one principal. That the SPA can reach the shipped default at all is a
real finding; the response to it is in `src/api/client.ts`, which retries a 429
once after the interval the server named.

```bash
mkdir -p /tmp/karakuri-e2e
KARAKURI_AUTH_JWT_SECRET=... KARAKURI_AUTH_BOOTSTRAP_PASSWORD=... \
  KARAKURI_CONFIG=web/e2e/karakuri.e2e.yaml ./bin/server &
cd web && KARAKURI_AUTH_BOOTSTRAP_PASSWORD=... npm run test:e2e
```

Vitest runs in jsdom and is what CI runs on every push. Playwright drives a real
browser against a real server, so it is a separate job: it is the only thing
here that catches a route guard which renders but does not navigate, and it is
far too slow to run on every save.

## Build

```bash
npm run build    # → web/dist
```

The Go server embeds `web/dist/` via `embed.FS` at `cmd/server/`, so once
`web/dist/` is present, the server binary serves the UI from `/` while keeping
`/api/v1/*` as the REST surface and `/api/v1/*/events` as SSE. SPA routes fall
back to `index.html`.

## Permissions in the interface

The navigation shows only what the signed-in principal can open, and where
somebody lands after signing in depends on what they hold — an auditor with no
objective access should not meet a 403 as their first impression of a system
that is working correctly.

**Hiding a link is a courtesy, not a control.** The server refuses the request
either way; what this buys is a menu without items that answer 403, which
otherwise teaches people to ignore errors. Nothing secret may rely on it:
anything that must not be seen has to be absent from the API's response, not
from the menu. `RequirePermission` carries the same caveat in its doc comment,
because that is where somebody will read it.

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
