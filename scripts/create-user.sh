#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./scripts/create-user.sh <email> <password>

Creates a user directly in the SQLite database so they can log in.

Environment:
  DATABASE_FILE  SQLite database path (default: <repo>/mal.db)
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -gt 2 ]]; then
  usage >&2
  exit 1
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
repo_root="$(cd -- "${script_dir}/.." >/dev/null 2>&1 && pwd)"
db_file="${DATABASE_FILE:-${repo_root}/mal.db}"

if [[ "${db_file}" != /* ]]; then
  db_file="$(pwd)/${db_file}"
fi

email="${1:-}"
password="${2:-}"

if [[ -z "${email}" ]]; then
  read -r -p 'Email: ' email
fi

if [[ -z "${password}" ]]; then
  read -r -s -p 'Password: ' password
  printf '\n'
fi

if [[ -z "${email}" || -z "${password}" ]]; then
  printf 'Email and password are required.\n' >&2
  exit 1
fi

if [[ ! -f "${db_file}" ]]; then
  printf 'Database file not found: %s\n' "${db_file}" >&2
  printf 'Run the server once to apply migrations, or set DATABASE_FILE.\n' >&2
  exit 1
fi

tmp_dir="${repo_root}/tmp"
mkdir -p "${tmp_dir}"
tmp_go="$(mktemp "${tmp_dir}/create-user.XXXXXX.go")"
trap 'rm -f "$tmp_go"' EXIT

cat >"${tmp_go}" <<'EOF'
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

func randomID() (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(id), nil
}

func trimTrailingNewline(v string) string {
	v = strings.TrimSuffix(v, "\n")
	v = strings.TrimSuffix(v, "\r")
	return v
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run create-user.go <db-file> <email>")
		os.Exit(2)
	}

	dbFile := os.Args[1]
	email := strings.TrimSpace(os.Args[2])

	passwordBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read password: %v\n", err)
		os.Exit(1)
	}

	password := trimTrailingNewline(string(passwordBytes))
	if email == "" || password == "" {
		fmt.Fprintln(os.Stderr, "email and password are required")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to hash password: %v\n", err)
		os.Exit(1)
	}

	userID, err := randomID()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create user id: %v\n", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	_, err = db.Exec(
		"INSERT INTO user (id, username, password_hash) VALUES (?, ?, ?)",
		userID,
		email,
		string(hash),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: user.username") {
			fmt.Fprintf(os.Stderr, "user already exists: %s\n", email)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stderr, "failed to insert user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created user: %s\n", email)
}
EOF

(
  cd "${repo_root}"
  printf '%s' "${password}" | go run "${tmp_go}" "${db_file}" "${email}"
)
