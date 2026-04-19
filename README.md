# MyAnimeList

<table align="center">
  <tr>
    <td>
      <picture>
        <source media="(prefers-color-scheme: dark)" srcset="static/readme-logo-dark.svg" />
        <img src="static/readme-logo-light.svg" alt="MyAnimeList logo" width="140" />
      </picture>
    </td>
    <td>
      <strong>MyAnimeList</strong><br />
      My personal anime tracker, built because nothing else felt right.
    </td>
  </tr>
</table>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/go-1.24-00ADD8?style=flat-square&logo=go" />
  <img alt="SQLite" src="https://img.shields.io/badge/database-sqlite-003B57?style=flat-square&logo=sqlite" />
  <img alt="templ" src="https://img.shields.io/badge/templ-server--rendered-111111?style=flat-square" />
  <img alt="HTMX" src="https://img.shields.io/badge/htmx-partial--updates-3366CC?style=flat-square" />
</p>

---

## Why this project exists

I built this for myself.

I was frustrated with the UI and UX of every tracker I tried. Even when something looked decent, it still felt awkward to use day-to-day, or it was missing pieces I considered essential. I wanted one place that matched how I actually watch anime: search fast, get context fast, update status fast, and move on.

So this project is personal first and public second. I put it on GitHub because I like shipping in the open, not because it was originally designed as a general-purpose product for everyone.

Technically, I also wanted to prove that a small, server-rendered Go app could stay reliable even when upstream anime APIs are inconsistent. A lot of this code exists because real APIs rate-limit, timeout, and occasionally fail at the worst possible moment.

## What the application offers

For my own workflow, MyAnimeList combines catalog browsing, seasonal discovery, quick search, detail pages with recommendations and relations, full watchlist management, continue-watching, and in-app playback in one server-rendered interface.

Authentication in the web UI is login-only.

## Technical approach

The application is written in Go and rendered on the server with `templ`, with SQLite as the primary datastore and `sqlc` for typed query generation. HTMX and small JavaScript modules handle incremental interactions, which keeps the interface responsive without moving the entire product into a heavy client-side architecture.

The external anime data source is Jikan (`https://api.jikan.moe/v4`). Because reliability is a first-class concern, the client layer includes request pacing, bounded retries, backoff behavior, stale-cache fallback, and a persisted retry queue for failed fetches that should be retried later.

## Repository structure

Instead of treating the repository as one flat service, the codebase is organized into focused boundaries.

| Path | Purpose |
| --- | --- |
| `cmd/server` | Application entrypoint and process lifecycle setup |
| `internal/server` | Route registration and middleware composition |
| `internal/features/anime` | Catalog, discovery, search, details, recommendations, and relations |
| `internal/features/watchlist` | Watchlist updates, retrieval, import/export, and continue-watching |
| `internal/features/playback` | Watch page, stream/subtitle proxying, and watch progress APIs |
| `internal/features/auth` | Login/session handling and auth service logic |
| `internal/jikan` | Upstream API client, caching, and retry-aware fetch behavior |
| `internal/worker` | Background relation sync, retry processing, and cache cleanup |
| `internal/database` | Migration runner, generated query layer, and DB models |
| `internal/templates` | Server-rendered page and partial templates |
| `internal/watchorder` | Watch-order scraping and parsing helpers |
| `migrations` | Schema evolution and operational DB changes |
| `static` | Source CSS, TypeScript, and static assets |
| `dist` | Built frontend assets served at `/dist/*` |

`cmd/` structure notes are documented in `cmd/README.md`.

## Runtime behavior

On startup, the server opens SQLite using `DATABASE_FILE` (defaulting to `mal.db`), runs migrations automatically, initializes core services, starts the background worker, and then serves HTTP traffic on `PORT` (defaulting to `3000`). A request enters the router, passes through global middleware for origin and auth boundaries, reaches a feature handler, and then resolves through service logic that combines database access with upstream data where needed before rendering HTML.

Public access is intentionally limited. `/`, `/login`, `/search`, `/api/search`, `/api/search-quick`, and static asset routes are available without auth; most other routes require a valid session.

The background worker continuously maintains relation data for sequel awareness, processes queued retryable anime fetches, and periodically removes expired cache records. This keeps user-facing pages stable even when data collection has to happen in multiple phases.

## Reliability and tradeoffs

The hardest part has been balancing freshness and resilience. Upstream APIs can fail transiently with `429` and `5xx` responses, so the app favors graceful degradation over hard failure. Cached values are used when fresh requests fail, retryable failures are persisted and replayed in the worker, and relation synchronization is incremental so one bad fetch does not block the rest of the graph.

There are still honest limits. Metadata quality still depends partly on external providers, and there is also no formal CI pipeline yet, so local validation is currently the main quality gate.

## Getting started

For local development, install Go `1.24+`, Bun, and the `templ` CLI, then generate templates, build frontend assets, and run the server.

```bash
go install github.com/a-h/templ/cmd/templ@latest
bun install
templ generate
./scripts/check.sh
PLAYBACK_PROXY_SECRET="your-32+char-secret" go run ./cmd/server
```

The frontend pipeline uses a single source stylesheet (`static/style.css`) and TypeScript sources in `static/*.ts`, then emits build artifacts into `dist/` (`dist/tailwind.css` and `dist/*.js`) for serving.

When the server starts, the app is available at `http://localhost:3000`.

Important notes:
- Environment variables are read directly from the process environment (`PORT`, `DATABASE_FILE`, `ENV`, `PLAYBACK_PROXY_SECRET`); `.env` is not auto-loaded.
- The web app currently exposes a login route only. If your database has no users yet, create the first user outside the web UI.

For containerized usage, the included `Dockerfile` uses a multi-stage build that installs Bun + templ, builds assets, generates templates, compiles `cmd/server`, and ships a slim runtime image with SQLite support.

```bash
docker build -t myanimelist .
docker run --rm -p 3000:3000 myanimelist
```

For persistent data in containers, set `DATABASE_FILE` to `/app/data/mal.db` and mount a volume:

```bash
docker run --rm \
  -p 3000:3000 \
  -e DATABASE_FILE=/app/data/mal.db \
  -v "$(pwd)/data:/app/data" \
  myanimelist
```

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `3000` | HTTP listen port |
| `DATABASE_FILE` | `mal.db` | SQLite database file path |
| `ENV` | _(empty)_ | Set to `production` to enable secure session cookies |
| `MIGRATIONS_DIR` | _(auto-discovered)_ | Optional explicit path to migration files |
| `PLAYBACK_PROXY_SECRET` | _(required)_ | HMAC secret for signed playback proxy tokens (min 32 chars) |

## Database and testing

Migrations run at startup, so schema changes are applied automatically before the server begins accepting traffic. Migration history includes the initial auth and watchlist schema, anime metadata expansion, relation tracking, Jikan cache persistence, indexing updates, and retry-queue support for failed fetches.

Current automated tests are unit-focused and cover watchlist behavior, relation helpers, auth middleware boundaries, and watch-order parsing. Run the full local validation suite with:

```bash
./scripts/check.sh
```

There is currently no CI workflow in this repository, so validation is local.

## Security

Keep secrets out of version control, do not publish real credentials in documentation or screenshots, and report security issues privately before public disclosure.

## License

This project is released under the MIT License. See `LICENSE` for details.
