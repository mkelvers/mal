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
  <img alt="Go" src="https://img.shields.io/badge/go-1.25-00ADD8?style=flat-square&logo=go" />
  <img alt="SQLite" src="https://img.shields.io/badge/database-sqlite-003B57?style=flat-square&logo=sqlite" />
  <img alt="templ" src="https://img.shields.io/badge/templ-server--rendered-111111?style=flat-square" />
  <img alt="Tailwind" src="https://img.shields.io/badge/tailwind-4-06B6D4?style=flat-square&logo=tailwindcss" />
  <img alt="HTMX" src="https://img.shields.io/badge/htmx-partial--updates-3366CC?style=flat-square" />
</p>

---

> [!WARNING]
> **This repository is archived.** It's still functional and you can run it if you want, but I won't be maintaining it anymore. The codebase became too complex for what it is, and the maintenance burden just isn't worth it.

## Why this project exists

I built this for myself.

I was frustrated with the UI and UX of every tracker I tried. Even when something looked decent, it still felt awkward to use day-to-day, or it was missing pieces I considered essential. I wanted one place that matched how I actually watch anime: search fast, get context fast, update status fast, and move on.

So this project is personal first and public second. I put it on GitHub because I like shipping in the open, not because it was originally designed as a general-purpose product for everyone.

Technically, I also wanted to prove that a small, server-rendered Go app could stay reliable even when upstream anime APIs are inconsistent. A lot of this code exists because real APIs rate-limit, timeout, and occasionally fail at the worst possible moment.

## What the application offers

For my own workflow, MyAnimeList combines catalog browsing, seasonal discovery, quick search, detail pages with recommendations and relations, watchlist management, continue-watching, and in-app playback in one server-rendered interface.

The interface is minimal and functional, featuring a dark theme and quick access to tracking tools.

## Technical approach

The application is written in Go and rendered on the server with `templ`, with SQLite as the primary datastore and `sqlc` for typed query generation. Styling uses Tailwind CSS v4. HTMX and small TypeScript modules handle incremental interactions, which keeps the interface responsive without moving the entire product into a heavy client-side architecture.

The external anime data source is Jikan (`https://api.jikan.moe/v4`). Because reliability is a first-class concern, the client layer includes request pacing, bounded retries, backoff behavior, stale-cache fallback, and a persisted retry queue for failed fetches that should be retried later. Playback proxying uses uTLS to bypass Cloudflare protections.

## Repository structure

The codebase follows standard Go project layout conventions with clear separation between public APIs, external integrations, private internals, and web presentation.

### Public API Layer

| Path | Purpose |
| --- | --- |
| `api/anime` | Catalog, discovery, search, details, recommendations, and relations |
| `api/auth` | Login and session handling logic |
| `api/playback` | Watch page, stream/subtitle proxying, and watch progress APIs |
| `api/watchlist` | Watchlist updates, retrieval, and continue-watching |

### External Integrations

| Path | Purpose |
| --- | --- |
| `integrations/jikan` | Upstream API client, caching, and retry-aware fetch behavior |
| `integrations/watchorder` | Watch-order scraping and parsing helpers |

### Private Internal Code

| Path | Purpose |
| --- | --- |
| `cmd/server` | Application entrypoint and process lifecycle setup |
| `internal/db` | Migration runner, generated query layer, and DB models |
| `internal/middleware` | App-specific auth and access policy middleware |
| `internal/server` | Route registration and middleware composition |
| `internal/worker` | Background relation sync, retry processing, and cache cleanup |

### Reusable Libraries

| Path | Purpose |
| --- | --- |
| `pkg/middleware` | Generic HTTP middleware (CSRF, rate limiting, logging) |

### Web Layer

| Path | Purpose |
| --- | --- |
| `web/templates` | Server-rendered page and partial templates |
| `web/components` | Reusable UI components and icons |

### Assets & Operations

| Path | Purpose |
| --- | --- |
| `migrations` | Schema evolution and operational DB changes |
| `static` | Source CSS, TypeScript, and static assets |
| `dist` | Built frontend assets served at `/dist/*` |

`cmd/` structure notes are documented in `cmd/README.md`.

## Runtime behavior

