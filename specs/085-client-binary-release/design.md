# Design: 085 Client Binary Release

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

> **Planning-only.** Authored to the `specs_hardened` ceiling. Every reused
> smackerel/knb primitive below is cited by a real, verified path. Proposed new
> paths (the CI client-build job, the contract `clients:` block, gradle signing
> wiring, the Lane-B lane, the pre-push block) are PROPOSED and are created
> during the downstream delivery node — NOT by this planning run.

## Current Truth (verified 2026-06-12)

smackerel already owns most of the machinery this pipeline needs. Verified
during planning:

| Primitive | Verified path | What it gives us |
|-----------|---------------|------------------|
| Flutter client (android present, ios absent) | [`clients/mobile/assistant/pubspec.yaml`](../../clients/mobile/assistant/pubspec.yaml) + `clients/mobile/assistant/android/` | The native client to build. `pubspec.yaml` declares an `ios` *plugin platform stub* but there is NO `clients/mobile/assistant/ios/` app target (confirmed: `list_dir` + `file_search ios/**` empty). → android-only. |
| CI image pipeline | [`.github/workflows/build.yml`](../../.github/workflows/build.yml) | `name: build`; triggers push→main + tags `v*` + `workflow_dispatch`; `permissions: id-token: write` (cosign keyless); `build-images` job resolves `sourceSha`, builds + pushes `smackerel-core`/`smackerel-ml`, Trivy CRITICAL/HIGH gate, `sigstore/cosign-installer`. The client-build job composes here. |
| cosign-keyless + Rekor + SBOM + SLSA | `build.yml` (cosign-installer, `provenance: true`, `sbom: true`) + [`deploy/contract.yaml`](../../deploy/contract.yaml) `signing:` (`scheme: cosign-keyless`, `transparencyLog: rekor`, `attestations: [sbom, provenance]`) | The SAME signing scheme the client artifacts reuse. |
| Deploy contract | [`deploy/contract.yaml`](../../deploy/contract.yaml) | `contractVersion: 1`; `images:`, `externalImages:`, `configBundles:`, `signing:`, `rolloutStrategies:`, `sstKeyCatalog:`. **No `clients:` group yet** → Scope 01 adds it. |
| Contract-drift lockstep tests | `internal/deploy/external_images_contract_test.go` (cited in `deploy/contract.yaml` `externalImages:` note) | The Go-contract-test pattern to copy for a `clients:`-block drift test. |
| Build manifest (consumer side) | `deploy/contract.yaml` `configBundles.manifestSchema` (`refField: ref`, `hashField: sha256`) + `docs/Deployment.md` (Build-Once Deploy-Many) | `build-manifest-<sourceSha>.yaml` is the CI-produced artifact the `clients.artifacts[]` block is appended to. |
| Release trains + flag bundles | [`config/release-trains.yaml`](../../config/release-trains.yaml) (`mvp`→self-hosted, `next`→staging) + [`config/feature-flags.mvp.yaml`](../../config/feature-flags.mvp.yaml) + [`config/feature-flags.next.yaml`](../../config/feature-flags.next.yaml) | Where `clientReleaseLaneB: false` is declared in BOTH bundles. Format: `flags: { <name>: <bool> }` + `metadata`. |
| Flag guard | `.github/bubbles/scripts/release-train-guard.sh` (framework — read-only) | Check 8 (G111): a flag is a violation only if default-ON in a train OTHER than the spec's `releaseTrain`. OFF-in-all-trains (dormant) PASSES. |
| Pre-push downstream-sync precedent | [`scripts/git-hooks/pre-push`](../../scripts/git-hooks/pre-push) | `resolve_knb_checkout()` (→ `$KNB_REPO_ROOT` or `$HOME/knb`) then `bash "$_knb_root/scripts/lint/deploy-cli-uniformity.sh"`; WARN-if-missing; no bypass. The client-binary gate block is authored alongside this, IDENTICAL shape. |
| knb self-hosted adapter (Lane A consumer) | `smackerel.sh` `deploy-target` → `$KNB_REPO_ROOT/scripts/deploy/orchestrator.sh` (verified lines ~1873–1881) | Lane A delivery is the knb adapter's job (knb node n10). smackerel does NOT build clients or call the lib directly. |

### knb pattern (conformance source) — verified

