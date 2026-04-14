# Contributing

Thanks for your interest in improving MAL.

## Before you start

- Open an issue first for large changes so scope is clear
- Keep pull requests focused and small when possible
- Prioritize user-facing clarity: cleaner flows, less friction, better defaults

## Local setup

```bash
# install templ CLI
go install github.com/a-h/templ/cmd/templ@latest

# generate templates
templ generate

# run tests
go test ./...

# run app
go run ./cmd/server
```

## Development guidelines

- Follow existing folder boundaries (`internal/features/*`, `internal/jikan`, `internal/templates`)
- Prefer simple, explicit solutions over broad abstractions
- Do not add dependencies unless there is a clear benefit
- Keep generated files in sync when changing `.templ` or SQL query definitions

## Pull request checklist

- Explain the user problem this change solves
- Describe tradeoffs or constraints
- Include before/after behavior notes
- Ensure `go test ./...` passes locally

## Security

- Never commit secrets, private tokens, or real credentials
- Keep `.env` values local
- Report security issues privately before public disclosure
