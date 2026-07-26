# Report: SCOPE-106-03 Truthful State And Feedback Foundation

Links: [scope.md](scope.md) | [spec.md](../../spec.md) | [scope index](../_index.md)

## Summary

**In Progress — slice 1 of a multi-slice scope (TAKEOVER continuation of the
dormant spec-106 shell cluster).**

This slice builds the CHECK-ONLY, renderer-neutral PRESENTATION foundation and
its single fast, no-live-stack unit lane (XP106-03-U). It adds three pure Go
presentation types under `internal/experience/**` that map ABSTRACT,
owner-classified outcomes to closed presentation vocabularies. They query no
domain store, parse no raw error string, import no domain logic, and never
convert a failure into empty or success:

- `ExperienceStatePresenter` (`internal/experience/state_presenter.go`): maps a
  closed `OwnerReadKind` seam (loading, ready, first-use-empty, filtered-empty,
  stale, degraded, needs-setup, disabled, unauthorized, access-denied,
  not-found, error) to a closed `ViewState`. Unknown, contradictory, or unsafe
  outcomes fail closed to the typed `*F106PresentationError`. It also presents
  the INDEPENDENT `Availability` axis through a closed `AvailabilitySignal`
  boundary: only a resolved readiness fact can establish availability — a
  registered route, an enabled flag, an HTTP 200, a mounted handler, an empty
  array, or a health probe CANNOT (SCN-106-005).
- `MutationFeedbackPresenter` (`internal/experience/mutation_presenter.go`):
  maps a closed `OwnerMutationKind` seam (idle, pending, persisted, idempotent,
  conflict, refused, partial, failed) to a closed `MutationState`. `partial` is
  NEVER complete (`IsComplete(partial) == false`) and is NEVER announced as
  success; success is announced only from `persisted` (validated to carry an
  authoritative read-back) or an owner-complete `idempotent` (SCN-106-010).
- `AuthenticatedRequestAdapter` (`internal/experience/auth_adapter.go`):
  presentation-only. A 401 synchronously CLEARS the closed set of protected
  presentation targets (protected DOM markers, accessible labels, in-memory
  business state, pending work, graph pixels) before re-auth and retains no
  session; a 403 RETAINS the valid session and shows access-denied without a
  login loop; an authorized outcome imposes no content override (axis
  independence). It never touches token issuance or middleware verification —
  those are excluded by the Change Boundary.

A shared redaction contract (`safeCode`) rejects any non content-free code
(raw error text, stack frames, secrets, URLs, queries, personal content)
reaching a presented value.

**Renderer-neutral SEAMS only — no domain logic reimplemented.** Search, Digest,
Assistant, Graph, Cards, Recommendations, Sources, Activity, and readiness owners
keep their own typed outcomes; this foundation maps the ABSTRACT outcome passed
across the seam. No owner package is imported.

## Decision Record

- **One typed fail-closed error.** A single `*F106PresentationError` (with a
  `Surface` discriminator and a deterministic `Violations` slice) is shared by
  the state, mutation, and auth presenters, mirroring the `*F106RouteDrift`
  pattern already established in `internal/experience/validator.go`.
- **Availability independence as a closed type boundary.** Availability is not a
  field on `OwnerReadOutcome`; it is reached only through `PresentAvailability`,
  which refuses every `AvailabilitySignal` except `SignalReadinessResolved`.
  This makes "route/flag/health/HTTP-200/empty/handler cannot create
  availability" a mechanical type-level guarantee, not a convention.
- **Partial can never be complete.** `IsComplete` returns true only for
  `persisted`/`idempotent`; `AnnouncesSuccess` returns true only for a
  read-back-confirmed `persisted`/complete `idempotent`. A `core_plus_pending`
  outcome maps to `partial` and both predicates return false regardless of how
  much core state persisted.
- **Change boundary honored.** Only new files under `internal/experience/**` and
  this scope's artifacts were touched. Active auth middleware/token code,
  handwritten nav/handlers, and every excluded surface are unchanged.

