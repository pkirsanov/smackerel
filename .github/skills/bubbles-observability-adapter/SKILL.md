---
description: How to author and wire telemetry adapters under bubbles/adapters/observability/ — uniform 4-verb contract, none default, prometheus reference, swappable per downstream repo.
---

# bubbles-observability-adapter

Bubbles ships a uniform adapter contract for fetching live production telemetry (alerts, SLO burn, error rate, deploy impact). Mirrors the knb offsite-backup adapter pattern: declarative selection in `traceContracts.liveTelemetryEndpoints`, swappable backends under `bubbles/adapters/observability/<name>.sh`, `none` as the safe default.

## When to author a new adapter

- You run a telemetry stack (Sentry, Grafana Cloud, Datadog, New Relic, custom Prometheus federation) that the shipped `prometheus` adapter does not cover.
- You want `bubbles.retro target: framework` and `bubbles.stabilize` (incident-fastlane) to enrich diagnosis context with live signals from that stack.

## Contract (every adapter MUST implement all 4 verbs)

| Verb | Stdout | Exit 0 means | Exit 1 means |
|------|--------|--------------|--------------|
| `fetch-alerts` | JSON array of active alerts | Adapter reachable, no parse errors | Adapter unreachable / parse failure (NOT a framework failure) |
| `fetch-slo-burn` | JSON map service→burn-rate | (same) | (same) |
| `fetch-error-rate` | JSON map service→error-pct | (same) | (same) |
| `fetch-deploy-impact` | JSON map sha→regression-delta | (same) | (same) |

Unknown verbs MUST exit 1 with `unknown verb` on stderr. `-h` / `--help` MUST print a usage block and exit 0.

## File location

`bubbles/adapters/observability/<adapter-name>.sh` (chmod +x).

## Required structure

```bash
#!/usr/bin/env bash
set -euo pipefail
VERB="${1:-}"
# validate env / dependencies
case "$VERB" in
  fetch-alerts) ... ;;
  fetch-slo-burn) ... ;;
  fetch-error-rate) ... ;;
  fetch-deploy-impact) ... ;;
  -h|--help|"") usage; exit 0 ;;
  *) echo "[<name>][ERROR] unknown verb '$VERB'" >&2; exit 1 ;;
esac
```

The case statement is what `observability-adapter-lint.sh` greps for. If a verb is not listed verbatim, the lint fails.

## NO defaults for adapter inputs

Adapters that need env vars (URL, token, etc.) MUST refuse to run when the var is unset. NO `: "${VAR:=fallback}"`. Example: `prometheus.sh` requires `PROMETHEUS_BASE_URL` and exits 1 with an explicit error if it is missing.

This matches the framework-wide NO DEFAULTS policy. The operator (or knb adapter overlay) sets the env var explicitly per environment.

## Wiring

In your repo's `.github/bubbles-project.yaml` (or wherever you keep `traceContracts`):

```yaml
traceContracts:
  liveTelemetryEndpoints:
    alerts: "prometheus"
    slo-burn: "prometheus"
    error-rate: "prometheus"
    deploy-impact: "none"   # mix-and-match is fine
```

Consumers (`bubbles.retro`, `bubbles.stabilize`) resolve each verb to the named adapter at runtime.

## Verification

```bash
bash bubbles/scripts/observability-adapter-lint.sh
# Output:
# [observability-adapter-lint] OK (2 adapter(s) validated)
```

If you add a third adapter (e.g. `sentry.sh`), the lint discovers it via the `*.sh` glob. No registry update needed.

## Reference adapters shipped in v5

| Adapter | Verbs | Env vars |
|---------|-------|----------|
| `none` | all 4 return `{}` | — |
| `prometheus` | all 4 query Prometheus HTTP API | `PROMETHEUS_BASE_URL` (required), `PROMETHEUS_BEARER_TOKEN`, `PROMETHEUS_CURL_MAX_TIME`, `PROMETHEUS_QUERY_*` (optional overrides) |

## Failure semantics

Adapter exit 1 is **not** a framework failure. It means "telemetry unavailable, proceed without enrichment". Consumers MUST gracefully degrade when an adapter returns non-zero or empty. The framework continues normally.

## Anti-patterns

| Don't | Why |
|-------|-----|
| Hardcode a default URL/token in the adapter | Violates NO DEFAULTS policy — operator sets env vars |
| Return non-JSON on success | Breaks consumer parsing |
| Exit 2+ on adapter unavailable | Reserved for framework-level errors |
| Skip the `-h` / `--help` block | Lint and human discovery both expect it |
| Edit `bubbles/scripts/*` to mention a specific adapter | Adapters discovered via filesystem glob; never hard-referenced |
