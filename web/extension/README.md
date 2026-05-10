# Smackerel Browser Extension

Capture pages and selected text from the browser into a self-hosted
Smackerel instance.

## Supported browsers

- Chrome / Chromium / Edge — Manifest V3 (`manifest.json`).
- Firefox — Manifest V2 (`manifest-firefox.json`).

## Installation (development)

1. Build / start a Smackerel instance with `./smackerel.sh up`.
2. Open the browser's extension page (`chrome://extensions`,
   `edge://extensions`, or `about:debugging`).
3. Enable Developer Mode and load the unpacked
   `web/extension/` folder.
4. Click the Smackerel toolbar icon to open the popup; click
   **Settings** → enter the server URL and an auth token.

## Auth token formats

Spec 044 introduces per-user PASETO bearer auth. The extension storage
slot `authToken` accepts EITHER format with no client-side change —
the value is forwarded verbatim as
`Authorization: Bearer <authToken>` and the Go core picks the
validation branch:

| Token format | When | How to obtain |
|--------------|------|---------------|
| Per-user PASETO v4.public | Production | `./smackerel.sh auth enroll <user_id>` — paste the wire token shown once into the extension setup screen. |
| Shared dev token | Development / test | The value of the `SMACKEREL_AUTH_TOKEN` env var (or whatever the operator set in `config/smackerel.yaml`). |

The wire token is shown exactly once during enrollment. If you lose
it, rotate via the admin UI at `/admin/auth/tokens` (also delivered in
spec 044 Scope 03) or via
`./smackerel.sh auth rotate <user_id> <prior_token_id>` and paste the
fresh wire token into the extension setup screen.

The extension stores the token in
`chrome.storage.local` (`browser.storage.local` on Firefox). The
storage scope is per-profile and per-extension; it is NOT synced to
other browsers and NOT readable by web pages. If the
`Authorization` header is rejected with HTTP 401 the extension shows
a **Re-authenticate** prompt that reopens the setup screen.

## Files

- `manifest.json` / `manifest-firefox.json` — extension manifests.
- `background.js` — service worker; offline queue, capture POST,
  context-menu wiring.
- `popup/` — toolbar popup UI (setup + capture).
- `content_scripts/` — selection-capture content script.
- `lib/` — shared helpers (storage, queue, fetch).
- `icons/` — toolbar + permissions UI assets.

## Development

The extension has no build step. Edit a file and reload the unpacked
extension on the browser's extension page.

To exercise the extension end-to-end against the live test stack:

```bash
./smackerel.sh test integration --go-run '^TestExtensionAuth_'
```

These integration tests live in `tests/integration/auth_extension_test.go`
and prove the per-user PASETO bearer flow round-trips through the
real production-mode router.

## Spec reference

- Spec 044 Scope 03 — extension surface (per-user PASETO carrier).
- Spec 033 — original capture surface contract.
