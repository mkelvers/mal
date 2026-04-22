FROM golang:1.24-bullseye AS builder

WORKDIR /app

# Enable CGO for sqlite3
ENV CGO_ENABLED=1

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Install build dependencies for bun + assets
RUN apt-get update && apt-get install -y ca-certificates sqlite3 curl unzip && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

COPY go.mod go.sum ./
RUN go mod download

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .

# Build frontend assets (tailwind + ts)
RUN bun run build:assets

# Generate templ files
RUN templ generate

# Build the server and CLI tools
RUN go build -o main_server ./cmd/server

FROM debian:bullseye-slim

WORKDIR /app

# Required at runtime (sqlite)
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Create data directory for sqlite
RUN mkdir -p /app/data

COPY --from=builder /app/main_server .
COPY --from=builder /app/entrypoint.sh .
COPY --from=builder /app/static ./static
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/migrations ./migrations

RUN chmod +x ./entrypoint.sh

# Expose the application port
EXPOSE 3000

# Run entrypoint which handles migrations and cache clearing
ENTRYPOINT ["./entrypoint.sh"]
