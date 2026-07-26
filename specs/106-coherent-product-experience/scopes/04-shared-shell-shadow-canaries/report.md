# Report: SCOPE-106-04 Shared Shell Shadow Adapters And Canaries

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary
Slice 1 (shared shell shadow adapters + unit golden-parity lane) is implemented. A
renderer-neutral `ExperienceProjection` is built from the generated catalog, the
shell appearance, and the readiness-owned availability state contract, and rendered
through three SHADOW adapters (server / PWA / Card) into content-free comparison
fixtures + a deterministic projection digest. This is SHADOW mode only: no active
navigation, route, page body, or behavior is changed. The XP106-04-U unit lane, the
XP106-04-I integration and XP106-04-R rollback lanes, and — added in this slice — the
XP106-04-A e2e-api shadow-safety lane pass on the live stack with current-session
evidence below. The e2e-ui (XP106-04-W) / shared-infrastructure canary (XP106-04-C)
lanes and the authenticated-session shadow-parity acceptance remain coupled-forward
to later slices (BUG-070-001 supplies the production browser-session canary).
## Decision Record
The scope owns shadow renderer adapters and high-fan-out canaries only.
## Completion Statement
Not complete — status remains `in_progress`. The XP106-04-U unit, XP106-04-I
integration, XP106-04-R rollback, and XP106-04-A e2e-api shadow-safety lanes are
proven with current-session evidence on the live stack; the e2e-ui (XP106-04-W) and
shared-infrastructure canary (XP106-04-C) lanes, and the authenticated-session
shadow-parity acceptance, are coupled-forward and their DoD items remain unchecked.
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

### XP106-04-A

Regression E2E (e2e-api, SCN-106-003): through the REAL registered routes on the live
`./smackerel.sh test e2e` stack (core+ml+postgres+nats, auth ENFORCED via a non-empty
`SMACKEREL_AUTH_TOKEN`), the renderer-neutral `ExperienceProjection` built from the
REAL generated catalog renders through the three SHADOW adapters (server / PWA / Card)
to an IDENTICAL projection digest per real audience (the "shadow projection digests
agree" half: daily_user 18 surfaces, operator 20 surfaces, server==pwa==card each);
AND every route the shadow projection binds is a REAL registered route (no projected
href 404s; an adversarial unregistered control DOES 404 — non-tautological); AND the
routes' CURRENT authorization + behavior are UNCHANGED by the shadow adapters:
protected server routes (`/`, `/cards`, `/digest`, `/knowledge`, `/notifications`,
`/recommendations`, `/settings`) still return an auth outcome (401) unauthenticated,
public routes (`/assistant` 302, `/pwa/…` 200) still serve, and no live response
carries the shadow fixture's content-free markers or the projection digest (SHADOW
mode is not wired into any live response). It fails CLOSED without altering the live
route: a tampered projection (a navigate leaf loses its route) surfaces a visible
`ShadowFailure` (non-settled, typed `*F106PresentationError`, no optimistic fixture)
on all three adapters while the `/pwa/` live route is byte-identical (same status +
body sha256) before and after. No interception, no mock, no auth injection, no
invented endpoints.

HONEST AUTH COUPLING: this lane runs UNAUTHENTICATED and proves the AUTHORIZATION
DECISION shadow mode preserves (protected→auth outcome, public→served). The
AUTHENTICATED-SESSION acceptance (observing the shadow parity inside the rendered
authenticated shell) is coupled-forward to BUG-070-001's unified production browser
session — the scope's declared External Entry Gate — and is NOT faked here.

Command and full captured PASS output (current session; no `/home/...` paths in the
output; soft line-wraps from the 80-column terminal capture rejoined into their
logical lines):

