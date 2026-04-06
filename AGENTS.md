# malago

personal anime tracking platform. go/htmx rewrite.

## stack

- go standard library (`net/http`)
- htmx + templ templates
- sqlite + sqlc
- tailwind (dark theme)
- jikan api (myanimelist)

## structure

```
cmd/server/           main entry
internal/
  auth/               sessions, passwords
  database/           sqlc generated, migrations
  handlers/           http handlers by domain
  middleware/         auth, logging
  jikan/              api client
  templates/          templ components
migrations/           sql files
static/               css, js
```

## go patterns

### errors

always handle errors explicitly. wrap with context using `fmt.Errorf`:

```go
if err != nil {
    return fmt.Errorf("failed to fetch user: %w", err)
}
```

use `errors.Is` and `errors.As` to check wrapped errors.

### early returns

reduce nesting. check errors first, return early:

```go
func getUser(id string) (*User, error) {
    if id == "" {
        return nil, ErrInvalidID
    }
    
    user, err := db.FindUser(id)
    if err != nil {
        return nil, err
    }
    
    return user, nil
}
```

### defer for cleanup

always close resources with defer:

```go
resp, err := http.Get(url)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### interfaces

accept interfaces, return structs. keep interfaces small:

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}
```

### naming

- short, lowercase package names: `auth`, `jikan`, `db`
- `MixedCaps` for exports, `mixedCaps` for internal
- getters: `Owner()` not `GetOwner()`
- interfaces: single method = method name + `er` suffix (`Reader`, `Writer`)

### zero values

design structs so zero value is useful:

```go
var buf bytes.Buffer // ready to use, no init needed
buf.WriteString("hello")
```

### composition over inheritance

embed types to compose behavior:

```go
type Handler struct {
    db     *database.Queries
    jikan  *jikan.Client
    logger *slog.Logger
}
```

## htmx

check for htmx requests:

```go
func isHTMX(r *http.Request) bool {
    return r.Header.Get("HX-Request") == "true"
}
```

return partials for htmx, full pages otherwise. use `hx-swap-oob` for multiple updates. trigger toasts with `HX-Trigger` header.

## templ

render components directly to response:

```go
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
    templates.HomePage(data).Render(r.Context(), w)
}
```

pass data explicitly. keep components focused. use layouts.

## database

sqlc generates type-safe queries. always use parameterized queries.

tables: `user`, `session`, `account`, `anime`, `watch_list_entry`

watch statuses: `watching`, `completed`, `on_hold`, `dropped`, `plan_to_watch`

## jikan api

rate limit: 3 req/sec max. stagger batch requests. cache in local db.

## commands

```bash
make dev          # hot reload (air)
make build        # binary
make test         # tests
make migrate      # run migrations
make sqlc         # generate code
make create-user  # cli user creation
```

## env

```bash
DATABASE_FILE=malago.db
SESSION_SECRET=min_32_chars_random
PORT=3000
```

## avoid

- panics in handlers
- forgetting `defer resp.Body.Close()`
- unstaggered jikan requests (429 errors)
- globals for config/state
- large monolithic templates