On startup, the server opens SQLite using `DATABASE_FILE` (defaulting to `mal.db`), runs migrations automatically, initializes core services, starts the background worker, and then serves HTTP traffic on `PORT` (defaulting to `3000`). A request enters the router, passes through global middleware for origin and auth boundaries, reaches a feature handler, and then resolves through service logic that combines database access with upstream data where needed before rendering HTML.

Public access is restricted. Only `/login` and static asset routes (`/static/*`, `/dist/*`) are available without authentication; all other routes require a valid session. There is no public registration; users are created via the CLI:

```bash
go run ./cmd/server create-user <username> <password>
```

The background worker continuously maintains relation data for sequel awareness, processes queued retryable anime fetches, and periodically removes expired cache records. This keeps user-facing pages stable even when data collection has to happen in multiple phases.

## Reliability and tradeoffs

The hardest part has been balancing freshness and resilience. Upstream APIs can fail transiently with `429` and `5xx` responses, so the app favors graceful degradation over hard failure. Cached values are used when fresh requests fail, retryable failures are persisted and replayed in the worker, and relation synchronization is incremental so one bad fetch does not block the rest of the graph.

Playback proxying originally struggled with Cloudflare challenges; this was solved by switching to uTLS for TLS fingerprinting bypass. HTML sanitization was added to prevent XSS from upstream anime data.

There are still honest limits. Metadata quality depends on external providers, and there is no formal CI pipeline yet, so local validation is the primary quality gate.

## Getting started

For local development, install Go `1.25+`, Bun, and the `templ` CLI, then generate templates, build frontend assets, and run the server.

```bash
bun install                                    # Install Bun dependencies
go install github.com/a-h/templ/cmd/templ@latest # Install templ CLI
templ generate                                 # Generate Go templates from .templ files
bun run build:css && bun run build:ts          # Build frontend assets (CSS and TypeScript)
PLAYBACK_PROXY_SECRET="your-32+char-secret" go run ./cmd/server  # Run the Go server
```

The frontend pipeline uses Tailwind CSS v4 (`static/style.css`) and TypeScript sources in `static/*.ts`, then emits build artifacts into `dist/` for serving.

When the server starts, the app is available at `http://localhost:3000`.

### Creating a user

The app has no public registration. Use the built-in CLI command to create a user:

```bash
go run ./cmd/server create-user <username> <password>
# or with a built binary:
./server create-user <username> <password>
```

If the username already exists, you will be prompted to confirm overwriting the password.

### Justfile

Common tasks are automated via the `justfile`. Run `just <task>` after installing [`just`](https://github.com/casey/just):

| Task | Description |
| --- | --- |
| `just fmt` | Format Go code |
| `just lint` | Run go fmt and go vet |
| `just test` | Run Go tests |
| `just templ` | Regenerate templ files |
| `just build` | Full build (templ, Go binary, CSS, TS) |
| `just check` | Run all checks (lint, test, typecheck, build) |
| `just dev` | Build and start the server |
| `just install-hooks` | Install lefthook pre-push hooks |

For containerized usage:

```bash
docker build -t myanimelist .
docker run --rm -p 3000:3000 -e PLAYBACK_PROXY_SECRET="your-32+char-secret" myanimelist
```

For persistent data in containers, set `DATABASE_FILE` to `/app/data/mal.db` and mount a volume:

```bash
docker run --rm \
  -p 3000:3000 \
  -e DATABASE_FILE=/app/data/mal.db \
  -v "$(pwd)/data:/app/data" \
  myanimelist
```

After the container is running, exec into it to create a user:

```bash
docker exec <container> /server create-user <username> <password>
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

Migrations run at startup automatically. Schema history includes auth, watchlist, anime metadata, relation tracking, Jikan cache persistence, and retry-queue support.

There is no CI workflow, so validation is local. Use `just check` to run all checks (lint, test, typecheck, build) or `just install-hooks` to set up the pre-push hook that runs them automatically before each push.

> [!NOTE]
> [`just`](https://github.com/casey/just) must be installed first (e.g. `brew install just`).

Alternatively, run tests manually with:

```bash
go test ./...
```

## Security

Keep secrets out of version control, do not publish real credentials in documentation or screenshots, and report security issues privately before public disclosure.

## License

This project is released under the MIT License. See `LICENSE` for details.
