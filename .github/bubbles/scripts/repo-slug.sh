#!/usr/bin/env bash
# Canonical repository basename to safe Bubbles repository slug conversion.

[[ -n "${_BUBBLES_REPO_SLUG_SOURCED:-}" ]] && return 0
_BUBBLES_REPO_SLUG_SOURCED=1

bubbles_repo_slug_of() {
  local name="$1"
  local slug=""

  [[ -n "$name" && "$name" != */* && "${#name}" -le 255 ]] || return 1
  slug="$(printf '%s' "$name" \
    | LC_ALL=C tr '[:upper:]' '[:lower:]' \
    | LC_ALL=C sed -e 's/[^a-z0-9]/-/g' -e 's/--*/-/g' -e 's/^-//' -e 's/-$//')"
  [[ -n "$slug" ]] || slug="repo"
  [[ "${#slug}" -le 128 && "$slug" =~ ^[a-z0-9][a-z0-9-]*$ ]] || return 1
  printf '%s' "$slug"
}

bubbles_repository_alias_from_root() {
  local root="$1"
  local canonical_root=""
  local basename_value=""

  [[ "$root" == /* && -d "$root" ]] || return 1
  canonical_root="$(cd -P -- "$root" && pwd -P)" || return 1
  basename_value="$(basename -- "$canonical_root")"
  bubbles_repo_slug_of "$basename_value"
}
