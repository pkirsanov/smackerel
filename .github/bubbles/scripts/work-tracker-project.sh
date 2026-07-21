#!/usr/bin/env bash
# Work-Tracker Projection Adapter (IMP-100 Phase 4 / IMP-026 SCOPE-7)
# ---------------------------------------------------------------------------
# Projects a Bubbles feature's durable state (`state.json`) into a
# PROVIDER-NEUTRAL, normalized work-item model that an external work tracker
# (Jira, GitHub Issues, Linear, …) can mirror — WITHOUT coupling Bubbles to any
# specific tracker. It is a projection, not a sync:
#
#   - specs/** stays AUTHORITATIVE: this reads `state.json` and emits stdout
#     only. It never writes back, never mutates a spec, never edits a tracker.
#   - DRY-RUN by default (the only mode shipped here): it prints the normalized
#     projection. A provider push adapter is a separate, opt-in extension that
#     CONSUMES this output and supplies its own credentials via approved secret
#     tools — this core carries no secrets and performs no network I/O.
#   - IDEMPOTENT: the projection is a pure function of `state.json` (same input →
#     byte-identical output), so a downstream mirror is a safe upsert.
#
# Normalized model (stable field names, provider-neutral):
#   { "source": "bubbles", "specRef": "<feature>", "items": [
#       { "id": "<feature>",          "type": "epic", "title": ..., "status": ..., "parent": null },
#       { "id": "<feature>/<scope>",  "type": "task", "title": ..., "status": ..., "parent": "<feature>" }, ...
#   ] }
#
# Exit 0 = projection printed / no-op. Exit 2 = usage error. Reads JSON with
# `jq`; if `jq` is not installed it WARNs and prints an empty projection (exit 0)
# rather than failing. No bypass flag; there is nothing to bypass (read-only).
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: work-tracker-project.sh --feature-dir <dir>

Projects <dir>/state.json into a provider-neutral work-item model on stdout
(dry-run, read-only, idempotent). One epic (the feature) + one task per scope.
Exit 0 = printed / no-op (no state.json, or jq missing). Exit 2 = usage error.
EOF
}

feature_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --feature-dir) feature_dir="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "work-tracker-project: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ -z "$feature_dir" ]]; then
  echo "work-tracker-project: --feature-dir is required" >&2
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "work-tracker-project: feature dir not found: $feature_dir" >&2
  exit 2
fi

spec="$(basename "$feature_dir")"
state_file="$feature_dir/state.json"

emit_empty() { printf '{ "source": "bubbles", "specRef": "%s", "items": [] }\n' "$spec"; }

if [[ ! -f "$state_file" ]]; then
  emit_empty
  exit 0
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "[work-tracker-project] WARN — jq not installed; emitting empty projection." >&2
  emit_empty
  exit 0
fi

# Pure, idempotent projection: one epic (the feature) + one task per scope.
# Scopes are read from `.scopes[]` when present; each may key its id on
# `.id`/`.name`/`.scope` and its status on `.status` (all optional, defaulted).
jq -S \
  --arg spec "$spec" \
  '{
    source: "bubbles",
    specRef: $spec,
    items: (
      [ { id: $spec, type: "epic",
          title: (.title // .feature // .name // $spec),
          status: (.status // "unknown"),
          parent: null } ]
      + [ (.scopes // [])[]
          | ( (.id // .name // .scope // "scope") | tostring ) as $sid
          | { id: ($spec + "/" + $sid), type: "task",
              title: (.name // .title // $sid),
              status: (.status // "unknown"),
              parent: $spec } ]
    )
  }' "$state_file" 2>/dev/null || {
  echo "[work-tracker-project] WARN — could not parse $state_file; emitting empty projection." >&2
  emit_empty
}
exit 0
