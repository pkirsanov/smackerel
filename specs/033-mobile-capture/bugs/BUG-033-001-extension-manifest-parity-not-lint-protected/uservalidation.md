# User Validation — BUG-033-001 — Extension manifest parity not lint-protected

## Checklist

This bug is a developer-facing contract test addition with adversarial
regression sub-tests. There is no end-user-visible surface to validate
manually — the live manifests were already in lockstep before this fix
(verified during the round 9 devops probe and re-verified by
`TestExtensionManifestParity_LiveFiles` after the fix).

- [x] Owner reviewed the new contract test in
      `internal/web/extension_parity_contract_test.go` (511 lines,
      `package web`).
- [x] Owner confirmed `go test -v -run TestExtensionManifestParity
      ./internal/web/...` reports 9 PASS lines in the report.
- [x] Owner confirmed the broader `go test ./internal/web/...` gate is
      green with zero regressions in the report.
- [x] Owner accepts the bug as resolved.

## Rationale For Lightweight Validation

The change is:

1. Additive — adds one new file under `internal/web/`. No manifest
   content edit, no lint-script edit, no package-extension-script edit,
   no production runtime path touched.
2. Backward-compatible — the contract assertion only fires on actual
   parity drift; existing manifest contents pass on the first run
   (`TestExtensionManifestParity_LiveFiles` green in the report).
3. Locked by adversarial sub-tests — 7 in-memory mutations
   (covering name, version, description, API permissions, host
   patterns, and CSP `object-src` drift directions) prove the contract
   is non-tautological and would catch the next GAP-F01- or
   GAP-F03-class regression at unit-test time.

No operator-facing behaviour changes, so no end-to-end browser
extension installation or manual UI run is required.
