#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
repo_root="$(cd -- "${script_dir}/.." >/dev/null 2>&1 && pwd)"

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

(
  cd "${repo_root}"
  printf '%s' "${password}" | go run ./cmd/create-user --email "${email}" --password-stdin
)
