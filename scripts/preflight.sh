#!/usr/bin/env bash

set -euo pipefail

required_commands=(
  docker
  kubectl
  kind
  curl
  jq
  go
)

missing=()
for command_name in "${required_commands[@]}"; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    missing+=("${command_name}")
  fi
done

if [[ ${#missing[@]} -gt 0 ]]; then
  printf 'missing required tools: %s\n' "${missing[*]}" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker is installed but the daemon is not reachable" >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose CLI plugin is not available" >&2
  exit 1
fi

echo "preflight checks passed"
