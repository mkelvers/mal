FROM golang:1.24-bullseye AS builder

WORKDIR /app

# Enable CGO for sqlite3
ENV CGO_ENABLED=1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the server and the CLI tools
RUN go build -o main_server ./cmd/server

FROM debian:bullseye-slim

WORKDIR /app

# Required for sqlite
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Create data directory for sqlite
RUN mkdir -p /app/data

COPY --from=builder /app/main_server .
COPY --from=builder /app/static ./static
COPY --from=builder /app/migrations ./migrations

# Expose the application port
EXPOSE 3000

# Run entrypoint which handles migrations
CMD ["./main_server"]