| knb artifact | Verified path | Contract for smackerel |
|---|---|---|
| Conformance gate | [`knb/scripts/lint/client-binary-conformance.sh`](../../../knb/scripts/lint/client-binary-conformance.sh) | `--repo <path>` runs checks a–f; `E025-CLIENT-*` exit codes (91 source-no-contract, 92 manifest-no-digest, 93 contract-missing, 94 raw-signing-material, 95 laneb-autosubmit); C3 no-bypass. |
| Lane-A shared lib | `knb/scripts/deploy/client-delivery-lib.sh` | knb-side; smackerel's self-hosted adapter sources it (knb node n10). smackerel NEVER calls it. |
| Manifest/contract schema | knb spec 025 FR-003/FR-006 | `clients: { none: <bool>, artifacts: [ {platform,variant,kind,ref,sha256,provenance,embeds,laneB} ] }`. |
| Binding instruction | `knb/.github/instructions/bubbles-client-binary-release.instructions.md` | Per-platform taxonomy: android → kind `aab`+`apk`, Lane A = APK sideload, Lane B = Play Store (AAB). |

---

## 1. Contract `clients:` Group (Scope 01)

Add a top-level `clients:` group to `deploy/contract.yaml` (additive;
`contractVersion` unchanged). PROPOSED shape:

```yaml
# Native client binaries this project builds. Each is pulled by digest only,
# cosign-keyless provenance-signed, and pinned in build-manifest-<sha>.yaml
# under clients.artifacts[]. Conforms to knb spec 025 (Client Binary Release &
# Delivery Pattern). iOS is intentionally absent (no clients/mobile/assistant/ios
# target); reserved for a future spec.
clients:
  artifacts:
  - platform: android
    variant: "-"            # single-variant platform
    kind: [aab, apk]        # AAB for Lane B (Play); APK for Lane A (sideload)
    ref: ghcr.io/pkirsanov/smackerel-clients   # OCI artifact repo; digest at deploy time
    provenance: cosign-keyless
    embeds: []              # ios would embed [watchos]; android embeds nothing
    laneB: false            # Lane B (Play Store) default-OFF; gated by clientReleaseLaneB
```

A Go contract-drift test (mirroring `external_images_contract_test.go`) asserts
the android entry's shape and fails the build if it regresses.

> **knb gate interaction (OQ-1):** the knb gate's `detect_client_source()` only
> globs repo-root `pubspec.yaml`+`android/` (plus `mobile-app/`,
> `roku-app/manifest`). smackerel's client is at `clients/mobile/assistant/`, so
> the gate does NOT auto-detect it. The voluntary `clients:` android declaration
> above is what anchors conformance: with it present and non-`none`, the gate
> derives `contracted_keys = "android"` and enforces check (c) on the manifest.

## 2. CI Client-Build Job (Scope 02)

A new job (e.g. `build-clients`) in `build.yml` that **composes** with the
existing `build-images` job (reuses its `sourceSha` output) and stops at
registry push (no SSH, no apply). PROPOSED pipeline:

1. **Resolve sourceSha** from `build-images` output (same `sourceSha` as the
   images — single source of truth for the manifest).
2. **Set up Flutter** (pinned SDK version from `pubspec.yaml` `environment.flutter`).
3. **Reproducible build** (FR-CBR-003 / NFR-CBR-005): derive the version code +
   name from `sourceSha`; export `SOURCE_DATE_EPOCH` from the commit timestamp
   (`git show -s --format=%ct`); pass `--build-number`/`--build-name` from
   commit-derived inputs; no wall-clock.
4. **Build** `flutter build appbundle --release` (AAB) and `flutter build apk
   --release` (APK) in `clients/mobile/assistant`.
5. **Distribution sign** (Scope 03): gradle `signingConfigs.release` reads the
   keystore + passwords from env; the keystore is base64-decoded from a GitHub
   secret into a runner-tmp path (never committed; removed at job end).
6. **cosign-keyless provenance sign** (FR-CBR-004): `cosign sign-blob` / OCI
   attach over each artifact (Rekor-logged), reusing `id-token: write` +
   `sigstore/cosign-installer` already in `build.yml`.
7. **Push by digest** (FR-CBR-005): push the AAB + APK to
   `ghcr.io/pkirsanov/smackerel-clients` as OCI artifacts; capture each `sha256`.
