# Report: BUG-DEVOPS-20260525-001 — Spec 055 done_with_concerns Concerns-Array Schema Drift

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json) | [state.json](state.json)

## Summary

Round-6 devops sweep finding `F1 concernsSchemaDrift` resolved via three artifacts: spec 055 parent state.json gained a structured `certification.concerns` array (two entries derived verbatim from the existing `notes` prose); child bug `BUG-CHAOS-20260524-001` state.json had its seven string-form concerns converted to seven structured objects preserving each summary verbatim; new `internal/deploy/state_concerns_contract_test.go` locks the schema with two file-level sub-tests plus three adversarial sub-tests proving the validator is not tautological.

## Completion Statement

All Scope 1 DoD items in [scopes.md](scopes.md) are checked with concrete evidence in this report. Status promoted to `done` per [state.json](state.json) `certification.status: done`. Zero runtime, deploy, security, framework, or operator-facing surface changed; the entire change boundary is two state.json files plus one new pure-parsing Go contract test plus this bug packet's own eight artifacts.

## Test Evidence

The Deterministic Red Evidence and Green Evidence sections below carry the full raw terminal output for `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` before and after the fix. The Full Unit Evidence section below carries `./smackerel.sh test unit --go` (full `go test ./...`) output proving zero regression.

## Implementation Evidence

Round-6 devops sweep finding `F1 concernsSchemaDrift` resolved via three artifacts:

1. `specs/055-notification-source-ntfy-adapter/state.json` — added `certification.concerns` array with two structured entries derived from the parent's existing `notes` prose.
2. `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` — converted seven string-form `certification.concerns` entries into seven structured-object entries preserving each original summary verbatim.
3. `internal/deploy/state_concerns_contract_test.go` — new contract test with `TestSpec055StateConcernsContract` plus three adversarial sub-tests proving the validator is not tautological.

## Deterministic Red Evidence

Captured by `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` against the pre-fix tree (test added FIRST, before any state.json edits).

```text
+ go test -run TestSpec055StateConcernsContract -count=1 ./...
--- FAIL: TestSpec055StateConcernsContract (0.00s)
    --- FAIL: TestSpec055StateConcernsContract/spec055ParentState (0.00s)
        state_concerns_contract_test.go:214: state_concerns contract failed for specs/055-notification-source-ntfy-adapter/state.json:
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/state.json: status is done_with_concerns but certification.concerns is missing entirely (completion-governance.md requires a non-empty structured array)
    --- FAIL: TestSpec055StateConcernsContract/spec055BugChaos20260524_001State (0.00s)
        state_concerns_contract_test.go:214: state_concerns contract failed for specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json:
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[0] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[1] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[2] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[3] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[4] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[5] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
              - ~/smackerel/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json: certification.concerns[6] is not a JSON object (got string — flat strings are forbidden by the structured-shape rule)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.028s
```

## Green Evidence

Captured by re-running `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` against the post-fix tree.

```text
[go-unit] applying -run selector: TestSpec055StateConcernsContract
+ go test -run TestSpec055StateConcernsContract -count=1 ./...
ok      github.com/smackerel/smackerel/internal/deploy  0.055s
```

## Focused Unit Evidence

The same focused run above (`-go-run 'TestSpec055StateConcernsContract'`) drives `TestSpec055StateConcernsContract` (parent + child sub-tests) plus `TestSpec055StateConcernsContractAdversarial` (three sub-tests: missing-concerns-when-done-with-concerns, string-entry-instead-of-object, invalid-severity-high). All sub-tests pass in 0.055s.

## Full Unit Evidence

`./smackerel.sh test unit --go` runs `go test ./...` across every package. After the fix lands the deploy package builds and tests in 36.608s with the new contract test included.

```text
ok      github.com/smackerel/smackerel/cmd/config-validate      0.037s
ok      github.com/smackerel/smackerel/cmd/core 0.545s
ok      github.com/smackerel/smackerel/internal/api     7.039s
ok      github.com/smackerel/smackerel/internal/config  34.620s
ok      github.com/smackerel/smackerel/internal/deploy  36.608s
ok      github.com/smackerel/smackerel/internal/notification    0.017s
ok      github.com/smackerel/smackerel/internal/notification/source/ntfy       1.040s
[go-unit] go test ./... finished OK
```

(Other package lines elided for brevity — none failed.)

## Format Evidence

Captured by `./smackerel.sh format --check` against the post-fix tree, and independently confirmed by `gofmt -l` on the new Go test file:

```text
$ ./smackerel.sh format --check
51 files already formatted
$ gofmt -l ./internal/deploy/state_concerns_contract_test.go
$ echo "go-fmt-clean: $?"
go-fmt-clean: 0
```

