#!/usr/bin/env bash

set -euo pipefail

go test ./...
go vet ./...
bun run typecheck
bun run build:assets