## Completion Statement

Not complete. SCOPE-106-03 is **In Progress — 9 of 16 DoD items closed with
current-session evidence; the remaining 7 are honestly left `[ ]`, coupled
forward to the SCOPE-106-04/05 shell cutover, not fabricated.** Closed across
slices 1–3: the three presentation types + shared fail-closed error + redaction
contract; the unit lane XP106-03-U (axes CLOSED/INDEPENDENT/FAIL-CLOSED); the
real-owner integration lane XP106-03-I; the shared-state privacy-clear/redaction
canary XP106-03-P; the atomic rollback contract XP106-03-rollback (shadow-artifact
grep + deterministic 5-invariant contract test); the grouped Build-Quality Gate
(check / lint / format / artifact-lint / traceability, zero warnings); and the
Change-Boundary item. Left `[ ]` and honestly coupled forward to SCOPE-106-04/05
(the live shell cutover that wires the presenters into the live renderers): the
two cross-renderer Core Outcomes (401/403 equivalence across renderers; shared
canaries/privacy/rollback protecting every high-fan-out consumer), XP106-03-A
(e2e-api) and XP106-03-W (e2e-ui) with their two regression-planning rows, and
the live-renderer half of the Independent-canary-suite row — each genuinely
requires the SCOPE-106-04/05 nav/renderer cutover and is not provable while the
handwritten renderers remain the untouched live authority. The slice-4 XP106-03-A
/ XP106-03-W sections below record what IS provable NOW against the existing real
routes/UI and cite the exact coupled clause.

**Foreign surfaces observed but untouched.** The working tree carries
pre-existing FOREIGN modifications from a parallel session
(`internal/api/graphapi/activation.go`, `internal/web/handler_test.go`,
`docs/Development.md` documentation-freshness, and `specs/079-*`). They are
outside this scope's Change Boundary and were NOT created or modified by this
slice; this scope's four new files are untracked additions under
`internal/experience/**`. The scoped check/lint/format/unit lanes all ran clean
with those foreign modifications present.

## Code Diff Evidence

Four new untracked files under `internal/experience/**`: `state_presenter.go`,
`mutation_presenter.go`, `auth_adapter.go`, and `state_presenter_test.go`. No
tracked file was modified by this slice.

<!-- SLICE-1-EVIDENCE-BEGIN -->

## Test Evidence

Slice 1 executed one fast check-only lane (XP106-03-U unit). The four live lanes
(XP106-03-I integration, XP106-03-A e2e-api, XP106-03-W e2e-ui, XP106-03-P
shared-infrastructure canary) plus the canary/rollback and Build-Quality gates
are honestly unchecked pending slice 2.

### XP106-03-U

**Claim Source:** executed — `./smackerel.sh test unit --go --go-run 'TestExperienceState'`, current session 2026-07-26.

`TestExperienceStateAvailabilityAndMutationAxesRemainClosedIndependentAndFailClosed`
(`internal/experience/state_presenter_test.go`) ran under the `-run` selector and
passed. Its subtests prove, against the real presenter code (no mocks):

- axes CLOSED + total: every `OwnerReadKind` maps to a valid `ViewState`, and
  `ViewState` / `OwnerReadKind` / `OwnerMutationKind` / `MutationState`
  `valid()` reject fabricated values;
- SCN-106-004: `needs_setup` / `disabled` render their exact optional state, not
  an outage or a ready journey;
- SCN-106-005 + independence: `PresentAvailability` accepts only
  `SignalReadinessResolved`; a route / flag / HTTP-200 / handler / empty / health
  signal returns a typed error and leaks no value;
- failure ≠ empty/success: no non-success read kind maps to
  ready/first-use-empty/filtered-empty; `failed → error`;
- SCN-106-010: `partial` is never complete and never announced as success;
  `persisted` without authoritative read-back fails closed;
- auth axis: a 401 clears all five protected targets, retains no session, offers
  safe re-auth; a 403 retains the session, clears nothing, offers no login loop;
  an authorized outcome imposes no content override;
