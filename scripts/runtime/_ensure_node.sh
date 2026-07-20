#!/usr/bin/env bash
# Shared library for Go E2E tests that execute checked-in JavaScript
# renderers. This runs only inside the repository's Debian Go tooling
# container; it never installs or discovers host Node.

ensure_node() {
  if [[ "$#" -ne 1 || -z "$1" ]]; then
    echo "ensure_node: exactly one non-empty log tag is required" >&2
    return 64
  fi
  local tag="$1"

  if command -v node >/dev/null 2>&1; then
    echo "[${tag}] node already present"
    return 0
  fi

  echo "[${tag}] node missing - installing nodejs inside the tooling container"
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends nodejs

  if ! command -v node >/dev/null 2>&1; then
    echo "[${tag}] nodejs install completed without a node executable" >&2
    return 1
  fi
  echo "[${tag}] nodejs install OK"
}