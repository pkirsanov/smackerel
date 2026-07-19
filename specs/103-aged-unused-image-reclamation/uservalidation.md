# User Validation 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)

**Feature:** `103-aged-unused-image-reclamation`
**Owner:** smackerel

> **Baseline checklist (authoring pass).** These items are checked `- [x]` as the *acceptance
> contract the implementation pass must uphold*. They are validated for real during the
> implementation/validation passes against `./smackerel.sh --env dev clean …` output. If any
> item regresses during delivery, it MUST be unchecked and the regression fixed before the
> spec can advance.

---

## Checklist

- [x] `DRY_RUN=true ./smackerel.sh --env dev clean smart` (dev plane) previews the exact planned command including `until=<N>h`, the project owner-label filter (`--filter label=io.smackerel.lifecycle.owner=smackerel`), and the env=prod exclusion (`--filter label!=io.smackerel.environment=prod`) — and changes nothing.
- [x] Orphaned smackerel image versions older than `unused_image_min_age_hours` are reclaimed on a normal `clean smart` (reclaimed space appears in the summary), after the existing volume-preserving teardown; images newer than the age are preserved.
- [x] Peer-product images (no `io.smackerel.lifecycle.owner=smackerel` label) are never referenced or removed under project scope.
- [x] The stage NEVER references or prunes volumes (`docker volume` / `--volumes` absent); the persistent postgres/pgvector and NATS jetstream data volumes are structurally out of reach.
- [x] The stage NEVER references or prunes containers (`docker container prune` / `docker rm` absent); a running container's image is never removed.
- [x] On a non-dev plane (`SMACKEREL_ENV=production`, e.g. a `runtime.environment: production` config under `--env dev`), `assert_dev_plane` refuses (non-zero) and no `docker image prune` runs.
- [x] Setting `cleanup.remove_unused_images = false` skips the stage and logs "Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)".
- [x] A missing SST key (`remove_unused_images` / `unused_image_min_age_hours` / `unused_image_scope`) aborts config generation non-zero with `Missing config key: cleanup.<key>`; an invalid value aborts with a validation message (smackerel-no-defaults).
- [x] `clean full`, `clean status`, and `clean measure` behave byte-for-byte as before; the new stage runs in NONE of them.
- [x] `Dockerfile` (core stage) and `ml/Dockerfile` (runtime stage) each carry `LABEL io.smackerel.lifecycle.owner="smackerel"` (the label-add prerequisite — smackerel is the "else" branch), and the helper constant matches the literal.
- [x] `./smackerel.sh --env dev clean test` runs the cleanup unit harness green with full unfiltered output (terminal-discipline); all operations via `./smackerel.sh`.
- [x] The 3 new keys live under `cleanup:` in `config/smackerel.yaml` (SST); the generated `config/generated/*.env` carries `CLEANUP_*` and is never hand-edited.

---

## Notes

- This is developer/CI build-tooling; it exchanges no business data (protobuf-only rule N/A)
  and has no app/web/dashboard/service runtime surface.
- The label finding (smackerel images do NOT carry an owner label) means this spec takes the
  **"else" branch**: a label-add prerequisite scope stamps
  `io.smackerel.lifecycle.owner=smackerel` on both images, and the `smackerel-*` name fallback
  is documented as explicitly-optional transitional coverage — the genuine structural
  difference from the WanderAide 162 / GuestHost 152 / QuantitativeFinance 096 references
  (whose owner labels already existed).