- fail-closed: unknown/contradictory/unsafe (redaction-violating) outcomes on
  every presenter return `*F106PresentationError` and never a downgrade to
  empty/ready/available/success.

The `internal/experience` package reports `ok ... 0.009s` (real tests ran — NOT
`[no tests to run]`), and the whole `go test ./...` sweep finished OK with zero
FAIL (`UNIT_EXIT=0`).

```text
$ ./smackerel.sh test unit --go --go-run 'TestExperienceState'
+ go test -run TestExperienceState -count=1 ./...
[go-unit] applying -run selector: TestExperienceState
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/config-validate      0.023s [no tests to run]
ok      github.com/smackerel/smackerel/cmd/core 0.194s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api     0.174s [no tests to run]
ok      github.com/smackerel/smackerel/internal/experience      0.009s
ok      github.com/smackerel/smackerel/internal/web     0.175s [no tests to run]
ok      github.com/smackerel/smackerel/web/pwa/tests    0.006s [no tests to run]
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

### Supporting quality lanes (slice 1)

**Claim Source:** executed — current session 2026-07-26. These support the unit
lane; the grouped Build-Quality Gate DoD item stays `[ ]` (its full scope
includes traceability/canary/rollback/docs, which are slice 2).

- `./smackerel.sh check` → `CHECK_EXIT=0` (config-validate OK, "Config is in sync
  with SST", env_file drift guard OK, scenario-lint OK: 17 registered, 0
  rejected).
- `./smackerel.sh lint` → `LINT_EXIT=0` (web validation passed; no finding on the
  new files).
- `./smackerel.sh format` → `FORMAT_EXIT=0`; independent `gofmt -l` on the four
  new files returned an empty list (all gofmt-clean).

<!-- SLICE-1-EVIDENCE-END -->

<!-- SLICE-2-EVIDENCE-BEGIN -->

### XP106-03-I

**Claim Source:** executed — `./smackerel.sh test integration` and
`./smackerel.sh test integration-light --go-run 'TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess'`,
current session 2026-07-26.

`TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess`
(`tests/integration/experience/state_presenter_test.go`, package
`integrationexperience`, `//go:build integration`) feeds REAL, owner-classified
typed outcomes from ACTUAL smackerel domain owners through the spec-106
presenters — NO mock, NO stub, NO interception:

- **readiness owner** `internal/recommendation/availability.Determine` (the real
  provider-backed readiness determination). Enabled + zero providers resolves to
  a real `CapabilityUnavailable` / `CauseZeroConfiguredProviders` / `Ready()==false`
  → the presenter projects `AvailabilityUnavailable` and the content axis NEVER
  fabricates a ready/empty/success state (SCN-106-005). A registered route cannot
  fabricate availability for the same real zero-provider capability (adversarial).
  Real degraded / available / disabled determinations project their exact truthful
  state.
- **persistence + post-commit read-back owner**
  `internal/intelligence.DeriveSynthesisHealth`. A real committed-but-partial
  output (`SynthesisDegradedPartial`, `Persisted=false`) maps to `partial`, is
  never `IsComplete`, and is never `AnnouncesSuccess` (SCN-106-010); a commit whose
  mandatory read-back gate did not verify is never success. The presenter's
  `IsComplete` / `AnnouncesSuccess` EXACTLY track the owner's `Persisted` / `Healthy`
  truth for all six real outcomes.
- **live digest read owner** `internal/digest.Generator` — a REAL live-PostgreSQL
  round-trip on the disposable stack: a real populated row written via the real
  production `HandleDigestResult` and read back via the real `GetLatest` projects
  `ready` (never false-empty); a real zero-row read projects `first_use_empty`
  (honest empty, never a fabricated ready). The `INFO digest stored date=2028-05-20`
  line is the real production write firing.

The planned command `./smackerel.sh test integration` passed (`PASS: go-integration`,
`INTEGRATION_EXIT=0`); the `-v` per-subtest detail below is from the stores-only
`integration-light` live lane (same disposable Postgres; the test needs no core/ml)
and shows all three real-owner sub-lanes green.

