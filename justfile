set shell := ["bash", "-c"]
set dotenv-load := true

fmt:
    go fmt ./...

lint:
    go fmt ./... && go vet ./...

test:
    go test ./...

build-go:
    go build -o server ./cmd/server

build-css:
    bunx @tailwindcss/cli -i ./static/style.css -o ./dist/tailwind.css

build-ts:
    bun build ./static/*.ts --outdir ./dist --target browser

build: build-go build-css build-ts

typecheck:
    bunx tsc -p tsconfig.json --noEmit

check: lint test typecheck build

install-hooks:
    bunx lefthook install

dev: build
    ./server

db_migrate:
    go run ./cmd/server migrate

clean:
    rm -rf dist/*
    rm -f server
