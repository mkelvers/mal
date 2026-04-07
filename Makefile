.PHONY: dev build test migrate sqlc create-user

dev:
	air

build:
	go build -o main_server ./cmd/server

test:
	go test ./...

migrate:
	for f in migrations/*.sql; do sqlite3 mal.db < "$$f" 2>/dev/null || true; done

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