```text
go-integration: applying -run selector: TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess
...
ok      github.com/smackerel/smackerel/tests/integration/drive  0.115s [no tests to run]
?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
=== RUN   TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess
=== RUN   TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/readiness_owner_availability_and_content_are_truthful
=== RUN   TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/synthesis_owner_partial_never_complete_commit_alone_never_success
=== RUN   TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/live_digest_read_never_false_empty
2026/07/26 05:39:40 INFO digest stored date=2028-05-20 words=42 model=test-model
--- PASS: TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess (0.03s)
    --- PASS: TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/readiness_owner_availability_and_content_are_truthful (0.00s)
    --- PASS: TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/synthesis_owner_partial_never_complete_commit_alone_never_success (0.00s)
    --- PASS: TestRealOwnerOutcomesProjectWithoutFalseEmptyReadyOrSuccess/live_digest_read_never_false_empty (0.03s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/experience     0.146s
...
PASS: go-integration-light
INTEGRATION_LIGHT_EXIT=0
```

### XP106-03-P

**Claim Source:** executed — `./smackerel.sh test integration-light --go-run 'TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail'`,
current session 2026-07-26.

`TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail`
(`tests/integration/experience/privacy_clear_test.go`, package
`integrationexperience`, `//go:build integration`) is the shared-state
PRIVACY-CLEAR + 403-DENIAL + REDACTION canary. It exercises the REAL
`experience.AuthenticatedRequestAdapter` and the REAL state/mutation presenters
(no mock, no interception) and proves:

- **401 privacy clear** — a session loss clears the EXACT closed set of five
  protected presentation targets (protected DOM, accessible labels, in-memory
  business state, pending work, graph pixels), retains NO session, and offers a
  safe re-auth path;
- **403 denial** — a valid-session access denial RETAINS the session, clears
  NOTHING, shows access-denied, and never loops through login;
- **redaction by construction** — every string the emitted presentation directive
  carries (the single SOURCE every renderer / a11y tree / log / metric / trace /
  storage surface derives from) is a member of the closed presentation
  vocabulary, so a stack frame / secret / bearer token / PII email / SQL query /
  reset URL structurally CANNOT ride along; a JSON walk of the directive rejects
  any non-vocabulary string;
- **fail-closed with no leak** — an unclassified auth outcome errors to a typed
  `*F106PresentationError` and emits a ZERO-value presentation; a state/mutation
  owner outcome carrying raw detail in a code field fails closed, leaks no
  presented value, and the error message carries no raw detail.

HONEST coupling-forward: the presenters are not yet wired into the live
server/PWA routes (cutover is SCOPE-106-04/05), so the LIVE cross-renderer
surface propagation (real PWA 401 clear, real HTMX / Card-PRG mutation canaries
against the live DOM/a11y/logs) is coupled forward — see the coupled-forward
Core Outcomes and the live-renderer canary-suite planning item, left `[ ]`. This
canary proves the shared SOURCE directive is privacy-clearing and redaction-clean
by construction, before any consumer adopts it.

```text
go-integration: applying -run selector: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/401_session_loss_clears_all_protected_presentation_and_retains_no_session
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/403_access_denial_retains_session_and_never_loops_login
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/emitted_auth_presentation_exposes_only_closed_vocabulary_no_sensitive_detail
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/unclassified_auth_outcome_fails_closed_and_leaks_no_protected_value
=== RUN   TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/state_and_mutation_presenters_reject_raw_detail_and_leak_no_value
--- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail (0.00s)
    --- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/401_session_loss_clears_all_protected_presentation_and_retains_no_session (0.00s)
    --- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/403_access_denial_retains_session_and_never_loops_login (0.00s)
    --- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/emitted_auth_presentation_exposes_only_closed_vocabulary_no_sensitive_detail (0.00s)
    --- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/unclassified_auth_outcome_fails_closed_and_leaks_no_protected_value (0.00s)
    --- PASS: TestSessionLossClearsProtectedPresentationAndSafeStatesExposeNoSensitiveDetail/state_and_mutation_presenters_reject_raw_detail_and_leak_no_value (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/experience     0.108s
PASS: go-integration-light
P_EXIT=0
```

