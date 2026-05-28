# User Validation: 059 Google Keep Live Sync (gkeepapi production hardening)

## Checklist

- [x] The plan wires `KEEP_GOOGLE_APP_PASSWORD` through the three-mirror Bucket-2 secret manifest established by specs 051/052 and keeps `KEEP_GOOGLE_EMAIL` on the standard non-secret env contract.
- [x] The plan implements the NATS request/reply bridge between Go core (`internal/connector/keep`) and the ML sidecar (`ml/app/keep_bridge.py` + `ml/app/main.py`) over `keep.sync.request` / `keep.sync.response` with a 120 s timeout.
- [x] The plan adds the `drift_ack_token` config field and implements the in-memory circuit-breaker state machine (closed → tripping → open → closed on token rotation) as the drift-recovery signal distinct from `warning_acknowledged`.
- [x] The plan classifies sidecar auth errors as Connect-style fatals and protocol/schema/HTTP failures as drift counts, with explicit per-mutation schema validation.
- [x] The plan adds the Prometheus counter `smackerel_keep_protocol_drift_detected_total`, the sync-duration histogram, the notes-returned counter, and structured logs correlated by `request_id`, with redaction of credential material.
- [x] The plan documents the operator workflow (initial enablement, breaker recovery, App Password rotation) in `docs/Operations.md` using only `./smackerel.sh` commands and generic placeholders, with cross-references to specs 051, 052, 054 and `docs/Deployment.md`.
- [x] The plan honors SST zero-defaults, terminal discipline, and no env-specific values in any committed artifact.
- [x] The plan resolves the three design-time open questions (poll-interval floor → 15 min; response timeout → constant; drift counter → single shared counter) inline and records spec-narrative drift for follow-up.

## User-Reported Regressions

No user-reported regressions are open for this planning phase.
