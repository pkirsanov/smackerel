# Execution Report: [BUG-059-001] gkeepapi missing from ML sidecar build surfaces

## Status: DELIVERED + VERIFIED (build-manifest pin shipped; image carries gkeepapi) — pending downstream specialist certification

The maintainer chose **Path A** (DELIVER) and accepted the reverse-engineered-library supply-chain risk, resolving design.md Open Question Q1 and clearing the `state.json status: blocked`. This report records BOTH the original DIAGNOSTIC evidence (which confirmed the bug) AND the FIX VERIFICATION evidence. Every claim below cites real sanctioned-CLI command output with exit codes. The fix is a build-manifest pin + a structural guard test and changes ZERO runtime code (`ml/app/keep_bridge.py` untouched).

## Summary
- **Bug:** `gkeepapi` live-mode runtime dependency is absent from all three ML sidecar build surfaces (`ml/requirements.txt`, `ml/pyproject.toml`, `ml/Dockerfile`); `ml/app/keep_bridge.py:72` imports it lazily and `:82` raises `RuntimeError("gkeepapi is not installed")` on the first live-mode sync.
- **Severity:** Medium (fail-safe — default `sync_mode: takeout`, three explicit opt-ins required to reach the failure, loud `RuntimeError`, no data loss / no security exposure).
- **Root cause:** Lazy `import gkeepapi` plus a mock-based unit suite hid the missing build-manifest pin from both the build and the tests.
- **Status:** DELIVERED + VERIFIED. `gkeepapi==0.17.1` pinned on both build surfaces; smackerel-ml image rebuilt; `import gkeepapi` exits 0 inside the image (v0.17.1); structural guard test GREEN with the pin / RED without it; no unit regression. state.json `status: in_progress` (delivered, pending the bugfix-fastlane specialist-certification chain — separation of duties).
- **Scenarios validated:** SCN "built image contains the dependency" (Evidence D), "structural guard catches pin removal" (Evidence A+B), "live-mode sync no longer raises not-installed" (Evidence D + disposition). Full live Google authentication remains operator-credential-gated / out of scope.

## Diagnostic Evidence (verified at HEAD 9638b065, 2026-06-07)

### Evidence 1 — gkeepapi absent from ALL build surfaces (root cause)
Command:
```
grep -rn gkeepapi ml/requirements.txt ml/pyproject.toml ml/Dockerfile; echo "exit=$?"
```
Output:
```
exit=1
```
No matches in any of the three build surfaces; exit 1 confirms absence.

### Evidence 2 — consumer code DOES import gkeepapi (lazy, inside authenticate())
Command:
```
grep -nE "import gkeepapi|gkeepapi\.Keep\(\)|gkeepapi is not installed" ml/app/keep_bridge.py
```
Output:
```
72:        import gkeepapi  # noqa: F811
74:        keep = gkeepapi.Keep()
82:        raise RuntimeError("gkeepapi is not installed. Install with: pip install gkeepapi")
```

### Evidence 3 — Dockerfile installs only requirements.txt (which lacks the pin)
Command:
```
grep -n "requirements.txt" ml/Dockerfile
```
Output:
```
14:COPY requirements.txt .
15:RUN pip install --no-cache-dir -r requirements.txt
```

### Evidence 4 — unit suite is structurally blind (mocks the session / patches authenticate)
Command:
```
grep -nE "MagicMock|_keep_session|patch.object" ml/tests/test_keep.py | head
```
Output:
```
6:from unittest.mock import MagicMock, patch
91:        bridge._keep_session = MagicMock()
308:        bridge._keep_session = mock_keep
316:            with patch.object(bridge, "authenticate", return_value=mock_keep):
```
The pre-seeded `_keep_session` / patched `authenticate` mean the lazy `import gkeepapi` at `keep_bridge.py:72` never executes under test — the suite cannot catch the missing dependency.

### Evidence 5 — SST default is takeout (fail-safe; live mode not active by default)
Command:
```
sed -n '357,361p' config/smackerel.yaml
```
Output:
```
  google-keep:
    enabled: false
    sync_mode: takeout # takeout, gkeepapi, or hybrid
    import_dir: "" # path to Google Takeout Keep export directory
    include_archived: false
```

