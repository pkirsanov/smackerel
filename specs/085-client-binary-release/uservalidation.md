# User Validation Checklist

> Items are checked `[x]` by default (validated via the delivery effort — see
> [report.md](report.md#delivery-evidence--full-delivery-node-n6-2026-06-12)).
> All five scopes are **delivered** and the items below are now **implemented**
> (the CI-runtime steps are coded + statically verified and produce their first
> real artifact on the next CI push with operator secrets — see the report's
> CI-runtime-vs-local-proof split). The owner unchecks `[ ]` an item to report
> that the delivered behavior is wrong or missing — an unchecked item is a
> blocking, owner-reported regression that `bubbles.validate` must resolve.

## Checklist

- [x] Baseline checklist initialized for this feature
- [x] Conformance target is knb spec 025 (Client Binary Release & Delivery Pattern); composed by reference, no knb/framework file edited
- [x] Native platform scope is **android only** (Flutter app at `clients/mobile/assistant`; no `ios/` app target exists; ios reserved for a future spec)
- [x] Client binary is planned as an immutable, digest-pinned `clients.artifacts` entry in `build-manifest-<sha>.yaml` (stored in `ghcr.io/pkirsanov/smackerel-clients` by sha256)
- [x] CI cosign-keyless provenance signing is always-on per client artifact (reuses `build.yml` `id-token: write` + cosign-installer)
- [x] Builds are reproducible/deterministic (commit-derived inputs + `SOURCE_DATE_EPOCH`, no wall-clock)
- [x] Android distribution signing material stays operator-private (GitHub secrets, env-ref gradle only); no raw key file or inline password in the repo
- [x] Lane A (evo-x2 self-host) delivery is performed by the EXISTING knb home-lab adapter consuming the digest; smackerel never builds clients in an adapter and never calls the knb Lane-A lib directly
- [x] Lane B (Play Store) is CODED but default-OFF behind `clientReleaseLaneB`, never auto-runs; the literal flag guard is co-located with the submit action
- [x] `clientReleaseLaneB` is declared default-OFF (`false`) in BOTH `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml`; G111-clean (verified against `release-train-guard.sh` Check 8); bundle edit routed to `bubbles.train` during delivery
- [x] Release train set to `mvp` (Lane A targets home-lab = mvp's slot)
- [x] Pre-push + CI wire the knb `client-binary-conformance.sh` gate via `--repo`, mirroring the `deploy-cli-uniformity.sh` downstream-sync precedent; no bypass flag (C3 inherited)
- [x] NO-DEFAULTS / fail-loud SST honored (`clientReleaseLaneB` read with no fallback default)
- [x] Open questions captured + routed: OQ-1 + OQ-2 → knb spec 025 owner (gate source-detection path; check-f bare-token false positives — OQ-2 blocks Scope 05 `--repo` green-run); OQ-3 → product owner (ios); OQ-4 → bubbles.train (owning train at Lane-B activation); OQ-5 → product/devops owner (sideload vs upload key)
