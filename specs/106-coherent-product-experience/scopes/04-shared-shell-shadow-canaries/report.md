# Report: SCOPE-106-04 Shared Shell Shadow Adapters And Canaries

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary
Slice 1 (shared shell shadow adapters + unit golden-parity lane) is implemented. A
renderer-neutral `ExperienceProjection` is built from the generated catalog, the
shell appearance, and the readiness-owned availability state contract, and rendered
through three SHADOW adapters (server / PWA / Card) into content-free comparison
fixtures + a deterministic projection digest. This is SHADOW mode only: no active
navigation, route, page body, or behavior is changed. The XP106-04-U unit lane, and
— added in slice 2 — the XP106-04-I integration and XP106-04-R rollback lanes, pass
on the live stack with current-session evidence below. The e2e-api / e2e-ui /
shared-infrastructure canary lanes and the authenticated PWA-auth canary remain
coupled-forward to later slices (BUG-070-001 supplies the production browser-session
canary).
## Decision Record
The scope owns shadow renderer adapters and high-fan-out canaries only.
## Completion Statement
Not complete — status remains `in_progress`. Slice 1 (XP106-04-U shared shell shadow
adapters + unit golden-parity) and slice 2 (XP106-04-I integration + XP106-04-R
rollback, both on the live stack) are proven with current-session evidence; the
e2e-api, e2e-ui, and shared-infrastructure canary lanes are coupled-forward and their
DoD items remain unchecked.
## Code Diff Evidence
Slice 2 adds live-lane test coverage only — no production/implementation code
changed. New file `tests/integration/experience/shadow_projection_test.go`
(XP106-04-I) and one added test function
`TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation` in
`tests/integration/experience/shell_rollback_test.go` (XP106-04-R); that file's
existing SCOPE-01 asset/adapter rollback test is unchanged. Both consume the
committed `internal/experience` (and, for XP106-04-I, the real
`internal/recommendation/availability` readiness owner) exported API read-only.
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

### XP106-04-I

Integration lane (SCN-106-003): the renderer-neutral `ExperienceProjection` is built
from the REAL generated catalog (`experience.GeneratedCatalog()`), the REAL declared
audiences (`daily_user`, `operator`), and REAL owner-availability outcomes derived
from the ACTUAL readiness owner (`recommendation/availability.Determine`), then
rendered through the three shadow adapters. Per real audience it asserts: identical
surface IDs / parents / order / labels / hrefs (via the digest) / audience /
availability / action AND identical projection digest across server / PWA / Card;
SHADOW mode (content-free `data-product-navigation=shadow` fixtures, safe-DOM only,
no non-`data-*` attribute, no href/active-link mutation, no user-content text node);
real-owner truth (a real not-ready owner outcome never projects Available, a real
ready outcome never projects Unavailable); current/parent-current highlight parity;
and fail-closed with NO optimistic fallback (an adapter tamper surfaces a visible
`ShadowFailure` + non-settled fixture, and a structural non-readiness availability
signal is rejected at build). No mock, no `httptest`/`route`/`intercept` of internal
code.

Both new tests were first run on the FULL `./smackerel.sh test integration` live
stack (core + ml + postgres + nats + searxng + jaeger), focused via `--go-run`; that
lane exited `PASS: go-integration` (+ `PASS: python-integration`) and auto-tore-down
the disposable stack. The full-lane verbose per-test lines were elided by the
terminal capture's 20 KB truncation, so the fully-captured per-test `--- PASS` block
below is the projection test re-run on the lighter live postgres+nats integration
lane (the shadow projection test is pure catalog/readiness/adapter and never touches
the blanked core/ml/searxng URLs, so it runs identically). Intermediate
no-tests-to-run packages elided; the `experience` block is verbatim:

```text
$ ./smackerel.sh test integration-light --go-run 'TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover'
go-integration: applying -run selector: TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover
=== RUN   TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover
=== RUN   TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/audience_daily_user
=== RUN   TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/audience_operator
=== RUN   TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/current_highlight_and_fail_closed_operator
--- PASS: TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover (0.00s)
    --- PASS: TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/audience_daily_user (0.00s)
    --- PASS: TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/audience_operator (0.00s)
    --- PASS: TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover/current_highlight_and_fail_closed_operator (0.00s)
ok      github.com/smackerel/smackerel/tests/integration/experience     0.133s
PASS: go-integration-light
```

Implementation file: `tests/integration/experience/shadow_projection_test.go` (new).

### XP106-04-R

Rollback lane (SCN-106-003): captures an explicit baseline of current renderer
behavior (the generated-catalog route/data inventory + the user appearance
preference + the fail-closed no-fallback contract), ENABLES the three shadow adapters
(renders content-free comparison fixtures + digest), then performs the atomic
rollback (reverts the release → discards the fixtures) and asserts the baseline is
RESTORED with no route, data, or user-preference mutation; the discarded fixtures
leave NO optimistic/settled residue; the fail-closed contract still holds (no static
optimistic fallback was installed by the rollback); the generated catalog +
comparison diagnostics stay intact; and re-enabling reproduces byte-identical
fixtures + digest (an atomic, byte-exactly reversible swap — no drift). Adversarial
(non-tautological): the after-rollback fail-closed assertion would FAIL if a static
optimistic fallback were installed, and the digest-reversibility assertion would FAIL
if the comparison diagnostic drifted. It drives the real `experience.GeneratedCatalog`
/ `BuildExperienceProjection` / shadow adapters with no mock, no stub, no
interception; the existing SCOPE-01 asset/adapter rollback test in the same file is
unchanged.

Re-run standalone on the lighter live postgres+nats integration lane (the same two
tests also passed together on the full `./smackerel.sh test integration` stack →
`PASS: go-integration`). Intermediate no-tests-to-run packages elided; the
rollback + `experience` block is verbatim:

```text
$ ./smackerel.sh test integration-light --go-run 'TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation'
go-integration: applying -run selector: TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation
ok      github.com/smackerel/smackerel/tests/integration        0.139s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/api    0.035s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/db     0.015s [no tests to run]
ok      github.com/smackerel/smackerel/tests/integration/drive  0.162s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
=== RUN   TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation
--- PASS: TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/experience     0.154s
PASS: go-integration-light
```

Implementation: one added test function in
`tests/integration/experience/shell_rollback_test.go` (XP106-04-R); the file's
existing SCOPE-01 rollback test is unchanged.

Supporting lanes (slice 2, current session): `./smackerel.sh check` → OK (`Config is
in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK` — 17 registered, 0
rejected); `gofmt -l` on the two touched files prints nothing (gofmt-clean).

Coupled-forward (unchecked, honest): XP106-04-A (e2e-api), XP106-04-W (e2e-ui), and
XP106-04-C (shared-infra canary) require real browser routes / the live PWA DOM in a
later slice; the authenticated PWA-auth canary is additionally gated on BUG-070-001's
unified production browser session.
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
