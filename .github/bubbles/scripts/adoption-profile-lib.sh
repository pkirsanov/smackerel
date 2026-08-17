#!/usr/bin/env bash
# adoption-profile-lib.sh — one parser for adoption profiles.
#
# IMP-042 SCOPE-13 / REG-12.
#
# adoption_profile_ids() was defined four times (cli.sh, developer-profile.sh,
# generate-release-manifest.sh, repo-readiness.sh), active_adoption_profile()
# three times, and adoption_profile_is_explicit() twice. The cli.sh and
# developer-profile.sh copies of the first were byte-identical, so three of the
# four were pure duplication.
#
# The copies had already drifted. cli.sh's active_adoption_profile piped through
# `head -1` and developer-profile.sh's did not, so a config carrying more than
# one adoptionProfile key produced one answer in the CLI and a different,
# multi-line answer in the profile tool. The `head -1` form is canonical here:
# the first declaration wins, and callers get a single value.
#
# Source this file; do not execute it.

# Registry parse: emit every profile id declared under `profiles:`.
bubbles_adoption_profile_ids() {
  local registry_file="$1"

  [[ -f "$registry_file" ]] || return 0

  awk '
    /^profiles:/ { in_profiles=1; next }
    in_profiles && /^  [A-Za-z0-9_-]+:$/ {
      profile=$1
      sub(":$", "", profile)
      print profile
    }
  ' "$registry_file"
}

# Config read: the repository's declared adoption profile, or empty when the
# config is absent or declares none.
bubbles_active_adoption_profile() {
  local config_file="$1"

  [[ -f "$config_file" ]] || return 0

  grep -oE '"adoptionProfile"[[:space:]]*:[[:space:]]*"[^"]+"' "$config_file" 2>/dev/null |
    head -1 |
    sed -E 's/.*"([^"]+)"$/\1/'
}

# Whether the repository declared a profile at all, as opposed to inheriting a
# framework default. An empty declared value still counts as explicit.
bubbles_adoption_profile_is_explicit() {
  local config_file="$1"

  [[ -f "$config_file" ]] && grep -q '"adoptionProfile"' "$config_file"
}

# Unknown-value policy, in one place. An unrecognised profile is a caller error,
# never a silent fallback to a default: a repository that names a profile the
# registry does not define has a real misconfiguration, and quietly substituting
# `delivery` would hide it behind behaviour the operator never asked for.
bubbles_adoption_profile_is_known() {
  local profile="$1"
  local registry_file="$2"
  local candidate

  [[ -n "$profile" ]] || return 1

  while IFS= read -r candidate; do
    [[ "$candidate" == "$profile" ]] && return 0
  done < <(bubbles_adoption_profile_ids "$registry_file")

  return 1
}
