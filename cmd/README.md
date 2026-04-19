# cmd layout

This repository keeps executable entrypoints under `cmd/`.

- `cmd/server`: main web process (`go run ./cmd/server`)

Why this exists:

- Keeps the repository root focused on project metadata (`README.md`, `go.mod`, `Dockerfile`, etc)
- Scales cleanly if more binaries are added later (for example: `cmd/migrate`, `cmd/admin`, `cmd/worker`)
- Matches common Go repository conventions for multi-binary or growing services