```text
$ ./smackerel.sh test e2e --go-run 'ShadowProjectionDigestsAgree'
go-e2e: applying -run selector: ShadowProjectionDigestsAgree
=== RUN   TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior
=== RUN   TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/shadow_projection_digests_agree_across_server_pwa_card
    experience_shadow_e2e_test.go:238: digest-parity audience=daily_user surfaces=18 server==pwa==card digest=sha256:403e6a5cba58bc758751c8b605a2aba219604b32d8c33ab843f86a89ea2a6818
    experience_shadow_e2e_test.go:238: digest-parity audience=operator   surfaces=20 server==pwa==card digest=sha256:04f7c457d0910c344a5c356fc144e1d0a80fc3340189644d511fc8c8a20804f3
    experience_shadow_e2e_test.go:260: adversarial control /definitely-not-registered-xp106-04-a    -> 404 (distinct not-found class)
    experience_shadow_e2e_test.go:268: auth-mode probe /knowledge -> 401 (authEnforced=true)
=== RUN   TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/real_routes_preserve_current_authorization_and_behavior
    experience_shadow_e2e_test.go:322: protected route /                              -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:303: public   route /assistant                      -> 302 (served; behavior preserved)
    experience_shadow_e2e_test.go:322: protected route /cards                         -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:322: protected route /digest                        -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:322: protected route /knowledge                     -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:322: protected route /notifications                 -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:303: public   route /pwa/                           -> 200 (served; behavior preserved)
    experience_shadow_e2e_test.go:303: public   route /pwa/connectors.html            -> 200 (served; behavior preserved)
    experience_shadow_e2e_test.go:303: public   route /pwa/model-connections.html     -> 200 (served; behavior preserved)
    experience_shadow_e2e_test.go:303: public   route /pwa/photo-health.html          -> 200 (served; behavior preserved)
    experience_shadow_e2e_test.go:303: public   route /pwa/wiki.html                  -> 200 (served; behavior preserved)
    experience_shadow_e2e_test.go:322: protected route /recommendations               -> 401 (auth outcome; authorization preserved)
    experience_shadow_e2e_test.go:322: protected route /settings                      -> 401 (auth outcome; authorization preserved)
=== RUN   TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/live_responses_carry_no_shadow_markers
    experience_shadow_e2e_test.go:366: shadow-marker scan /pwa/          -> 200 (no shadow sentinel / no projection digest in live body)
    experience_shadow_e2e_test.go:366: shadow-marker scan /login         -> 200 (no shadow sentinel / no projection digest in live body)
=== RUN   TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/adapter_fail_closed_does_not_alter_live_route_or_install_fallback
    experience_shadow_e2e_test.go:426: fail-closed pwa    adapter -> visible ShadowFailure, non-settled, no optimistic fixture
    experience_shadow_e2e_test.go:426: fail-closed card   adapter -> visible ShadowFailure, non-settled, no optimistic fixture
    experience_shadow_e2e_test.go:426: fail-closed server adapter -> visible ShadowFailure, non-settled, no optimistic fixture
    experience_shadow_e2e_test.go:441: live route /pwa/    identical before/after fail-closed: status=200 body-sha256=130a7cf817415434
--- PASS: TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior (0.02s)
    --- PASS: TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/shadow_projection_digests_agree_across_server_pwa_card (0.00s)
    --- PASS: TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/real_routes_preserve_current_authorization_and_behavior (0.01s)
    --- PASS: TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/live_responses_carry_no_shadow_markers (0.00s)
    --- PASS: TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior/adapter_fail_closed_does_not_alter_live_route_or_install_fallback (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.146s
PASS: go-e2e
```

Implementation file: `tests/e2e/experience_shadow_e2e_test.go` (new).

Supporting lanes (this slice, current session): `./smackerel.sh check` → OK (`Config
is in sync with SST`, `env_file drift guard: OK`, `scenario-lint: OK` — 17 registered,
0 rejected); `./smackerel.sh format --check` → `75 files already formatted` (the new
e2e file is gofmt-clean). The `./smackerel.sh test e2e` lane brought up and tore down
its own disposable stack.

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

Coupled-forward (unchecked, honest): XP106-04-W (e2e-ui) and XP106-04-C (shared-infra
canary) require the live PWA browser DOM in a later slice; the authenticated PWA-auth
canary + the authenticated-session shadow-parity acceptance are additionally gated on
BUG-070-001's unified production browser session. XP106-04-A (e2e-api) is proven
above in `report.md#xp106-04-a`.
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
