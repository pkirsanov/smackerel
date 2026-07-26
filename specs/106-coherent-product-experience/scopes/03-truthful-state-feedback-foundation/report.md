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

Not complete. SCOPE-106-03 is **In Progress — 3 of 15 DoD items closed with
current-session evidence; the remaining 12 are honestly left `[ ]` pending the
live slice 2, not fabricated.** Delivered + evidenced this slice: the three
presentation types + the shared fail-closed error + the redaction contract, and
the fast unit lane XP106-03-U proving the availability, content, and mutation
axes remain CLOSED, INDEPENDENT, and FAIL-CLOSED (failure never becomes
empty/success; a route/flag/health signal never becomes availability; partial
never becomes complete). Explicitly NOT claimed this slice and left unchecked:
the 401/403 cross-renderer equivalence core, the shared-state canary core, the
live lanes XP106-03-I (integration), XP106-03-A (e2e-api), XP106-03-W (e2e-ui),
XP106-03-P (shared-infrastructure canary), the four Shared-Infrastructure and
Regression planning items, and both Build-Quality-Gate items — all genuinely
require the live disposable stack and the shell adapters of the live slice 2 and
are left `[ ]` honestly, not fabricated.

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

## Planned Test References
**Claim Source:** not-run
The live lanes XP106-03-I, XP106-03-A, XP106-03-W, and XP106-03-P, plus the
`report.md#xp106-03-rollback` and `report.md#xp106-03-change-boundary` sections,
require the real disposable stack and the shared shell adapters and are coupled
forward to slice 2 (honestly unchecked here, not fabricated). Their concrete
files and titles are listed in `scope.md` and root `test-plan.json` and are not
execution evidence.
## Uncertainty Declarations
The 401/403 CROSS-RENDERER equivalence (server vs PWA behaving identically at
runtime), the shared-state privacy-clear/redaction behavior on the live stack,
and the atomic rollback path remain unverified until slice 2 executes them
against the real stack. This slice proves only the type-level presentation
contract.
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
