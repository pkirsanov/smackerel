# User Validation: BUG-006-006 — Learning Path Video Duration Never Parsed

## Validation Status: Accepted (automated)

## Checklist
- [x] A YouTube learning resource with `metadata.duration = "PT45M"` is estimated at 45 minutes, not the 15-minute default
- [x] Learning-path total/remaining time reflects real video durations rather than a flat 15-minute-per-video placeholder
- [x] Legacy bare-seconds duration values still produce the same estimate as before (no regression)
- [x] Missing or malformed duration still falls back to the 15-minute default without error

## Notes

This bug was surfaced by an automated `improve` quality-sweep probe and closed in
the same pass. Acceptance is evidenced by the adversarial RED→GREEN regression in
[report.md](report.md); no manual UI validation surface exists for the learning-path
time estimator (it is consumed by the digest/assistant pipeline).
