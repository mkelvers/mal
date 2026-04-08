.PHONY: dev build test sqlc templ create-user

dev:
	air

build:
	go build -o main_server ./cmd/server

test:
	go test ./...

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
