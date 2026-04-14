# MAL

<table align="center">
  <tr>
    <td>
      <img src="static/favicon.svg" alt="MAL logo" width="120" />
    </td>
    <td>
      <strong>MAL</strong><br />
      For everyone who wanted MyAnimeList-style tracking,<br />
      but with cleaner UI, better UX, and fewer compromises.
    </td>
  </tr>
</table>

<p align="center">
  A calmer anime tracker built for people who care about flow.
</p>
<p align="center">
  Discover anime, track progress, follow sequels, and keep your watchlist clean without fighting noisy UI.
</p>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/go-1.24-00ADD8?style=flat-square&logo=go" />
  <img alt="SQLite" src="https://img.shields.io/badge/database-sqlite-003B57?style=flat-square&logo=sqlite" />
  <img alt="templ" src="https://img.shields.io/badge/templ-server--side-111111?style=flat-square" />
  <img alt="HTMX" src="https://img.shields.io/badge/htmx-partial--updates-3366CC?style=flat-square" />
</p>

---

## Why this project exists

I built this because I genuinely disliked the UI and UX of the alternatives.

Most anime tracking tools I tried felt cluttered, slow to navigate, or missing basic pieces I wanted in one place. I wanted a product that felt focused: search quickly, open details quickly, decide status quickly, and move on.

This project is that answer.

Under the hood, it is also an engineering exercise in building a reliable product on top of an unreliable upstream data source. Anime metadata APIs can rate-limit, timeout, or intermittently fail. Instead of pretending that never happens, this codebase is designed around that reality.

## What you can do with it

- Browse a catalog view of popular anime
- Discover airing and upcoming shows
- Search instantly from the header quick-search
- Open anime detail pages with related titles and recommendations
- Add, update, remove, import, and export watchlist entries
- Track statuses (`watching`, `completed`, `plan_to_watch`, etc.)
- Get notification-oriented views for tracking and upcoming sequels
- Register/login/logout, rotate account recovery keys, and recover accounts

## Product philosophy

- **Minimal friction:** fewer clicks and less visual noise
- **Practical over perfect:** fast, readable pages over heavy front-end complexity
- **Resilient by default:** graceful fallback behavior when upstream services fail
- **Honest constraints:** explicitly acknowledge tradeoffs and incomplete pieces

## Technical overview

This is a server-rendered Go web application with SQLite persistence, generated SQL accessors, background workers, and templ-based UI rendering.

### Stack

- Go `1.24`
- SQLite (`github.com/mattn/go-sqlite3`)
- `sqlc` for typed query generation
- `templ` for server-side HTML components
- HTMX + small vanilla JS modules for interactivity
- Jikan API (`https://api.jikan.moe/v4`) as primary anime data provider
- `goquery` for watch-order parsing/fallback extraction

### Repository layout

```text
cmd/server/                  # app entrypoint
internal/server/             # route composition + middleware wiring
internal/features/anime/     # anime handlers + service logic
internal/features/watchlist/ # watchlist handlers + service logic
internal/features/auth/      # auth/session/recovery logic
internal/jikan/              # upstream API client + caching + retry paths
internal/worker/             # relation sync + retry processing + cache cleanup
internal/database/           # sqlc models/queries + migration runner
internal/templates/          # templ views and partials
migrations/                  # schema evolution scripts
static/                      # CSS/JS assets
```

## How the app works

At startup, the server:

1. Opens SQLite (`DATABASE_FILE`, default `mal.db`)
2. Runs migrations
3. Initializes typed DB queries, auth service, and Jikan client
4. Starts a background worker
5. Starts HTTP server on `PORT` (default `3000`)

Request flow is intentionally straightforward:

1. Request enters `http.ServeMux`
2. Middleware enforces origin checks and auth boundaries
3. Feature handlers call feature services
4. Services read/write SQLite and/or fetch from Jikan
5. Templ renders pages or partials

## The hard parts (and how they are handled)

### 1) Upstream instability

Jikan can return `429`/`5xx`, intermittently timeout, or vary response behavior under load.

Mitigations implemented in this codebase:

- Request pacing to stay under known rate limits (base delay between requests)
- Retry with bounded backoff and `Retry-After` support
- Cache-first reads where possible
- Stale cache fallback if fresh fetch fails
- Persisted retry queue (`anime_fetch_retry`) for retryable failures
- Worker signaling so retries can be processed quickly after enqueue

### 2) Keeping sequel graphs useful

Finding upcoming sequels is not just a single lookup. The app keeps relation data updated using worker jobs and recursive SQL CTEs, then maps those relations back to your watchlist context.

### 3) UI responsiveness without a heavy front-end framework

The app uses server-rendered templates plus HTMX partial updates and focused JS files. The goal is immediate UI feedback with less complexity than a large SPA stack.

### 4) Security basics done properly

- Password rules enforce complexity and minimum length
- Password hashes use bcrypt
- Session cookies are `HttpOnly` and `SameSite=Strict`
- `Secure` cookies are enabled in production mode (`ENV=production`)
- Origin/Referer checks protect non-GET actions
- Auth routes are rate-limited per IP

## Getting started

### Prerequisites

- Go `1.24+`
- SQLite
- `templ` CLI

### Local development

```bash
# install templ CLI
go install github.com/a-h/templ/cmd/templ@latest

# generate templ code
templ generate

# run server
go run ./cmd/server
```

Open `http://localhost:3000`.

### Docker

The repository includes a multi-stage `Dockerfile` that:

- installs dependencies
- generates templ files
- builds `./cmd/server`
- ships a slim runtime image with SQLite support

Example:

```bash
docker build -t mal .
docker run --rm -p 3000:3000 mal
```

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` | `3000` | HTTP listen port |
| `DATABASE_FILE` | `mal.db` | SQLite database file path |
| `ENV` | _(empty)_ | Set to `production` to mark session cookies as secure |

## Database and migrations

Migrations run automatically on startup.

Current migration history covers:

- initial auth/session/watchlist schema
- anime title and airing metadata
- notifications data model
- relation graph support
- Jikan response caching
- query-performance indexes
- account recovery key support
- persisted anime fetch retry queue

## Testing

The repo includes tests around core behavior such as:

- watchlist service logic
- watch-order parsing behavior
- relation helpers
- auth middleware behavior

Run all tests:

```bash
go test ./...
```

## Tradeoffs and known limitations

- Watchlist sorting by `score` is currently a placeholder path
- External data quality and uptime still depend on Jikan/third-party sources
- There is no formal CI pipeline configured in this repository yet
- Project docs (contributing/license) are still lightweight and evolving

## Roadmap direction

- Complete score sorting semantics for watchlist
- Expand test coverage for handler + integration paths
- Add clearer contribution and governance docs
- Improve observability around worker retries and cache health
- Continue refining the UI for speed and clarity over visual noise

## Development notes

- Generated templ outputs (`*_templ.go`) are checked in
- SQL is authored in `internal/database/queries.sql` and generated through `sqlc`
- Static assets live in `static/`

## Contributing

Please read `CONTRIBUTING.md` before opening a pull request.

If you want to contribute, open an issue or pull request with:

- the user-facing problem you are solving
- the technical approach and tradeoffs
- before/after behavior notes

Small, focused changes are preferred over broad rewrites.

## Security and secrets

- Do not commit real API keys or private credentials
- Keep local `.env` values out of documentation and screenshots
- If you discover a security issue, report it privately before public disclosure

## License

MIT. See `LICENSE`.

---

If this project resonates with you, it is probably for the same reason it exists: you want anime tracking that gets out of your way and lets you focus on actually watching.