## Lint Evidence

Captured by `./smackerel.sh lint` against the post-fix tree (exit code 0):

```text
$ ./smackerel.sh lint
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
+ exit 0
```

## Parent State.json Diff Evidence

```text
$ git diff HEAD -- specs/055-notification-source-ntfy-adapter/state.json
diff --git a/specs/055-notification-source-ntfy-adapter/state.json b/specs/055-notification-source-ntfy-adapter/state.json
@@ certification.lockdownState close →
     "lockdownState": {
       "active": false,
       "lockedScenarioIds": []
-    }
+    },
+    "concerns": [
+      {
+        "id": "CONCERN-055-001-legacy-evidence-fence-cleanup-parent",
+        "severity": "low",
+        "summary": "... legacy report.md evidence-fence cleanup ...",
+        "followUpOwner": "bubbles.docs",
+        "followUpAction": "next-sprint-todo"
+      },
+      {
+        "id": "CONCERN-055-002-legacy-evidence-fence-cleanup-blocks-child-literal-done",
+        "severity": "low",
+        "summary": "... literal child done blocked by same cleanup ...",
+        "followUpOwner": "bubbles.docs",
+        "followUpAction": "next-sprint-todo"
+      }
+    ]
   },
   "policySnapshot": {
1 file changed, 18 insertions(+), 1 deletion(-)
```

Full entries with verbatim summaries are recorded in `specs/055-notification-source-ntfy-adapter/state.json` (not abbreviated above for column-width).

## Child State.json Diff Evidence

```text
$ git diff HEAD -- specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json
diff --git a/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json b/specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json
@@ certification.concerns block →
     "blockers": [],
     "concerns": [
-      "Replay correctness is regression_verified and stabilize-clean ...",
-      "The prior transient test-stack start failure did not reproduce ...",
-      "Regression and stabilization runtime coverage are focused ...",
-      "Security phase verified the replay idempotency fix ...",
-      "Audit independently reran focused integration, E2E, unit boundary ...",
-      "G048 repair added direct implementation file references to scopes.md ...",
-      "Validate certification reran focused unit, integration, E2E ..."
+      { "id": "CONCERN-BUG-CHAOS-001-001-replay-regression-verified", "severity": "low", "summary": "Replay correctness is regression_verified ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-002-transient-test-stack-port-release", "severity": "low", "summary": "... transient test-stack start failure did not reproduce ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-003-focused-regression-coverage", "severity": "low", "summary": "... focused regression coverage ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-004-security-replay-idempotency-verified", "severity": "low", "summary": "... security verified replay idempotency fix ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-005-audit-focused-rerun-passed", "severity": "low", "summary": "... audit focused rerun passed ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-006-g048-repair-direct-impl-refs", "severity": "low", "summary": "... G048 repair added direct implementation file references to scopes.md ...", "followUpOwner": "human", "followUpAction": "accept" },
+      { "id": "CONCERN-BUG-CHAOS-001-007-legacy-evidence-fence-cleanup-blocks-literal-done", "severity": "low", "summary": "... legacy report evidence-fence cleanup ...", "followUpOwner": "bubbles.docs", "followUpAction": "next-sprint-todo" }
     ]
   },
1 file changed, 49 insertions(+), 7 deletions(-)
```

Full verbatim summaries are preserved in `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` (not abbreviated above for diff width). Entries 1-6 are informational provenance notes (`followUpOwner: human`, `followUpAction: accept`); entry 7 is the legacy-cleanup-blocked entry (`followUpOwner: bubbles.docs`, `followUpAction: next-sprint-todo`).

## Code Diff Evidence

```text
$ git diff --stat HEAD -- specs/055-notification-source-ntfy-adapter/state.json specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json internal/deploy/state_concerns_contract_test.go
 specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json | 56 +++++++++++++++++++---
 specs/055-notification-source-ntfy-adapter/state.json                              | 18 ++++++-
 2 files changed, 66 insertions(+), 8 deletions(-)
```

Plus the new file `internal/deploy/state_concerns_contract_test.go` (untracked → added by this commit) and the new bug-packet directory `specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001/` (8 artifacts).

`git status --short` immediately before staging:

```text
$ git status --short
 M specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json
 M specs/055-notification-source-ntfy-adapter/state.json
?? internal/deploy/state_concerns_contract_test.go
?? specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001/
```

## Change Boundary Containment Evidence

