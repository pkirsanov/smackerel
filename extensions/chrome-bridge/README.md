# Smackerel Chrome Bridge

Chrome MV3 extension that captures bookmark events and browser history visits
and POSTs them to `POST /v1/connectors/extension/ingest` on the operator's
smackerel-core instance. See [`specs/058-chrome-extension-bridge/`](../../specs/058-chrome-extension-bridge/)
for the binding spec, design, and scope DoD.

## Local Development

```bash
cd extensions/chrome-bridge
npm install
npm test          # vitest unit suite
npm run typecheck # tsc --noEmit
npm run build     # esbuild → dist/extension/chrome-bridge/
```

Scope 4 (build/release wiring) adds `./smackerel.sh build --extension chrome-bridge`
which forwards to `scripts/commands/build-chrome-bridge.sh` for a deterministic,
signed zip. The local `npm run build` is for developer iteration only and is not
the release pipeline.

## Sideload (developer workflow)

1. `npm install && npm run build`
2. Chrome → `chrome://extensions` → "Developer mode" → "Load unpacked"
3. Select `dist/extension/chrome-bridge/`
4. Open the options page (`chrome://extensions` → Smackerel Chrome Bridge → "Details" → "Extension options")
5. Configure:
   - **Base URL** — your smackerel-core base URL (no trailing slash)
   - **Bearer token** — PASETO from `./smackerel.sh auth enroll --scope extension:bookmarks,extension:history`
   - **Source device id** — 1–32 chars `[a-z0-9-]`, or click "Auto" for `auto-<uuidv4>`
   - **Dedup window seconds** — integer in `[60, 86400]`
   - **Dwell threshold seconds** — integer in `[0, 3600]`
   - **Privacy allow/deny patterns** — up to 64 regex strings each
6. Click **Save**; the badge clears `SETUP` and listeners activate.

The full operator runbook (cosign verification, scoped PASETO enrollment, badge
states, revocation latency) lives in `docs/Operations.md` and is owned by Scope 5.

## Zero-Defaults Policy

The extension has **no built-in server URL, token, or device id**. Every
runtime-configurable value is operator-supplied through the options page. If
`base_url` or `bearer_token` is missing the background worker short-circuits
all event listeners and the badge displays `SETUP`. There are no fallback hosts.

## Layout

```text
extensions/chrome-bridge/
  manifest.json                  # MV3 manifest, minimum permissions
  package.json
  tsconfig.json
  esbuild.config.mjs             # bundles background + options
  vitest.config.ts
  src/
    background/
      index.ts                   # SW entrypoint, event wiring, alarms
      bookmarks.ts               # chrome.bookmarks listeners → enqueue
      history.ts                 # chrome.history listeners + dwell gate
      privacy_filter.ts          # compiled allow/deny matcher (cap 64)
      dedup_local.ts             # best-effort client-side dedup
      queue.ts                   # IndexedDB WAL, drain, badge
      transport.ts               # fetch wrapper, status → outcome
      backoff.ts                 # exponential schedule
      dwell_gate.ts              # history dwell threshold
      config.ts                  # chrome.storage.local accessor
    options/
      index.html
      index.ts                   # form, validation, persistence
    common/
      schema.ts                  # RawArtifact + Metadata typings
      uuid.ts                    # uuidv4 + uuidv7
      validation.ts              # options-page field validators
  test/
    unit/                        # vitest
```