<!-- SLICE-2-EVIDENCE-END -->

<!-- SLICE-3-EVIDENCE-BEGIN -->

## Slice 3 — Rollback Contract And Build-Quality Gate

Slice 3 closes the atomic rollback contract, the grouped Build-Quality Gate, and
the Change-Boundary item. Every command below was executed in the current session
(2026-07-26) with full, un-truncated output. These are build-quality/structural
lanes independent of the live e2e lanes (XP106-03-A e2e-api / XP106-03-W e2e-ui
are recorded in slice 4).

### XP106-03-rollback

**Claim Source:** executed — shadow-artifact import grep + `./smackerel.sh test integration-light --go-run 'TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior'`, current session 2026-07-26.

The atomic rollback contract (scope.md "Rollback") has two proofs.

**Shadow-artifact proof — "disabling the presenter package restores prior
renderer behavior".** The spec-106 presenters are an ADDITIVE shadow layer: NO
live-render package (`internal/**`, `cmd/**`) imports them, so disabling the
package is a zero-live-change decision and the still-active handwritten renderers
keep serving unchanged. A tree-wide import grep confirms the ONLY importers are
test files:

```text
$ grep -rnE '"github.com/smackerel/smackerel/internal/experience"' internal/ cmd/
$ echo "LIVE_IMPORT_GREP_EXIT=$?"
LIVE_IMPORT_GREP_EXIT=1        # no matches — no live-path importer
$ grep -rlE '"github.com/smackerel/smackerel/internal/experience"' --include='*.go' .
./tests/e2e/product_experience_catalog_e2e_test.go
./tests/integration/experience/route_inventory_test.go
./tests/integration/experience/rollback_contract_test.go
./tests/integration/experience/state_presenter_test.go
./tests/integration/experience/privacy_clear_test.go
```

**Deterministic contract test — "rollback NEVER restores an unsafe behavior".**
`TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior`
(`tests/integration/experience/rollback_contract_test.go`, `//go:build integration`)
walks the ENTIRE closed owner-outcome space and proves the package STRUCTURALLY
cannot emit any of the five behaviors scope.md forbids on rollback — so disabling
it can never reintroduce them. Each assertion is adversarial (it would FAIL if the
presenter regressed to the forbidden behavior, e.g. `ReadFailed -> ViewReady`,
`partial -> complete`, `401` retaining the session, `already-committed -> a second
persisted success`). It ran GREEN on the real stores-only integration-light lane
(postgres + nats up; the contract needs no DB, so it is a pure deterministic Go
check under the integration build tag):

```text
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
...
go-integration: applying -run selector: TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior
=== RUN   TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior
    rollback_contract_test.go:82: 1. failure-as-empty: 4 failure/auth-loss kinds avoid ready/first_use_empty/filtered_empty (ok)
    rollback_contract_test.go:113: 2. optimistic-success: 5 non-persisted outcomes (incl. partial) never complete/announced-success (ok)
    rollback_contract_test.go:135: 3. raw-errors: raw owner detail fails closed with a typed error and no leaked value (ok)
    rollback_contract_test.go:151: 4. retained-protected-DOM-after-401: 401 clears all 5 targets and retains no session (ok)
    rollback_contract_test.go:166: 5. duplicate-submit: already-committed->idempotent (no 2nd success), accepted->pending lock (ok)
    rollback_contract_test.go:172: unsafe/unknown owner outcome fails closed to a typed error, never a guessed state (ok)
--- PASS: TestPresentationPackageRollbackIsAtomicAndRestoresNoUnsafeBehavior (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/experience     0.106s
PASS: go-integration-light
ROLLBACK_EXIT=0
```