### Evidence 6 — parent runbook assumes the pin already ships
Command:
```
sed -n '325,330p' specs/059-google-keep-live-mode/design.md
```
Output:
```
1. `./smackerel.sh logs` to inspect the `keep_protocol_drift_detected` event(s) and identify what changed (e.g., `gkeepapi` version, response shape).
2. If a library upgrade is needed, bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild.
3. Edit `config/smackerel.yaml` and change `drift_ack_token` to ANY new value.
```

## Consequence
In any built `smackerel-ml` image, the first real live-mode sync (`sync_mode ∈ {gkeepapi,hybrid}` + `gkeep_enabled:true` + `warning_acknowledged:true`) raises `RuntimeError("gkeepapi is not installed")` at `keep_bridge.py:82`. The spec 059 headline LIVE capability is non-deployable as shipped. The failure is LOUD and fail-safe (no data loss / no security exposure); default `takeout` users are unaffected.

## Test Evidence

### Files changed (change boundary honored)
- `ml/pyproject.toml` — added `gkeepapi==0.17.1` to `[project.optional-dependencies] runtime` (alongside the sibling feature libs youtube-transcript-api / trafilatura). Exact-pinned (not `>=`) deliberately because gkeepapi is a reverse-engineered client Google periodically breaks (design.md Q2).
- `ml/requirements.txt` — added the exact line `gkeepapi==0.17.1` (the hand-maintained lock that `ml/Dockerfile:15` installs into the image). pip resolves the transitive deps `gpsoauth>=1.1.0` and `future>=0.16.0`.
- `ml/tests/test_build_surface_pins.py` — NEW structural guard test. Reads the manifest TEXT; it never `import gkeepapi` (importing the lib would couple the test to the very dependency under question, re-introducing the blind spot in reverse).

**pyproject placement rationale:** `runtime` (not base `dependencies`) is the consistent home alongside the other optional feature libs AND keeps the dev/test install (`pip install -e ./ml[dev]`, which installs base+dev, NOT `runtime`) from dragging in the reverse-engineered library — so the structural guard stays environment-independent, exactly as design.md requires. The lock (`requirements.txt`) is the surface that actually ships gkeepapi in the image, and the Dockerfile installs it unconditionally. design.md said `[project] dependencies`; the maintainer explicitly authorized `runtime` if more consistent — it is.

### Evidence A — structural guard GREEN + full Python unit suite no-regression (sanctioned CLI)
**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh test unit --python`
```
s....................................................................... [ 14%]
... (all dots, no F) ...
......................................................................   [100%]
500 passed, 2 skipped, 2 warnings in 25.67s
[py-unit] pytest ml/tests finished OK
PY_UNIT_EXIT=0
```
500 passed (was 496 before the 4 new `test_build_surface_pins.py` tests), exit 0. The unit env installs `./ml[dev]` (base+dev, NOT `runtime`), so gkeepapi is NOT importable there — confirming the guard asserts on manifest text, not on an import, and is environment-independent.

### Evidence B — adversarial RED proof (non-tautological), sanctioned CLI
**Executed:** YES &nbsp; With the `gkeepapi==0.17.1` pin TEMPORARILY removed from `ml/requirements.txt`, re-ran `cd ~/smackerel && ./smackerel.sh test unit --python`:
```
FAILED ml/tests/test_build_surface_pins.py::test_gkeepapi_pinned_in_requirements
FAILED ml/tests/test_build_surface_pins.py::test_gkeepapi_pin_removal_fails_red
2 failed, 498 passed, 2 skipped, 2 warnings in 23.51s
PY_UNIT_RED_EXIT=1
```
The guard FAILS RED precisely when the pin is absent. `test_gkeepapi_pinned_in_pyproject` still PASSED (pyproject was untouched — the requirements guard is independent), proving the test discriminates on pin presence and is NOT tautological — it catches the exact reintroduction of this bug. The pin was then RESTORED byte-identically and re-verified GREEN (`500 passed, 2 skipped`, exit 0).

### Evidence C — format clean (sanctioned CLI)
**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh format --check`
```
63 files already formatted
FORMAT_CHECK_EXIT=0
```
(The new test file was formatted in place via `./smackerel.sh format` — `1 file reformatted, 62 files left unchanged` — before this clean re-check.)

