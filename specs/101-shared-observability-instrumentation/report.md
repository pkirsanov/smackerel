# Report — Spec 101 Shared-Observability Instrumentation Contract

**Status:** in_progress (SCOPE-01 Done · SCOPE-02 Blocked/DEFERRED-to-flip)
**Workflow mode:** full-delivery (parent-expanded — the workflow runtime lacked
the `runSubagent` tool, so the analyst→design→plan→implement→test→validate roles
were executed directly in-session; this is the documented smackerel precedent,
e.g. spec 097 state.json)
**Agent:** bubbles.workflow

## Summary

Reconciled smackerel onto the knb spec-014 scope-03 shared-observability
contract. Current-truth audit found `/metrics`, the OTLP/gRPC exporter, and the
otel-go SDK already shipped; the genuine gaps (env-var naming, a fail-loud
service-tier reader, `com.bubbles.*` labels) were closed WITHOUT forking the
existing exporter (knb FINDING-014-03-1). All in-repo (SCOPE-01) work is complete
and offline-proven; the live <deploy-host> verification (SCOPE-02) is DEFERRED-to-flip
(no live host mutation this session).

## Files changed

**New:**

- `internal/observability/shared.go` — service-tier fail-loud contract reader (`Config`, `Validate`, `FromEnv`).
- `internal/observability/shared_test.go` — 9 fail-loud unit tests.
- `internal/deploy/shared_observability_labels_contract_test.go` — compose label contract (4 tests).
- `specs/101-shared-observability-instrumentation/` — spec/design/scopes/scenario-manifest/report/uservalidation/state.

**Modified:**

- `internal/config/config.go` — struct + loader: `OTELExporterEndpoint` → `OTLPTracesEndpoint` / `OTLPLogsEndpoint` / `MetricsScrapeLabelProduct`.
- `cmd/core/services.go` — `OTEL_ENABLED`-gated `observability.Config.Validate()` boot gate + import.
- `config/smackerel.yaml` — `observability:` block: 3-var contract replacing `otel_exporter_endpoint`.
- `scripts/commands/config.sh` — read (`required_value`) + emit of the 3 vars.
- `config/generated/{dev,test,self-hosted}.env` — regenerated (3-var contract).
- `docker-compose.yml` — `com.bubbles.*` labels on all 9 services.
- `deploy/compose.deploy.yml` — `com.bubbles.*` labels on all 7 services.
- `docs/Operations.md` — corrected the obsoleted `otel_exporter_endpoint` reference.
- `tests/e2e/assistant_regression_e2e_test.sh` — corrected the obsoleted `OTEL_EXPORTER_ENDPOINT` echo.

## Test Evidence

<a id="unit-observability"></a>### T1 / SCN-101-A01 — `internal/observability` unit tests (fail-loud)

`./smackerel.sh test unit --go --go-run 'Validate_Accepts|Validate_Rejects|FromLookup_|Constants_MatchKnbCanonicalNames' --verbose`:

```
--- PASS: TestValidate_AcceptsAllNonEmpty (0.00s)
--- PASS: TestValidate_RejectsEmptyTracesEndpoint (0.00s)
--- PASS: TestValidate_RejectsEmptyLogsEndpoint (0.00s)
--- PASS: TestValidate_RejectsEmptyScrapeLabelProduct (0.00s)
--- PASS: TestValidate_RejectsWhitespaceOnly (0.00s)
--- PASS: TestFromLookup_AllPresentSucceeds (0.00s)
--- PASS: TestFromLookup_MissingOneFailsLoud (0.00s)
--- PASS: TestFromLookup_EmptyOneFailsLoud (0.00s)
--- PASS: TestConstants_MatchKnbCanonicalNames (0.00s)
ok      github.com/smackerel/smackerel/internal/observability   0.003s
___GO_UNIT_EXIT=0___
```

The full-module compile (implicit in `go test ./...`) is GREEN, proving the
`internal/config/config.go` field migration and the `cmd/core/services.go` boot
gate build cleanly; the `internal/config` validation tests also ran GREEN.

<a id="a01-fail-loud"></a>### SCN-101-A01 / T3 — fail-loud contract

