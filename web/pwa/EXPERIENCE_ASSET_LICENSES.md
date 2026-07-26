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

### Vendored fonts (SIL Open Font License 1.1)

The three typeface families are served as same-origin `.woff2` bytes committed
under `web/pwa/fonts/`, embedded via `web/pwa` `//go:embed fonts`, and
byte-integrity-locked by SHA-256 in the manifest exactly like every first-party
asset. They load under CSP `font-src 'self'` only — no CDN, no runtime fetch.
The trusted acquisition source is the pinned `@fontsource` npm package recorded
with `sha512` integrity in `web/pwa/package-lock.json` (the repo's lockfile
source-lock mechanism, per `bubbles-supply-chain-source-locking`).

| Asset (served path) | Real SHA-256 | Size | Trusted source (pinned) | Licence |
|---|---|---|---|---|
| `/pwa/fonts/ibm-plex-sans-latin-400-normal.woff2` | `3b646991d30055a93a4ecc499713d4347953a74a947ecab435ab72070cbdab0e` | 22588 | `@fontsource/ibm-plex-sans@5.3.0` (registry.npmjs.org) | SIL OFL 1.1 |
| `/pwa/fonts/ibm-plex-sans-latin-600-normal.woff2` | `8960851d691c054ed38e259bdcf1a6190d157b4203ed5bb32c632a863fb8ec2f` | 24252 | `@fontsource/ibm-plex-sans@5.3.0` (registry.npmjs.org) | SIL OFL 1.1 |
| `/pwa/fonts/source-serif-4-latin-400-normal.woff2` | `02194deb92d3975dd30e11a3824a1f1db32b48c93654e60560cb81ce8e7b5f95` | 20088 | `@fontsource/source-serif-4@5.3.0` (registry.npmjs.org) | SIL OFL 1.1 |
| `/pwa/fonts/source-serif-4-latin-600-normal.woff2` | `f2b7e1cf1d277b7608231868135648f8ad8e2b58d8e97ca088bee15dc357bee7` | 21532 | `@fontsource/source-serif-4@5.3.0` (registry.npmjs.org) | SIL OFL 1.1 |
| `/pwa/fonts/ibm-plex-mono-latin-400-normal.woff2` | `08949f728dc52d528e69b1667d15c89a5686a4ee9a296ff90983985f99c380f7` | 14708 | `@fontsource/ibm-plex-mono@5.3.0` (registry.npmjs.org) | SIL OFL 1.1 |

**OFL attribution.** IBM Plex Sans / IBM Plex Mono © IBM Corp. (with Reserved
Font Name "IBM Plex"). Source Serif 4 © Adobe (with Reserved Font Name "Source").
Both are distributed under the SIL Open Font License, Version 1.1. The full OFL
texts as shipped with the vendored bytes are committed verbatim at
`web/pwa/fonts/OFL-IBM-Plex.txt` and `web/pwa/fonts/OFL-Source-Serif-4.txt`.

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

The three IBM Plex / Source Serif 4 font families that previously appeared here as
`not-yet-vendored-network-required` are now **vendored same-origin** (see the
vendored-fonts table above); a real SHA-256 is asserted for each and no font is
recorded as external any longer. A browser fallback family keeping content
readable is not evidence that font delivery succeeded — the vendored `.woff2`
bytes are.