### Evidence D — image build + in-image `import gkeepapi` (sanctioned build)
**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh build`
```
 => [smackerel-ml builder 5/9] RUN pip install --no-cache-dir -r requir  117.9s
 ...
 ✔ smackerel-core  Built
 ✔ smackerel-ml    Built
BUILD_EXIT=0
```
The ML image rebuilt with the new `requirements.txt`; gkeepapi installs at the `pip install -r requirements.txt` step. Then verified the dependency is present in the built artifact (read-only inspection of the image the sanctioned build produced):
**Command:** `docker run --rm smackerel-smackerel-ml python -c "import importlib.metadata as m; import gkeepapi; print('IMPORT_OK gkeepapi version=', m.version('gkeepapi'))"`
```
IMPORT_OK gkeepapi version= 0.17.1
IMPORT_EXIT=0
```
`import gkeepapi` exits 0 inside the image, resolving to the pinned 0.17.1. Therefore the lazy `import gkeepapi` at `keep_bridge.py:72` now SUCCEEDS, and the `except ImportError → RuntimeError("gkeepapi is not installed")` branch (`:81-82`) is structurally unreachable in the built image.

### Evidence E — broader no-regression across the whole codebase (Go unit, sanctioned CLI)
**Executed:** YES &nbsp; **Command:** `cd ~/smackerel && ./smackerel.sh test unit --go`
All Go packages report `ok` EXCEPT three PRE-EXISTING, unrelated failures, none of which import any file this bug changed (the change is Python-ML-manifest-only, zero Go delta):
```
--- FAIL: TestDocFreshness_AllPromptContractsDocumented (docs/Development.md stale; spec-032 prompt contracts)
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (node not on PATH; spec-073)
--- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (dart not on PATH; spec-073)
GO_UNIT_EXIT=1
```
`internal/docfreshness` (doc staleness) and `tests/unit/clients` (`node`/`dart` not on PATH in the Go test container) are the exact two pre-existing FAIL packages BUG-056-002's regression pass already documented as unrelated. No regression is introduced by BUG-059-001.

### Live-mode authentication smoke (honest disposition)
The image now carries gkeepapi (Evidence D), so `authenticate()` reaches `keep.login(email, password)` (`keep_bridge.py:73-74`) instead of raising the not-installed `RuntimeError`. The DoD requirement — "no `gkeepapi is not installed` RuntimeError" — is therefore PROVEN: the ONLY source of that error is `ImportError` on `import gkeepapi`, which now succeeds. FULL live Google Keep authentication (a successful session) is **operator-credential-gated** (real `KEEP_GOOGLE_EMAIL` + Google app password, and Google not having server-side-broken the path) and **OUT OF SCOPE** for this build-pin bug — it is NOT claimed as verified.

### Regression Disposition — "Broader E2E regression suite" (honest not-run UD)
**Claim Source: not-run (deliberate).** This bug changes only build-manifest lines + a new unit test (zero runtime-code delta). design.md DELIBERATELY chose a structural-unit guard over a live-import/E2E test (an import test would couple execution to the very dependency under question; a text-level manifest assertion is environment-independent). The live `./smackerel.sh test e2e` stack exercises Go-core/stack runtime behavior a Python-ML-manifest change cannot affect, and the design designated structural-unit + build-image-import as this bug's regression contract. The proportionate broader-regression evidence is provided: full Python unit suite GREEN (Evidence A) + full Go unit suite GREEN modulo the enumerated pre-existing unrelated failures (Evidence E) + image build + in-image import (Evidence D). The live E2E suite was NOT separately spun up; this one DoD row is left as an explicit not-run Uncertainty Declaration (anti-fabrication, Gate G021), which is why the packet terminal status is `in_progress` rather than a forced `done` (mirroring BUG-056-002's integration-gated migration row).

## Parent-Spec Non-Interference + Governance Follow-Up
Parent spec `059-google-keep-live-mode` status remains `done`; NO parent artifact (spec.md / design.md / scopes.md / state.json / report.md / uservalidation.md / scenario-manifest.json) was modified by this delivery. Only this bug packet's artifacts (report.md, scopes.md, bug.md, state.json) changed.

**Governance follow-up flagged (NOT acted on here — parent is `done`, editing it would lift its grandfather clause):**
- **No false live-mode DELIVERY claim found.** Parent report.md is honest: its Summary + Completion Statement explicitly mark all live-stack DoD rows as Uncertainty-Declared and routed via `state.json.transitionRequests` "for a follow-up integration round." It does NOT claim live mode is end-to-end deployable/verified. So there is nothing to correct.
- **Prerequisite linkage worth recording for the parent's routed live-stack round.** That deferred follow-up integration round had an unmet hard prerequisite — the `gkeepapi` pin was entirely absent from the build surfaces, so the sidecar's `authenticate()` would raise `ImportError` before any live-stack integration could run. BUG-059-001 (this fix) is the unblocker. parent `design.md:327` recovery-runbook step ("bump the `gkeepapi` pin in `ml/requirements.txt` and rebuild") also presupposed a pin that did not exist until now; it is accurate going forward. A maintainer picking up the parent's routed live-stack round should reference this bug as the prerequisite.

## Completion Statement
The fix is DELIVERED and VERIFIED at the structural level: `gkeepapi==0.17.1` is pinned on both build surfaces, the smackerel-ml image rebuilds cleanly (`BUILD_EXIT=0`), and `import gkeepapi` exits 0 inside the built image at version 0.17.1 — so the live-mode `RuntimeError("gkeepapi is not installed")` is structurally eliminated. The structural guard test is GREEN with the pin and RED without it (non-tautological, proven by real execution), the full Python unit suite is regression-free (500 passed), Go unit shows only pre-existing unrelated failures, and `format --check` is clean. The one not-run row is the broader LIVE E2E suite (deliberate, zero-runtime-delta + design's structural-regression choice — explicit UD), and full live Google authentication is operator-credential-gated/out-of-scope. Accordingly the packet is `in_progress` (delivered, pending the bugfix-fastlane specialist-certification chain — regression/simplify/stabilize/security/audit — owned downstream by separation of duties), NOT a forced `done`. Parent spec 059 stays `done` with its protected artifacts unchanged; the governance follow-up above is flagged, not acted on.

## Audit Findings (bubbles.audit — 2026-06-09)

Separation-of-duties terminal-status audit. The delivery agent explicitly declined to self-certify and routed the terminal call here. This audit **independently re-ran** the key evidence (did not trust the delivery summary) and adjudicated the terminal status + the E2E DoD-row disposition.

### A1 — Independent verification re-run (what audit executed this session)

**Executed:** YES &nbsp; **Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/059-google-keep-live-mode/bugs/BUG-059-001-gkeepapi-missing-from-sidecar-build`
```
--- Check 4: DoD Completion (Zero Unchecked) ---
ℹ️  INFO: DoD items total: 12 (checked: 11, unchecked: 1)
🔴 BLOCK: Resolved scope artifacts have 1 UNCHECKED DoD items — ALL must be [x] for 'done'
--- Check 5: Scope Status Cross-Reference ---
🔴 BLOCK: Resolved scope artifacts have 1 scope(s) still marked 'In Progress' — ALL scopes must be Done
--- Check 6: Specialist Phase Completion ---
🔴 BLOCK: Required phase 'regression' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'simplify' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'stabilize' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'security' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: Required phase 'audit' NOT in execution/certification phase records (Gate G022 violation)
🔴 BLOCK: 5 specialist phase(s) missing — work was NOT executed through the full pipeline
--- Check 15: Phase-Scope Coherence (Gate G027) ---
🔴 BLOCK: Execution/certification phases claim implement/test phases but ZERO scopes are marked 'Done' — FABRICATION (Gate G027)
GUARD_EXIT=1
```

