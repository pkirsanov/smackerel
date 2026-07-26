# Report: SCOPE-106-04 Shared Shell Shadow Adapters And Canaries

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary
Slice 1 (shared shell shadow adapters + unit golden-parity lane) is implemented. A
renderer-neutral `ExperienceProjection` is built from the generated catalog, the
shell appearance, and the readiness-owned availability state contract, and rendered
through three SHADOW adapters (server / PWA / Card) into content-free comparison
fixtures + a deterministic projection digest. This is SHADOW mode only: no active
navigation, route, page body, or behavior is changed. The XP106-04-U unit lane
passes with current-session evidence below. The integration / e2e-api / e2e-ui /
canary / rollback lanes and the authenticated PWA-auth canary are coupled-forward
to later slices (BUG-070-001 supplies the production browser-session canary).
## Decision Record
The scope owns shadow renderer adapters and high-fan-out canaries only.
## Completion Statement
Not complete — status remains `in_progress`. Slice 1 (XP106-04-U shared shell shadow
adapters + unit golden-parity) is proven with current-session evidence; the
integration, e2e-api, e2e-ui, shared-infrastructure canary, and rollback lanes are
coupled-forward and their DoD items remain unchecked.
## Code Diff Evidence
No implementation-bearing diff is claimed.
## Test Evidence

### XP106-04-U

Unit golden-parity lane (SCN-106-003): server, PWA, and Card shadow adapters consume
ONE `ExperienceProjection` and produce identical content-free fixtures + digest;
safe DOM construction (no innerHTML); fail-closed adapter/build errors with no
optimistic fallback. Command and full captured PASS output (current session,
PII-scrubbed `<repo-root>/`):

```text
$ ./smackerel.sh test unit --go --go-run TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection --verbose
+ go test -v -run TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection -count=1 ./...
[go-unit] starting go test ./...
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/three_shadow_adapters_produce_identical_golden_fixtures
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/fixtures_emit_only_content_free_contract_markers
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/pwa_and_peers_construct_nodes_safely
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/projection_fields_match_catalog_and_owner_truth
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/operator_audience_changes_projection_and_digest
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/adapters_fail_closed_without_optimistic_fallback
=== RUN   TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/build_rejects_non_readiness_availability_and_missing_outcomes
--- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/three_shadow_adapters_produce_identical_golden_fixtures (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/fixtures_emit_only_content_free_contract_markers (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/pwa_and_peers_construct_nodes_safely (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/projection_fields_match_catalog_and_owner_truth (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/operator_audience_changes_projection_and_digest (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/adapters_fail_closed_without_optimistic_fallback (0.00s)
    --- PASS: TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection/build_rejects_non_readiness_availability_and_missing_outcomes (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/experience      0.018s
[go-unit] go test ./... finished OK
```

Implementation files: `internal/experience/renderer_projection.go` (new),
`internal/experience/renderer_projection_test.go` (new).

Supporting lanes (current session, scoped as the CLI allows):
- `./smackerel.sh check` — `Config is in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK` (17 registered, 0 rejected).
- `./smackerel.sh lint` — go vet + python-lint + `Web validation passed`; the two new files are not flagged.
- `./smackerel.sh format --check` — the two new files are gofmt-clean. One PRE-EXISTING FOREIGN file, `tests/e2e/experience_state_e2e_test.go` (a SCOPE-03 e2e test outside this slice's allowlist), is reported as unformatted and was intentionally left untouched.

Coupled-forward (unchecked, honest): XP106-04-I (integration), XP106-04-A (e2e-api),
XP106-04-W (e2e-ui), XP106-04-C (shared-infra canary), and XP106-04-R (rollback)
require the live stack in a later slice; the authenticated PWA-auth canary is gated
on BUG-070-001's unified production browser session.
## Planned Test References
**Claim Source:** not-run
Planned execution uses `./smackerel.sh`; the concrete not-yet-authored files and titles are listed in `scope.md` and root `test-plan.json` and are not execution evidence.
## Uncertainty Declarations
BUG-070 production-session evidence is an external entry gate.
## Scenario Contract Evidence
See `scenario-manifest.json` and `test-plan.json` at the spec root.
## Coverage Report
No runtime coverage is claimed.
## Lint/Quality
No scope execution quality result is claimed.
## Validation Summary
No validation or certification result is claimed.
## Audit Verdict
No audit verdict is claimed.
