#!/usr/bin/env bash
set -euo pipefail

cd /workspace

mode="write"
if [[ "${1:-}" == "--check" ]]; then
  mode="check"
fi

mapfile -t files < <(find cmd internal -name '*.go' -type f | sort)
if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

if [[ "$mode" == "check" ]]; then
  unformatted="$(gofmt -l "${files[@]}")"
  if [[ -n "$unformatted" ]]; then
    echo "$unformatted"
    exit 1
  fi
  exit 0
fi

gofmt -w "${files[@]}"