`observability.Config.Validate()` rejects unset / empty / whitespace-only on each
of the 3 vars with a named error (see the 6 `Rejects*` / `*FailsLoud` PASS lines
above). The boot gate `cmd/core/services.go` runs `Validate()` only when
`cfg.OTELEnabled` — proven to compile by the GREEN module build. NO-DEFAULTS: the
generator uses `required_value` (fails on missing SST key), never a `|| default`.

<a id="unit-labels"></a><a id="a03-labels"></a>### T5 / SCN-101-A03 — `com.bubbles.*` label contract

`./smackerel.sh test unit --go --go-run 'SharedObservabilityLabels' --verbose`
(after fixing the `searxng` gap the test itself caught):

```
[go-unit] applying -run selector: SharedObservabilityLabels
--- PASS: TestSharedObservabilityLabels_DevComposeLiveFile (0.00s)
--- PASS: TestSharedObservabilityLabels_DeployComposeLiveFile (0.01s)
--- PASS: TestSharedObservabilityLabels_AdversarialMissingLabelRejected (0.00s)
--- PASS: TestSharedObservabilityLabels_CompliantSyntheticAccepted (0.00s)
EXIT=0
```

The test earned its keep: the first run FAILED
`TestSharedObservabilityLabels_DeployComposeLiveFile` with
`services.searxng missing "com.bubbles.product"/"com.bubbles.service"` — a 7th
smackerel service in `deploy/compose.deploy.yml` I had missed. Fixed, re-run GREEN.

Label counts (grep):

```
docker-compose.yml:        com.bubbles.product=9  com.bubbles.service=9
deploy/compose.deploy.yml: com.bubbles.product=7  com.bubbles.service=7
```

<a id="a02-metrics-route"></a>### SCN-101-A02 (route side) — `/metrics` reused

`internal/api/router.go` L62: `r.Handle("/metrics", metrics.Handler())` →
`internal/metrics/metrics.go` `promhttp.Handler()` (existing `smackerel_*` metric
set). Existing coverage: `internal/api/health_test.go` (`GET /metrics`). This spec
REUSES the route + handler (knb FINDING-014-03-1: no duplicate exporter). The LIVE
200 is SCOPE-02 (deferred).

<a id="config-generate"></a>### Contract migration — `config generate`

`./smackerel.sh config generate` (+ `--env test` / `--env self-hosted`) → exit 0.
Each generated env now carries the 3-var contract, `OTEL_EXPORTER_ENDPOINT` gone:

```
--- dev.env / test.env / self-hosted.env (identical block) ---
402:OTEL_ENABLED=false
403:OTLP_TRACES_ENDPOINT=
404:OTLP_LOGS_ENDPOINT=
405:METRICS_SCRAPE_LABEL_PRODUCT=smackerel
old OTEL_EXPORTER_ENDPOINT count: 0
```

<a id="t2-pii"></a>### T2 — PII scan (added lines)

`git diff HEAD` over all changed source files, added lines scanned for
`ts.net` / tailnet-100.x / `<deploy-host>` / `/home/<user>` / raw IPs (excluding
loopback):

```
PII_HITS=0
```

Only env-var NAMES, empty placeholders, `smackerel`, and the generic in-cluster
DNS example `otel-collector:4317` appear (spec 014 FR-013 / AC-009 satisfied).

<a id="quality"></a>### Build Quality Gate

```
./smackerel.sh format  → 69 files left unchanged   (my Go files already gofmt-clean)
./smackerel.sh lint    → All checks passed!         (go vet ./... + web validation)
bash -n scripts/commands/config.sh → BASH_N_OK
```

`git status -- '*.go'` after format confirmed only the four spec-101 Go files were
touched — no foreign file was reformatted.

<a id="code-diff-evidence"></a>### Code Diff Evidence

`git diff --stat HEAD` over the changed runtime/source/config surface (the new
spec artifacts + regenerated generated env are separate):