Together: the package is a self-contained additive unit (shadow grep) whose only
possible outputs are members of closed truthful vocabularies (contract test), so
the atomic rollback — disabling the presenter package — restores the prior
handwritten-renderer behavior and can NEVER restore failure-as-empty, optimistic
success, raw errors, retained protected DOM after 401, or duplicate-submit. An
owner outcome that cannot be mapped safely fails closed to a typed error
(Unavailable-equivalent), never a guessed state.

### XP106-03-build-gate

**Claim Source:** executed — `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format` + `gofmt -l .`, `bash .github/bubbles/scripts/artifact-lint.sh specs/106-coherent-product-experience`, `bash .github/bubbles/scripts/traceability-guard.sh specs/106-coherent-product-experience`, current session 2026-07-26.

The grouped Build-Quality Gate is a build-quality/structural block independent of
the live e2e lanes. State-exclusivity / privacy / auth-access / no-raw-error /
no-sensitive-storage are proven by XP106-03-U/I/P and the XP106-03-rollback
contract above; this section records check / lint / format / artifact-lint /
traceability with zero warnings.

`./smackerel.sh check` — `CHECK_EXIT=0`:

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.3478124 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

`./smackerel.sh lint` — `LINT_EXIT=0` (`go vet ./...` silent-clean; web manifest /
JS / extension-version validation OK; pip editable-install noise elided):

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
LINT_EXIT=0
```

`format` — the ONLY gofmt offender tree-wide was the scope-03-owned
`tests/integration/experience/privacy_clear_test.go` (the XP106-03-P canary
committed by the prior slice-2 session), a trivial composite-literal alignment. It
was normalized via the sanctioned `./smackerel.sh format`; post-fix `gofmt -l .`
is EMPTY (whole Go tree clean) and only the scope-03 file changed (no foreign file
touched). NOTE: the two files the SCOPE-106-02 slice-3 report recorded as FOREIGN
gofmt offenders (`internal/api/graphapi/activation.go`,
`internal/web/handler_test.go`) are gofmt-CLEAN in the current tree:

```text
$ ./smackerel.sh format --check       # BEFORE — sole offender is a scope-03 file
tests/integration/experience/privacy_clear_test.go
FORMAT_EXIT=1
$ ./smackerel.sh format               # sanctioned normalize (gofmt -w)
$ gofmt -l .                          # AFTER — whole Go tree clean (empty list)
$ echo "GOFMT_L_EXIT=$?"
GOFMT_L_EXIT=0
$ git status --porcelain
 M docs/Development.md                                  # FOREIGN pre-existing (untouched)
 M internal/api/graphapi/activation.go                  # FOREIGN pre-existing (untouched, gofmt-clean)
 M internal/web/handler_test.go                         # FOREIGN pre-existing (untouched, gofmt-clean)
 M specs/079-prod-autonomous-supervisor/spec.md         # FOREIGN pre-existing (untouched)
 M specs/079-prod-autonomous-supervisor/state.json      # FOREIGN pre-existing (untouched)
 M tests/integration/experience/privacy_clear_test.go   # scope-03 file: gofmt-normalized only
?? tests/integration/experience/rollback_contract_test.go  # scope-03: new rollback contract
```

`bash .github/bubbles/scripts/artifact-lint.sh specs/106-coherent-product-experience`
— `ARTIFACT_LINT_EXIT=0`:

```text
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes/03-truthful-state-feedback-foundation/scope.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes/03-truthful-state-feedback-foundation/report.md
✅ No repo-CLI bypass detected in scopes/03-truthful-state-feedback-foundation/report.md command evidence
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

`bash .github/bubbles/scripts/traceability-guard.sh specs/106-coherent-product-experience`
— PASSED (0 warnings), `TRACEABILITY_EXIT=0`:

