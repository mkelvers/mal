FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Enable CGO for sqlite3
ENV CGO_ENABLED=1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Generate templ templates
RUN go run github.com/a-h/templ/cmd/templ@latest generate

# Build the server and the CLI tools
RUN go build -o main_server ./cmd/server
RUN go build -o create_user ./cmd/create-user

FROM debian:bookworm-slim

WORKDIR /app

# Install ffmpeg, sqlite and ca-certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    sqlite3 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create data directory for sqlite
RUN mkdir -p /app/data

COPY --from=builder /app/main_server .
COPY --from=builder /app/create_user .
COPY --from=builder /app/static ./static
COPY --from=builder /app/migrations ./migrations

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose the application port
EXPOSE 3000

# Run entrypoint which handles migrations
ENTRYPOINT ["/entrypoint.sh"]
CMD ["./main_server"]
