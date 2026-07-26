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