```text
--- Gherkin → DoD Content Fidelity (Gate G068) ---
✅ scopes/03-truthful-state-feedback-foundation/scope.md scenario maps to DoD item: SCN-106-004 Optional capability is represented honestly
✅ scopes/03-truthful-state-feedback-foundation/scope.md scenario maps to DoD item: SCN-106-005 Enabled capability with no working provider is not ready
✅ scopes/03-truthful-state-feedback-foundation/scope.md scenario maps to DoD item: SCN-106-010 Mutation reports authoritative outcome
ℹ️  DoD fidelity: 29 scenarios checked, 29 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 29
ℹ️  Test rows checked: 161
ℹ️  Scenario-to-row mappings: 29
ℹ️  DoD fidelity scenarios: 29 (mapped: 29, unmapped: 0)
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

The "canary" and "rollback" clauses of this gate item are satisfied by XP106-03-P
(shared-state privacy-clear/redaction canary, slice 2) and the XP106-03-rollback
contract above. The LIVE cross-renderer canary suite (native form / HTMX / Card-PRG
against the live DOM) is the SCOPE-106-04/05-coupled item tracked by the separate
"Independent canary suite" planning row, not by this Build-Quality gate item.

### XP106-03-change-boundary

**Claim Source:** executed — `git status --porcelain`, current session 2026-07-26.

Zero EXCLUDED file families were changed by this session. This session touched ONLY
scope-03-owned surfaces:

- `tests/integration/experience/rollback_contract_test.go` — NEW (the rollback
  contract test, occupying the boundary's sanctioned
  `tests/integration/experience/**` rollback-test slot);
- `tests/integration/experience/privacy_clear_test.go` — a SCOPE-03-owned file
  (the XP106-03-P canary), gofmt-normalized ONLY (pure mechanical formatting, no
  behavior change) to clear the sole tree-wide gofmt offender.

The other modified paths — `docs/Development.md`,
`internal/api/graphapi/activation.go`, `internal/web/handler_test.go`,
`specs/079-prod-autonomous-supervisor/{spec.md,state.json}` — are FOREIGN
pre-existing working-tree modifications from a parallel session; they were NOT
created or modified by this session and are recorded untouched. NONE of the
EXCLUDED families was changed by this session: no handwritten nav/renderers/auth
middleware, no `internal/api/*` (the foreign `activation.go` modification predates
this session and was left untouched), no `internal/recommendation/**`, no
`specs/079-*`, no specs 105/107/072/078, no proactive/telegram/whatsapp/
surfacing, no `docs/Development.md`, no `tests/integration/synthesis/**`.

<!-- SLICE-3-EVIDENCE-END -->

<!-- SLICE-4-EVIDENCE-BEGIN -->

## Slice 4 — Live E2E Lanes

Slice 4 records the two live e2e lanes against the REAL disposable stack
(`./smackerel.sh test e2e` / `test e2e-ui`), NO interception, NO mock. Both DoD
rows stay `[ ]` coupled forward to the SCOPE-106-04/05 presenter cutover; each
lane proves the maximal "holds NOW" truth against the existing routes/UI and
cites the exact coupled clause (the SCOPE-106-02 XP106-02-W precedent).

### XP106-03-A

**Claim Source:** executed — `./smackerel.sh test e2e --go-run 'TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes'`, current session 2026-07-26.

`TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes`
(`tests/e2e/experience_state_e2e_test.go`, `//go:build e2e`, package `e2e`) ran
against the disposable LIVE e2e stack (the runner exports
`CORE_EXTERNAL_URL=http://smackerel-core:PORT` and a non-empty
`SMACKEREL_AUTH_TOKEN`, so auth is enforced) with NO interception and NO mock. It
adds NO new route/API — it probes ONLY existing registered routes and proves the
running server ALREADY keeps the outcome classes STRUCTURALLY DISTINCT: every
protected read is an auth-loss (401), public content is a served 200, the capture
mutation is refused (401, never a fabricated success), and an unregistered route
is a 404 — so a failure is NEVER collapsed into an empty page or a success.

Adversarial (non-tautological): the deliberately-unregistered control
`/definitely-not-registered-xp106-03-a` returns 404, so the probe distinguishes a
real outcome class from an invented one.

The go-e2e lane passed (`PASS: go-e2e`, `E2E_A_EXIT=0`); the stack was brought up
HEALTHY (postgres / nats / ollama / searxng / jaeger / stub-providers / core / ml)
and fully torn down (containers + volumes + network removed = ephemeral, no
residue):

```text
$ ./smackerel.sh test e2e --go-run 'TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes'
=== RUN   TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes
    experience_state_e2e_test.go:93: adversarial control /definitely-not-registered-xp106-03-a  -> 404 (distinct not-found class)
    experience_state_e2e_test.go:100: auth-mode probe /settings -> 401 (authEnforced=true)
    experience_state_e2e_test.go:123: protected read /              -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /digest        -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /settings      -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /knowledge     -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /recommendations -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /notifications -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /api/digest    -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:123: protected read /api/recent    -> 401 (auth-loss, distinct from content)
    experience_state_e2e_test.go:137: public content /pwa/          -> 200 (served content, distinct from auth-loss/not-found)
    experience_state_e2e_test.go:137: public content /login         -> 200 (served content, distinct from auth-loss/not-found)
    experience_state_e2e_test.go:152: mutation POST /api/capture -> 401 (auth-loss; refused, not a fabricated success)
    experience_state_e2e_test.go:169: distinctness matrix (auth enforced): auth-loss=401/403 vs content=200 vs not-found=404 — mutually distinct (ok)
--- PASS: TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.142s
PASS: go-e2e
E2E_A_EXIT=0
```

**Honest coupling decision — DoD rows stay `[ ]` (coupled forward to
SCOPE-106-04/05).** The DoD-row behavior is the FULL four-axis "Availability
content auth AND mutation outcomes remain structurally distinct through real
routes". The live run above truthfully proves the CONTENT / AUTH / MUTATION axes
(plus not-found) are ALREADY structurally distinct through the real routes NOW —
the pre-existing no-collapse invariant this foundation preserves. The remaining
gap is genuinely cutover-coupled: the AVAILABILITY band surfaced as a DISTINCT
route axis (a zero-provider capability's readiness projected as a distinct
availability outcome, not folded into content) and the SHARED-PRESENTER PROJECTION
of all four outcomes through the live routes are wired only at the SCOPE-106-04/05
shell cutover; the presenters remain a shadow layer here (proven at the type level
by XP106-03-U and against the real readiness owner by XP106-03-I). So XP106-03-A
and its scenario-specific-E2E-regression row stay UNCHECKED, coupled forward —
exactly the precedent SCOPE-106-02 set for XP106-02-W. The test file header records
the same coupling; the pass is not faked as fully satisfying the DoD clause.

<!-- SLICE-4-EVIDENCE-END -->

## Planned Test References
**Claim Source:** not-run
The live e2e lanes XP106-03-A (e2e-api) and XP106-03-W (e2e-ui) are recorded in
slice 4 below; the integration lanes XP106-03-I and XP106-03-P and the rollback +
build-gate + change-boundary sections are already executed above. Concrete files
and titles are in `scope.md` and root `test-plan.json`.
## Uncertainty Declarations
The 401/403 CROSS-RENDERER equivalence (server vs PWA behaving identically at
runtime through the shared bands) and the shared-state band-rendering on the live
UI remain unverified until the SCOPE-106-04/05 shell cutover wires the presenters
into the live renderers. The type-level presentation contract, the shared SOURCE
directive redaction/privacy-clear (XP106-03-P), and the atomic rollback contract
(XP106-03-rollback) are proven; the LIVE cross-renderer projection of them is
coupled forward.
## Scenario Contract Evidence
See `scenario-manifest.json` and `test-plan.json` at the spec root for the
SCN-106-004 / SCN-106-005 / SCN-106-010 registrations this scope maps.
## Coverage Report
No runtime coverage is claimed. Slice 1 is a type-level unit lane.
## Lint/Quality
`./smackerel.sh lint` and `./smackerel.sh format` both exited 0; the four new
files are gofmt-clean.
## Validation Summary
No validation or certification result is claimed.
## Audit Verdict
No audit verdict is claimed.