```
 cmd/core/services.go                       | 26 ++++++++++++++++++++++++++
 config/smackerel.yaml                      | 14 +++++++++++++-
 deploy/compose.deploy.yml                  | 17 +++++++++++++++++
 docker-compose.yml                         | 21 +++++++++++++++++++++
 docs/Operations.md                         |  9 ++++++++-
 internal/config/config.go                  | 26 +++++++++++++++++++++-----
 scripts/commands/config.sh                 | 18 ++++++++++++++----
 tests/e2e/assistant_regression_e2e_test.sh |  3 ++-
 8 files changed, 122 insertions(+), 12 deletions(-)
```

New files (`git status -s`): `internal/observability/shared.go` +
`shared_test.go`, `internal/deploy/shared_observability_labels_contract_test.go`,
and `specs/101-shared-observability-instrumentation/`. The change touches real
runtime/source/config paths (`cmd/core/services.go`, `internal/config/config.go`,
`scripts/commands/config.sh`, both compose files) — not artifacts only.

<a id="consumer-sweep"></a>### Consumer Impact Sweep — config-surface rename

This scope renames one config surface across three representations. Every live
consumer surface was enumerated and reconciled; the old top-level surface had
ZERO runtime Go consumers (declared-but-not-consumed — grep-verified):

| Consumer surface | Before | After | Reconciled |
|---|---|---|---|
| `internal/config/config.go` field + loader | `OTELExporterEndpoint` | `OTLPTracesEndpoint` / `OTLPLogsEndpoint` / `MetricsScrapeLabelProduct` | migrated |
| `scripts/commands/config.sh` read + emit | `OTEL_EXPORTER_ENDPOINT` (soft fallback) | 3 vars via `required_value` | migrated |
| `config/smackerel.yaml` SST key | `otel_exporter_endpoint` | 3 keys | migrated |
| `config/generated/{dev,test,self-hosted}.env` | `OTEL_EXPORTER_ENDPOINT=` | 3 vars | regenerated (old-var count = 0 in all three) |
| `docs/Operations.md` opt-in tracing steps | old key | 3-var contract | updated |
| `tests/e2e/assistant_regression_e2e_test.sh` SCOPE-09 echo | old key | 3-var contract | updated |
| Go runtime readers of `OTELExporterEndpoint` | 0 (declared-not-consumed) | n/a | grep-verified 0 (only doc comments remain) |

Post-migration grep for the old names across `internal cmd scripts config
docker-compose.yml deploy` (excluding my own explanatory comments) = 0 hits.
Historical `done` spec reports (030 / 061) that mention the old name are
immutable historical record, NOT consumers, and are not rewritten.

## <a id="deferred-live"></a>Deferred (SCOPE-02 — DEFERRED-to-flip)

The LIVE shared-stack proofs are operator/apply-gated and were NOT run (the
acceptance constraints forbid live <deploy-host> mutation; accepted precedent:
WanderAide scope-05 T5/T7, knb scope-03):

| Item | Why deferred | Unblock condition |
|---|---|---|
| T4 — live `/metrics` 200 scraped by shared Prometheus | requires the shared stack + a running smackerel-core under the shared posture on <deploy-host> | operator sets smackerel `sharedServices.observability: shared` in the knb adapter params + runs `apply-shared-obs` on <deploy-host> |
| OTLP spans land in shared Tempo | same | same |
| `product=smackerel` visible in shared Prometheus target set | same | same |

These stay unchecked with the precise reason (anti-fabrication). No live host
mutation, no ssh to <deploy-host>, occurred.

## Completion Statement

SCOPE-01 (in-repo instrumentation contract) is complete and offline-proven: the
3-var contract is adopted across the SST, the fail-loud reader + `OTEL_ENABLED`-
gated boot gate are wired and unit-proven, the `com.bubbles.*` labels are on all
16 service blocks (9 dev + 7 deploy) and contract-locked, the existing `/metrics`
- OTLP exporter are reused (not forked), and gofmt / go vet / config-generate /
PII are clean. SCOPE-02 (live <deploy-host> verification) is Blocked/DEFERRED-to-flip
with a documented unblock condition. No DoD item is marked done without real
captured evidence.

**Verdict:** smackerel is **instrumentation-complete and flip-ready pending live
verification** — the only remaining work is the operator flip + live scrape proof
(SCOPE-02), which cannot be run from this session without live host mutation.
