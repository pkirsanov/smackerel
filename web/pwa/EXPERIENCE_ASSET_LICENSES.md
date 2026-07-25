# Experience Asset License Inventory — spec 106 SCOPE-106-01

Authoritative, machine-checked provenance/licence data lives in the
`ExperienceAssetManifest` (`internal/web/experience_assets.go`), asserted by
`TestExperienceAssetManifestLocksSourcesLicensesBytesTokensAndAppearanceEnums`.
This file is the human-readable companion.

## Source-locked same-origin assets (first-party)

Every asset below is served same-origin from `/pwa/...`, embedded via
`web/pwa` `//go:embed`, and byte-integrity-locked by SHA-256 in the manifest.

| Asset (served path) | Source | License |
|---|---|---|
| `/pwa/experience-tokens.css` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/experience-appearance.js` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/style.css` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/app.js` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/lib/appnav.js` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/lib/queue.js` | First-party (this repo) | Smackerel repository `LICENSE` |
| `/pwa/icon.svg` | First-party (this repo) | Smackerel repository `LICENSE` |

Icon glyphs for icon-only controls are first-party inline SVG source-locked in
`internal/web/icons` (24×24, 1.5px stroke, `currentColor`); they are compiled
into the same-origin templates, not fetched.

## External / not-yet-same-origin dependencies (honest — NOT fabricated)

These are recorded in the manifest's `ExternalDependencies` with an explicit
status; **no SHA-256 digest is asserted for them** because their same-origin
bytes are not present in this repo/environment.

| Dependency | Owner | Status | Intended licence |
|---|---|---|---|
| `htmx.org@1.9.12` | BUG-002-006 (`specs/002-phase1-foundation/bugs/BUG-002-006-...`) | `pending-same-origin-migration` — currently a pinned unpkg CDN `<script>` in `internal/web/templates.go`; BUG-002-006 owns the same-origin vendoring + digest. This manifest will reference that digest once it exists; it embeds **no second copy**. | BSD-0 / MIT (htmx) |
| `IBM Plex Sans` | spec 106 (this scope) | `not-yet-vendored-network-required` — same-origin `.woff2` byte vendoring needs a network fetch unavailable in this build environment. Family name is declared in `experience-tokens.css` with a platform fallback so a later same-origin `@font-face` is a token-value change only. | SIL OFL 1.1 |
| `Source Serif 4` | spec 106 (this scope) | `not-yet-vendored-network-required` (as above) | SIL OFL 1.1 |
| `IBM Plex Mono` | spec 106 (this scope) | `not-yet-vendored-network-required` (as above) | SIL OFL 1.1 |

Recording the font families as declared-with-fallback (rather than fabricating a
digest over bytes that are not present) is the honest posture required by the
scope: asset integrity still fails the release if a *claimed* same-origin byte is
missing, and a browser fallback family keeping content readable is not evidence
that font delivery succeeded.
