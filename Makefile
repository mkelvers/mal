.PHONY: dev build test migrate sqlc create-user

dev:
	air

build:
	go build -o main_server ./cmd/server

test:
	go test ./...

migrate:
	sqlite3 mal.db < migrations/001_init.sql

sqlc:
	sqlc generate

templ:
	templ generate

create-user:
	@if [ -z "$(EMAIL)" ] || [ -z "$(PASSWORD)" ]; then \
		echo "Usage: make create-user EMAIL=your@email.com PASSWORD=yourpassword"; \
	else \
		go run ./cmd/create-user -email=$(EMAIL) -password=$(PASSWORD); \
	fi
