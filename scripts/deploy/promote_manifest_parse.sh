#!/usr/bin/env bash
# scripts/deploy/promote_manifest_parse.sh — build-manifest extraction helpers.
#
# Spec 082 SCOPE-082-07 — schema convergence.
#
# There are TWO build-manifest shapes in this repo and they were previously
# parseable only by two different scripts:
#
#   1. CI list shape (.github/workflows/build.yml → build-manifest-<sha>.yaml):
#        sourceSha: <sha>
#        images:
#          - name: smackerel-core
#            ref: ghcr.io/.../smackerel-core@sha256:<d>
#          - name: smackerel-ml
#            ref: ghcr.io/.../smackerel-ml@sha256:<d>
#        configBundles:
#          - env: home-lab
#            ref: ghcr.io/.../smackerel-config-bundles:home-lab-<sha>
#            sha256: <hex>
#
#   2. local-operator map/object shape (scripts/commands/build-home-lab.sh →
#      dist/local-build-manifests/local-build-manifest-<sha>.yaml):
#        sourceSha: "<sha>"
#        images:
#          smackerel-core: "ghcr.io/.../smackerel-core@sha256:<d>"
#          smackerel-ml: "ghcr.io/.../smackerel-ml@sha256:<d>"
#        configBundle:
#          ref: "ghcr.io/.../smackerel-config-bundles:home-lab-<sha>"
#          tag: "home-lab-<sha>"
#          env: "home-lab"
#          sha256: "<hex>"
#
# Before SCOPE-082-07, the in-tree promote.sh parsed ONLY shape 1 and silently
# returned empty values for shape 2 (the local-operator manifest), so a local
# build had to be promoted via a different (knb) promote path. These helpers
# parse BOTH shapes: each extractor tries the CI list shape first and falls
# back to the local-operator object shape. The functions are factored here so
# tests/unit/cli/promote_manifest_parse_test.sh can drive them against both
# fixture shapes (DoD D07-2).
#
# This file is a pure function library: it sets NO `set -e`, defines only
# functions, and is meant to be sourced.

# manifest_source_sha <manifest> — prints the top-level sourceSha (both shapes
# share this field; quotes are stripped).
manifest_source_sha() {
  awk '/^sourceSha:/ { sub(/^[^:]+:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }' "$1"
}

# manifest_image_ref <manifest> <image-name> — prints the full image ref
# (repository@sha256:<digest>) for smackerel-core or smackerel-ml, trying the
# CI list shape then the local-operator map shape.
manifest_image_ref() {
  local manifest="$1" name="$2" ref
  # CI list shape: `- name: <name>` then the following `ref:`.
  ref="$(awk -v n="$name" '
    $0 ~ "^[[:space:]]*- name: "n"[[:space:]]*$" { found=1; next }
    found && /^[[:space:]]*ref:/ { sub(/^[^:]+:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$manifest")"
  if [[ -z "$ref" ]]; then
    # local-operator map shape: `  <name>: "<ref>"` under `images:`.
    ref="$(awk -v n="$name" '
      $0 ~ "^[[:space:]]+"n":[[:space:]]*" { sub(/^[^:]+:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
    ' "$manifest")"
  fi
  printf '%s' "$ref"
}

# _manifest_bundle_object_field <manifest> <field> — internal: prints a field
# from the singular `configBundle:` object block (local-operator shape).
_manifest_bundle_object_field() {
  awk -v f="$2" '
    /^configBundle:[[:space:]]*$/ { inblock=1; next }
    inblock && /^[^[:space:]#]/  { inblock=0 }
    inblock && $0 ~ "^[[:space:]]+"f":" { sub(/^[^:]+:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$1"
}

# manifest_bundle_field <manifest> <env> <field(ref|sha256)> — prints the
# config-bundle ref or sha256 for the requested environment, trying the CI
# list shape (keyed by env) then the local-operator object shape (whose
# single env MUST match the requested env).
manifest_bundle_field() {
  local manifest="$1" env="$2" field="$3" val
  # CI list shape: find `- env: <env>` then the requested field.
  val="$(awk -v env="$env" -v f="$field" '
    /^[[:space:]]*- env:[[:space:]]*/ { cur=$3; gsub(/"/, "", cur); found=(cur==env); next }
    found && $0 ~ "^[[:space:]]*"f":" { sub(/^[^:]+:[[:space:]]*/, ""); gsub(/"/, ""); print; exit }
  ' "$manifest")"
  if [[ -z "$val" ]]; then
    # local-operator object shape: extract only if the block's env matches.
    local oenv
    oenv="$(_manifest_bundle_object_field "$manifest" env)"
    if [[ "$oenv" == "$env" ]]; then
      val="$(_manifest_bundle_object_field "$manifest" "$field")"
    fi
  fi
  printf '%s' "$val"
}

# manifest_bundle_ref <manifest> <env> — convenience wrapper.
manifest_bundle_ref() { manifest_bundle_field "$1" "$2" ref; }

# manifest_bundle_sha <manifest> <env> — convenience wrapper.
manifest_bundle_sha() { manifest_bundle_field "$1" "$2" sha256; }

# manifest_trust_model <manifest> — prints the manifest's trust model so the
# legacy (C4 / spec-017+018) PATH-2 promote can forward it to the adapter.
#
# Spec 021 B1a: a CI build manifest OMITS the trustModel field on purpose
# (spec-019 FR-6: ci-keyless is inferred from the absence of an explicit
# value). When the field is absent we therefore return "ci-keyless" so a CI
# manifest never silently misresolves to the adapter's stored
# params.signing.trustModel default (local-operator). An explicit value in the
# manifest (either shape; top-level key) wins. Quotes and trailing comments are
# stripped to match the other extractors in this library.
manifest_trust_model() {
  local manifest="$1" tm
  tm="$(awk '/^trustModel:/ { sub(/^[^:]+:[[:space:]]*/, ""); sub(/[[:space:]]*#.*$/, ""); gsub(/"/, ""); print; exit }' "$manifest")"
  if [[ -z "$tm" ]]; then
    tm="ci-keyless"
  fi
  printf '%s' "$tm"
}
