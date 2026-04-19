# cmd

Executable entrypoints live here.

- `cmd/server`: main web process (`go run ./cmd/server`)

## Why this structure

I wanted to keep the repository root clean and focused on project metadata like `README.md`, `go.mod`, and `Dockerfile`. Keeping entrypoints under `cmd/` also makes it easy to add more binaries later without cluttering the root, and it matches standard Go conventions for projects that grow beyond a single binary.