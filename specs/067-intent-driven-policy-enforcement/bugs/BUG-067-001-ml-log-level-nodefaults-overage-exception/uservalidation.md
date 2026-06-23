# User Validation: BUG-067-001

This checklist is checked-by-default; each item is confirmed by the evidence in [report.md](report.md).

## Checklist

- [x] The ML sidecar log level is an SST value (`config/smackerel.yaml services.ml.log_level`), not a literal default in source.
- [x] `./smackerel.sh config generate` emits `ML_LOG_LEVEL` into `config/generated/dev.env` and `config/generated/test.env` (and the single emission point feeds the deploy bundle).
- [x] `ml/app/main.py` reads `ML_LOG_LEVEL` fail-loud (no `os.environ.get(..., "INFO")`) and carries no `# smackerel:policy-exception` markers.
- [x] `policy-exception-baseline.json` no longer contains the over-age `G067-A05-ml-log-level` exception.
- [x] The policy guard tests validate the REAL committed baseline at the REAL 90-day SST cap; the adversarial regression flags a future over-age exception and is RED if the bug is reintroduced.
- [x] No live stack was required; verification ran via file-system policy tests, ml Python unit tests, and `./smackerel.sh config generate` / `check` / `lint` / `format`.

## Sign-off
- Validated by: bubbles.workflow (parent-expanded `bugfix-fastlane`, validate phase) on 2026-06-22.
- Outcome: all acceptance criteria met; RED→GREEN adversarial proof captured in report.md.