**Executed:** YES &nbsp; **Command:** `./smackerel.sh test unit --python` (independent re-run of the deliverable's regression evidence)
```
s....................................................................... [ 14%]
... (all dots, no F) ...
......................................................................   [100%]
500 passed, 2 skipped, 2 warnings in 18.79s
[py-unit] pytest ml/tests finished OK
PY_UNIT_EXIT=0
```
Independently reproduces the delivery's Evidence A (`500 passed`) — the structural guard is GREEN and there is no unit regression.

**Executed:** YES &nbsp; **Commands:** skip-marker scan + image identity + in-image import (no rebuild)
```
=== skip-marker scan (audit 3.5/3.6) ===
SKIP_SCAN_EXIT=1 (1=clean/no-match)
=== docker image identity ===
smackerel-smackerel-ml:latest 8cfe72c97bf2 44 minutes ago
=== in-image import (read-only, no rebuild) ===
IMPORT_OK gkeepapi version= 0.17.1
IMPORT_EXIT=0
```
Independently reproduces Evidence D: the freshly-built image (`8cfe72c97bf2`) carries `gkeepapi==0.17.1`; `import gkeepapi` exits 0. Skip-marker scan of `ml/tests/test_build_surface_pins.py` is CLEAN (no `skip`/`xfail`/`TODO`/`FIXME`). The guard test was additionally reviewed by inspection: `test_gkeepapi_pin_removal_fails_red` strips the real `gkeepapi==` line and asserts the detector reports ABSENT, and `test_detector_rejects_floated_range` rejects `>=` — non-tautological, confirmed.

**Executed:** YES &nbsp; **Command:** `grep -rnE 'keep_bridge|gkeepapi|keep\.sync|sync_mode' tests/e2e/` (does any e2e scenario back the broad-E2E DoD row?)
```
(no matches — exit 1)
```
The live e2e suite (`./smackerel.sh test e2e` → `go test -tags e2e ./tests/e2e/...`) has **zero** Google-Keep-live coverage. No e2e scenario exercises the changed path.

### A2 — Terminal-status decision: `in_progress` (CONFIRMED — `done` is NOT honestly reachable this pass)

The deliverable is **sound on the merits** (pin on both surfaces, in-image import green, non-tautological guard, no regression — all independently reproduced above). But terminal `done` is **not** legitimately reachable, and **not primarily because of the E2E row**. The binding blockers are structural:

1. **The bugfix-fastlane specialist-certification chain is genuinely incomplete (Gate G022).** Guard Check 6 shows `regression`, `simplify`, `stabilize`, `security` are NOT in the phase records (and `audit` only ran now). The delivery agent ran implement+test+validate only. By contrast the cited precedent **BUG-056-002** ran the *entire* chain (its `certifiedCompletedPhases` = implement, test, validate, regression, simplify, stabilize, security, audit) and *still* rested at `in_progress`. This packet has strictly less of the chain done.
2. **Only `bubbles.validate` may certify `done`** (completion-governance: `certification.*` is validate-owned). `certification.certifiedCompletedPhases` is `[]`; validate has not certified. Audit cannot and must not write that certification.
3. **Multiple independent mechanical gates block `done`** even after the chain completes: G027 (zero `Done` scopes), G053 (no `### Code Diff Evidence`), G040 (deferral language), G060 (no red→green markers), plus the unchecked DoD row and the In-Progress scope.

Audit does **not** record `audit` as a completed phase (the audit-mode rule records the phase only on a `SHIP_IT` verdict). The audit ran and routed onward; that is logged in `state.json.executionHistory`, not in `completedPhaseClaims`/`certifiedCompletedPhases`.

### A3 — E2E DoD-row disposition: non-applicable broad-regression gate for this change class (audit-reviewed UD; NOT an independent blocker)

Adjudicating the user's central question — legitimate gate vs. phantom row:

- The change is **zero-runtime-delta** (`ml/app/keep_bridge.py` untouched) and **zero-Go-delta**; `./smackerel.sh test e2e` is a Go-core/live-stack suite with no causal path to an ML build-manifest pin.
- `tests/e2e/` has **zero** Keep-live coverage (grep above, no matches). There is **no backing e2e scenario** for this connector/change-class.
- The authoritative behavior contract `scenario-manifest.json` declares **3** scenarios — all `integration`/`unit`, **none** an e2e-suite scenario — and all three are verified (Evidence A/B/D).
- `design.md` **deliberately** designated structural-unit + build-image-import as this bug's regression contract, with a documented rationale (an import/e2e test would couple execution to the dependency under question).

Therefore the broad-E2E row is a **template-carryover with no backing scenario** for this change — materially **different** from BUG-056-002's not-run migration row, which had a real backing test (`TestTwitterOAuthMigration_AppliesCleanly`) and a real live-DB behavior that genuinely needed the integration stack. That row legitimately held BUG-056-002 open; **this** row does not independently hold BUG-059-001 open.

**Disposition:** the row stays `[ ]` (truthful — the broad live-e2e suite genuinely did not run; audit will **not** fabricate a green `[x]` for an unrun suite). It is recorded as an **audit-reviewed Uncertainty Declaration that `bubbles.validate` MAY resolve at final certification** per completion-governance ("a scope cannot be `Done` when any DoD item is unchecked unless it has an approved Uncertainty Declaration that was resolved by audit"). It is **not** the binding blocker — the incomplete specialist chain + validate certification are.

### A4 — Findings the eventual `done` closure MUST address (do not block `in_progress`, will block `done`)

| ID | Gate | Finding | Owner |
|----|------|---------|-------|
| AUD-059-001-F1 | G022/G027 | Run the remaining bugfix-fastlane specialist chain (regression → simplify → stabilize → security) and record phases; security was reported clean by the operator but is **not** recorded in `state.json` — it must be executed/recorded by `bubbles.security`. | bubbles.workflow → specialists |
| AUD-059-001-F2 | G053 | report.md lacks a `### Code Diff Evidence` section (build-manifest diff for `ml/requirements.txt` + `ml/pyproject.toml`). | bubbles.implement/test |
| AUD-059-001-F3 | G040 | report.md contains 7 deferral-language hits ("out of scope", "operator-credential-gated", "follow-up", etc.). Acceptable while `in_progress`; must be reconciled before `done`. | bubbles.docs/validate |
| AUD-059-001-F4 | G060 | No red→green markers detected by the guard; the genuine RED proof (Evidence B) should be tagged with the guard's expected red→green markers. | bubbles.test |
| AUD-059-001-F5 | G016/8A/8D | Test Plan missing the explicit scenario-specific regression-E2E row shape + refactor change-boundary DoD item the guard expects. | bubbles.plan |
| AUD-059-001-F6 | certification | Final `done` certification (incl. resolving the A3 UD) is `bubbles.validate`-owned; audit cannot perform it. | bubbles.validate |

### A5 — Spot-Check Recommendations (automation-bias mitigation — MANDATORY)

The user should manually verify:
1. **The specialist chain is real, not assumed.** `bubbles.security`'s clean result was asserted in the request but is **not** recorded in `state.json`; confirm security (and regression/simplify/stabilize) actually run and record evidence before any `done` push.
2. **The E2E-row resolution is validate's call, not audit's.** Audit recommends treating the row as non-applicable for this change class, but the *resolution* belongs to `bubbles.validate` at certification; spot-check that validate independently agrees rather than inheriting audit's view.
3. **Evidence-block hygiene.** The guard flagged 16/18 report evidence blocks as lacking terminal-output signals; the two load-bearing claims (Evidence A `500 passed`, Evidence D in-image import) were independently reproduced here, but the diagnostic blocks (grep/sed outputs) are short — confirm they are acceptable for the eventual `done` closure.

### A6 — Audit verdict

**🛑 REWORK_REQUIRED — but the rework is the certification pipeline, NOT the fix.** The substantive deliverable is audit-clean and independently reproduced. `done` is correctly withheld; the honest resting state is **`in_progress`** (delivered, pending the remaining bugfix-fastlane specialist chain + validate certification). Routed via the RESULT-ENVELOPE to `bubbles.workflow` to complete the chain; no change to the delivered fix is required.

### A7 — Governance follow-up (parent spec 059 — NOT acted on; parent stays `done`)

Audit affirms the delivery agent's flag: BUG-059-001 is the hard prerequisite unblocker for spec 059's routed live-stack round (before this pin, `authenticate()` raised `ImportError` before any live integration could run). No false live-mode delivery claim was found in the parent. Parent artifacts were not touched (grandfather clause preserved). A maintainer resuming spec 059's live-stack round should reference this bug as the prerequisite.
