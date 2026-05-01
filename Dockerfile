FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Enable CGO for sqlite3
ENV CGO_ENABLED=1

# Install templ
RUN go install github.com/a-h/templ/cmd/templ@latest

# Install sqlc for code generation
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0

# Install build dependencies for bun + assets
RUN apt-get update && apt-get install -y ca-certificates sqlite3 curl unzip && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

ENV GOPROXY=direct
COPY go.mod go.sum ./
RUN go mod download

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

# Copy key source files first to auto-bust cache when they change
# This ensures the COPY . . layer is never stale
COPY web/shared/layout/layout.templ ./
RUN cat web/shared/layout/layout.templ | md5sum > /tmp/source_hash.txt

COPY . .

# Generate templ files
RUN templ generate

# Build frontend assets (tailwind + ts)
# Touch input file to force Tailwind to rescan
RUN touch ./static/style.css && bun run build:assets

# Generate sqlc code
RUN sqlc generate

# Build the server and CLI tools
RUN go build -o main_server ./cmd/server

FROM debian:bookworm-slim

WORKDIR /app

# Required at runtime (sqlite)
RUN apt-get update && apt-get install -y ca-certificates sqlite3 && rm -rf /var/lib/apt/lists/*

# Create data directory for sqlite
RUN mkdir -p /app/data

COPY --from=builder /app/main_server .
COPY --from=builder /app/static ./static
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/migrations ./migrations

# Expose the application port
EXPOSE 3000

ENTRYPOINT ["./main_server"]