8. **Emit manifest block** (FR-CBR-006): append a `clients.artifacts[]` android
   entry (platform/variant/kind/ref/sha256/provenance/embeds/laneB) to
   `build-manifest-<sourceSha>.yaml` for the SAME `sourceSha`.

```
build.yml
├── build-images   (existing: core + ml → digests → manifest images[])
└── build-clients  (NEW: needs build-images.sourceSha)
        flutter build aab+apk → gradle release-sign (env keystore)
        → cosign-keyless sign → push to ghcr smackerel-clients by digest
        → append clients.artifacts[] to build-manifest-<sourceSha>.yaml
   (STOPS at registry push — no SSH, no apply; build.yml trust boundary)
```

> **Manifest emission ordering:** `build-clients` must merge its
> `clients.artifacts[]` into the SAME `build-manifest-<sourceSha>.yaml` the
> images job writes (single manifest per sourceSha). Delivery decides whether
> that is a manifest-merge step or a single manifest-writer job that consumes
> both jobs' outputs; either way the manifest carries `images[]` AND
> `clients.artifacts[]` for one `sourceSha`.

## 3. Reproducibility Model (FR-CBR-003 / NFR-CBR-005)

| Non-deterministic input | Deterministic replacement |
|---|---|
| Wall-clock build timestamp | `SOURCE_DATE_EPOCH` = commit timestamp (`git show -s --format=%ct $sourceSha`) |
| Auto-incrementing version code | Commit-derived (`sourceSha`-based) version code + name |
| Toolchain drift | Pinned Flutter SDK (`pubspec.yaml` `environment.flutter`) + pinned Gradle/AGP |
| Build-path leakage | Stable working dir + `--no-build-id` style flags where applicable |

Acceptance (SC-003): two CI runs on the same `sourceSha` → byte-identical AAB +
APK → identical `sha256` in the manifest.

## 4. Signing Model — two independent signatures

| Signature | Purpose | Material | Where |
|---|---|---|---|
| **cosign-keyless provenance** | supply-chain authenticity (Rekor-logged); knb gate check (c) requires `provenance: cosign-keyless` | OIDC (no key material); always-on | CI (`build-clients`) |
| **Android APK/AAB distribution signature** | makes the artifact installable / Play-acceptable | operator-private upload keystore + passwords (GitHub secrets) | gradle `signingConfigs.release`, env-ref only |

These are orthogonal: cosign proves *who built it*; the Android signature makes
it *installable*. FR-CBR-007 governs the distribution material:

```gradle
// PROPOSED — clients/mobile/assistant/android/app/build.gradle(.kts)
signingConfigs {
    release {
        storeFile file(System.getenv("ANDROID_KEYSTORE_PATH"))   // runner-tmp path, never committed
        storePassword System.getenv("ANDROID_KEYSTORE_PASSWORD") // GitHub secret
        keyAlias System.getenv("ANDROID_KEY_ALIAS")
        keyPassword System.getenv("ANDROID_KEY_PASSWORD")
    }
}
```

NO `storePassword "literal"`, NO committed `.jks/.keystore/.p12/.p8`. knb gate
check (e) refuses either. (OQ-5: whether the Lane-A sideload APK uses a separate
sideload key or the same upload key — operator decision; both stay env-ref.)

## 5. Lane B — coded, default-OFF, flag-gated (Scope 04)

Lane B (Play Store submission) is fully coded but dormant:

- A submission lane (fastlane `supply`, or a `client-release-laneb` workflow job)
  uploads the AAB to the Play Store.
- It is guarded by `clientReleaseLaneB` read from an env var with **no fallback
  default** (missing ⇒ fail-fast; NFR-CBR-001). It does not run when the flag is OFF.
- The literal string `clientReleaseLaneB` MUST appear in the same file as the
  submit action so knb gate check (f) sees the guard and does not flag it.
- The flag is declared `false` in BOTH `config/feature-flags.mvp.yaml` and
  `config/feature-flags.next.yaml` (bubbles.train-owned; edit deferred to
  delivery). G111 passes because the flag is never default-ON in a non-owning
  train; dormant-everywhere is permitted (verified in `release-train-guard.sh`
  Check 8).