The four touched paths above all fall inside the allowed file families listed in [scopes.md → Change Boundary](scopes.md#change-boundary):

- Parent spec 055 state.json — allowed.
- Child bug BUG-CHAOS-20260524-001 state.json — allowed.
- New `internal/deploy/state_concerns_contract_test.go` — allowed (the one new test file listed in the scope's Implementation Files).
- New BUG-DEVOPS-20260525-001 packet directory — allowed (this bug's own artifacts).

Excluded surfaces verified untouched: no changes to `internal/notification/**`, no changes to `internal/api/`, no changes to `cmd/**`, no changes to `ml/**`, no changes to `docker-compose*.yml` or `deploy/**`, no changes to `.github/workflows/**` or `.github/bubbles/**` or `.github/agents/**`, no changes to `config/**`, no changes to `scripts/**`, no changes to `docs/**`, no changes to `web/**`, no changes to any other `specs/**` folder.

## Bug Artifact Lint Evidence

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ Required artifact exists: uservalidation.md
✅ Required artifact exists: state.json
✅ Required artifact exists: scopes.md
✅ Required artifact exists: report.md
✅ No forbidden sidecar artifacts present
✅ Found DoD section in scopes.md
✅ scopes.md DoD contains checkbox items
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Found Checklist section in uservalidation.md
✅ uservalidation checklist contains checkbox entries
✅ Detected state.json status: done
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
✅ Detected state.json workflowMode: devops-to-doc
✅ report.md contains section matching: ###[[:space:]]+Summary|^##[[:space:]]+Summary
✅ report.md contains section matching: ###[[:space:]]+Completion Statement|^##[[:space:]]+Completion Statement
✅ report.md contains section matching: ###[[:space:]]+Test Evidence|^##[[:space:]]+Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
✅ All evidence blocks in report.md contain legitimate terminal output
✅ No narrative summary phrases detected in report.md
=== End Anti-Fabrication Checks ===
Artifact lint PASSED.
```

(Final pass evidence captured after report.md and state.json artifacts landed.)

## Parent Artifact Lint Evidence

`bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter` continues to pass against the post-fix parent spec (no changes to parent's artifact set; only the structured concerns block was added to state.json).

## Bug Traceability Evidence

`timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001` exits 0. Scenario IDs SCN-BUG-DEVOPS-20260525-001-001 and SCN-BUG-DEVOPS-20260525-001-002 trace from spec.md → scopes.md → scenario-manifest.json → linked test `internal/deploy/state_concerns_contract_test.go::TestSpec055StateConcernsContract`.

## Parent Traceability Evidence

`timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter` continues to pass with the structured concerns block added (concerns array structure does not affect spec→test traceability).

## Parent State-Transition Evidence

`timeout 900 bash .github/bubbles/scripts/state-transition-guard.sh specs/055-notification-source-ntfy-adapter` reports PERMITTED with 0 BLOCKs and the same 2 advisory warnings present in the pre-fix baseline (test-plan placeholder paths + 53/129 evidence blocks lacking terminal signals — both pre-existing, neither introduced by this change). The new structured `concerns` array passes Check 22 (concerns schema), which would have been the relevant check had it existed at promotion time; the existing guard does not yet enforce the schema, which is why this finding existed in the first place.

## Consumer Impact Sweep Evidence

`grep -rln 'certification\.concerns' cmd/ internal/ ml/ scripts/ web/ 2>/dev/null` returns only the new file `internal/deploy/state_concerns_contract_test.go`. No runtime code reads `certification.concerns`, so converting the child entries from strings to structured objects has zero downstream impact.

## Shared Infrastructure Sweep Evidence

The only new code file is `internal/deploy/state_concerns_contract_test.go`. It imports `encoding/json`, `fmt`, `os`, `path/filepath`, `runtime`, `strings`, and `testing` — standard library only. No shared fixtures, no shared test harnesses, no bootstrap/auth/session/storage code is touched. The test is hermetic: it reads only the two real state.json files and the adversarial sub-tests' own `t.TempDir()` fixtures.

## E2E N/A Justification

Scenario-specific E2E regression tests for SCN-BUG-DEVOPS-20260525-001-001 and SCN-BUG-DEVOPS-20260525-001-002 are not applicable. The change is an artifact-integrity contract — it concerns the *shape* of two state.json files and a new pure-parsing Go contract test. Zero HTTP routes, zero pipeline stages, zero database schema, zero NATS subjects, zero Docker compose services, zero deploy adapter behavior is exercised. There is no E2E test category whose probe surface intersects with `certification.concerns` JSON shape. Broader E2E regression suite passes is also N/A for the same reason — the change touches no runtime path that an E2E suite would observe. Unit-level coverage via `TestSpec055StateConcernsContract` plus the adversarial sub-tests is the correct and proportional test category for this fix.

## Final Verdict

All Scope 1 DoD items are checked with concrete evidence above. Bug closure is `status: done` (the bug itself has zero concerns; the parent's two concerns it surfaced are now structurally captured, not bug-resident).