> **knb gate interaction (OQ-2 — BLOCKING):** the gate's `scan_laneb_autosubmit()`
> greps the WHOLE `--repo` tree for the bare tokens `upload_to_play_store`,
> `upload_to_app_store`, `app_store_connect`, `appstoreconnect`,
> `channel-store-submit`, `roku_channel_publish`, `supply`, `pilot`, `deliver`,
> and flags any matching file lacking the literal `clientReleaseLaneB`.
> smackerel's tree has 60+ legitimate `deliver`/`supply` matches (Intelligence
> Delivery, digest/CalDAV/notification **delivery**, supply-chain docs). So
> `--repo "<smackerel-root>"` would emit many false-positive
> `E025-CLIENT-LANEB-AUTO-SUBMIT`. smackerel CANNOT fix this (knb-owned gate; C3
> forbids a scope/bypass flag). Routed upstream as OQ-2; FR-CBR-011's pre-push
> `--repo` wiring is blocked until knb anchors check (f) to client-release
> contexts or word-boundaries.

## 6. Lane A — delivered by the existing knb adapter (FR-CBR-010/012)

Lane A is NOT smackerel's code. smackerel's self-hosted deployment already routes
through the knb adapter (`smackerel.sh deploy-target` →
`$KNB_REPO_ROOT/scripts/deploy/orchestrator.sh`). The knb adapter (knb node n10)
sources `client-delivery-lib.sh` to: pull the android artifact by digest →
cosign-verify provenance → byte-check `sha256` → place under the serve root →
append a `client_deliver` audit line; rollback is a pure pointer-swap. The
CBR-014 customization seam (generic signed binary + knb-injected client-config,
never a per-env rebuild) is knb-owned. smackerel's only obligations are FR-CBR-001..006
(produce the signed, digest-pinned artifact + the contract declaration) and
FR-CBR-012 (never build a client in an adapter).

## 7. Pre-Push + CI Gate Wiring (Scope 05)

A SECOND block in `scripts/git-hooks/pre-push`, authored IDENTICALLY to the
existing deploy-cli-uniformity block (reusing `resolve_knb_checkout`):

```bash
# PROPOSED — alongside the existing deploy-cli-uniformity block
if _knb_root="$(resolve_knb_checkout)"; then
  _cbr_gate="$_knb_root/scripts/lint/client-binary-conformance.sh"
  if [ -x "$_cbr_gate" ]; then
    bash "$_cbr_gate" --repo "$(git rev-parse --show-toplevel)" || exit $?
  else
    printf '%s\n' "[F025-PRE-PUSH-MISSING-GATE] client-binary-conformance gate not executable at $_cbr_gate" >&2
    exit 1
  fi
else
  printf '%s\n' "[WARN] knb client-binary-conformance gate SKIPPED: no knb checkout found." >&2
fi
```

Plus a CI safety-net workflow step that runs the same gate (mirrors the knb CI
drift safety net). **Blocked on OQ-2** — until the knb gate stops false-positiving
on bare `deliver`/`supply`, the `--repo` invocation cannot run clean against the
smackerel tree. The wiring is authored now (delivery) but its green-run DoD item
is gated on the upstream fix; the scope records this dependency explicitly.

## 8. Conformance Self-Check (how the gate sees smackerel)

| Gate check | smackerel result |
|---|---|
| (a) source detection | Does NOT fire (client at `clients/mobile/assistant/`, not repo root) — OQ-1 |
| (b) source ⇒ contract | N/A while (a) is silent; smackerel declares android voluntarily |
| (c) manifest digest fail-closed | ENFORCED in CI (manifest present): android `sha256` + `provenance: cosign-keyless` required; SKIPPED in pre-push (no manifest) |
| (d) silence fails | PASSES — smackerel declares `clients.artifacts` (android), not silence |
| (e) raw signing material | PASSES — env-ref gradle signing; nothing committed (Scope 03) |
| (f) Lane-B unguarded auto-submit | smackerel's Lane-B file co-locates `clientReleaseLaneB`; BUT whole-tree bare-token false positives → OQ-2 (blocking) |

## 9. Out of Scope / Reserved

- **iOS** — no `ios/` app target; the contract schema reserves it (`embeds:
  [watchos]`) for a future spec (OQ-3).
- **knb gate / lib / manifest schema** — knb-owned; this spec composes by
  reference and routes OQ-1/OQ-2 upstream (NFR-CBR-004).
- **The knb adapter's client-delivery step** — knb node n10.
