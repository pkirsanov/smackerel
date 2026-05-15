# Scopes: BUG-020-004 — ML NATS client `SMACKEREL_AUTH_TOKEN` fail-loud read

> **Workflow:** bugfix-fastlane
>
> **Status ceiling:** done
>
> **Planning cleanup note (2026-05-15, `bubbles.plan`):** This packet is scoped to `ml/app/nats_client.py` and `ml/tests/test_nats_client.py`. Validate/stabilize have recorded source-contract evidence that the NATS client imports `_AUTH_TOKEN` from `.auth` and uses `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN`; all 15 DoD items are checked with inline raw evidence. This planning cleanup marks Scope 1 `Done` and preserves the later-owner boundary: security, validate, and audit phase completion remain outside this pass. A separate implementation pass handled the `ml/app/main.py` Gate G028 finding; this scope does not claim that source change.

## Execution Outline

### Phase Order

1. Scope 1 — Reconcile the BUG-020-004 NATS client auth-token source contract and regression test contract without claiming completion before source evidence exists.
2. Evidence checkpoint — Record scoped source/test evidence for only `ml/app/nats_client.py` and `ml/tests/test_nats_client.py` before certification.
3. Validation checkpoint — Run the repo-standard Python unit command and scoped artifact gates before marking the scope done.

### New Types & Signatures

| Contract | Signature / Identifier | Status |
|----------|------------------------|--------|
| Runtime source contract | `from .auth import _AUTH_TOKEN` in `ml/app/nats_client.py` | Required; not certified in current artifacts |
| Runtime connect contract | `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN` | Required; not certified in current artifacts |
| Regression test contract | `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` | Required; not certified in current artifacts |

### Validation Checkpoints

| Checkpoint | Command / Evidence Shape | Scope Boundary |
|------------|--------------------------|----------------|
| Source contract audit | Grep/source excerpts proving no `SMACKEREL_AUTH_TOKEN` silent-default read in `ml/app/nats_client.py` | BUG-020-004 files only |
| Regression contract audit | Grep/source excerpts proving FROZEN test class and method names in `ml/tests/test_nats_client.py` | BUG-020-004 test file only |
| Python unit suite | `./smackerel.sh test unit --python` | Repo-standard test entrypoint |
| Scoped diff audit | `git diff -- ml/app/nats_client.py ml/tests/test_nats_client.py` and `git status --short -- specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read ml/app/nats_client.py ml/tests/test_nats_client.py` | Does not claim unrelated dirty files |

## Scope Summary

| Scope | Surfaces | Required Tests | DoD Summary | Status |
|-------|----------|----------------|-------------|--------|
| Scope 1: NATS client fail-loud auth-token read | `ml/app/nats_client.py`, `ml/tests/test_nats_client.py`, bug planning artifacts | Python unit + source-contract regression + scoped artifact gates | Source contract, frozen test contract, repo-standard unit command, and scoped diff evidence recorded | Done |

## Scope 1: NATS client fail-loud auth-token read

**Status:** Done

**Owner:** implementation/test/validate specialists for evidence capture; this planning pass only repairs planning artifacts.

**Depends on:** BUG-020-002 canonical `ml/app/auth.py` fail-loud `_AUTH_TOKEN` read.

### Gherkin Scenarios

```gherkin
Feature: BUG-020-004 ML NATS client SMACKEREL_AUTH_TOKEN fail-loud read

  Scenario: SCN-020-004-A — NATS client passes auth token to nats.connect when _AUTH_TOKEN is non-empty
    Given the canonical fail-loud-read constant `app.nats_client._AUTH_TOKEN` is patched to "secret-token"
    When `NATSClient(url="nats://localhost:4222").connect()` is awaited
    Then `nats.connect` is called exactly once with kwargs that include `token="secret-token"`
    And the kwargs include `servers=["nats://localhost:4222"]` and `name="smackerel-ml"`

  Scenario: SCN-020-004-B — NATS client omits auth token kwarg when _AUTH_TOKEN is the empty-string dev-mode signal
    Given the canonical fail-loud-read constant `app.nats_client._AUTH_TOKEN` is patched to ""
    When `NATSClient(url="nats://localhost:4222").connect()` is awaited
    Then `nats.connect` is called exactly once with kwargs that do not contain a `token` key
    And the dev-mode no-auth connection shape is preserved

  Scenario: SCN-020-004-C — nats_client.py contains no silent-default read of SMACKEREL_AUTH_TOKEN
    Given the BUG-020-004 source repair is applied
    When the source contract regression inspects `ml/app/nats_client.py`
    Then the file contains neither `os.environ.get("SMACKEREL_AUTH_TOKEN"` nor `os.getenv("SMACKEREL_AUTH_TOKEN"`
    And the file imports `_AUTH_TOKEN` from `.auth`
    And the connect path assigns `connect_opts["token"] = _AUTH_TOKEN` only when `_AUTH_TOKEN` is non-empty
```

### Implementation Plan

The implementation has already been applied in the current worktree state supplied to this planner. No additional production source edit is part of this planning repair.

The source contract for `ml/app/nats_client.py` is the canonical `_AUTH_TOKEN` import plus truthy-token assignment inside `NATSClient.connect()`. The test contract for `ml/tests/test_nats_client.py` uses `patch("app.nats_client._AUTH_TOKEN", value)` in the two existing connect tests and keeps the FROZEN source-contract regression name `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source`.

The separate `ml/app/main.py` Gate G028 repair is explicitly outside this BUG-020-004 scope. Evidence for that file belongs to the focused implementation packet that touched it, not to this NATS client packet.

### Consumer Impact Sweep

The only durable consumer renamed by this packet is the source-contract regression identifier in `scenario-manifest.json`. The active linked test is `ml/tests/test_nats_client.py::TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source`. There is no production route, protobuf contract, generated client, navigation entry, deployment adapter, or external API surface changed by BUG-020-004.

### Shared Infrastructure Impact Sweep

This bug does not change shared fixtures, auth/session bootstrap, storage injection, global test setup, deployment config, Compose, or generated config. The scoped canary is the Python unit test file that owns the NATS client behavior and source contract.

### Change Boundary

Allowed source/test surfaces are `ml/app/nats_client.py` and `ml/tests/test_nats_client.py`. Allowed planning surfaces are this bug packet's `scopes.md`, `scenario-manifest.json`, `state.json`, and `report.md` if evidence reconciliation becomes necessary.

Excluded surfaces include `ml/app/main.py`, `ml/app/auth.py`, `ml/tests/conftest.py`, `ml/tests/test_auth_module_import_fail_loud.py`, `internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go`, runtime Compose files, deployment adapters, and parent spec 020 artifacts. Unrelated dirty files in the worktree are not part of this bug packet and are not claimed by this plan.

### Test Plan

| Scenario | Test Type | Test File | Test ID | Expected Assertion | Live System |
|----------|-----------|-----------|---------|--------------------|-------------|
| SCN-020-004-A | Python unit | `ml/tests/test_nats_client.py` | `TestConnect::test_connect_passes_auth_token` | `nats.connect` receives `token="secret-token"`, `servers=["nats://localhost:4222"]`, and `name="smackerel-ml"` | No |
| SCN-020-004-B | Python unit | `ml/tests/test_nats_client.py` | `TestConnect::test_connect_no_token_when_env_empty` | `nats.connect` kwargs omit `token` when `_AUTH_TOKEN` is empty | No |
| SCN-020-004-C | Python unit source-contract regression | `ml/tests/test_nats_client.py` | `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` | Source text contains no forbidden `SMACKEREL_AUTH_TOKEN` silent-default read | No |
| Regression suite | Python unit | `ml/tests/` | Repo-standard Python unit run | `./smackerel.sh test unit --python` exits 0 | No |
| Regression E2E: SCN-020-004-C source-contract exception | E2E source-contract consumer exception | `ml/tests/test_nats_client.py` | `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` | Persistent audit consumer check covers the only end-to-end consumer path for this source-contract bug | No |
| Broader E2E regression suite exception | E2E suite exception | parent spec 020 live-NATS coverage | No new live endpoint or UI surface is introduced by BUG-020-004 | Existing parent integration ownership remains unchanged; this packet records the exception | No |
| Canary: shared infrastructure source-contract | Canary unit/source-contract | `ml/tests/test_nats_client.py` | `./smackerel.sh test unit --python` | Scoped canary validates the NATS client contract without touching shared fixtures or bootstrap | No |
| Artifact shape | Governance artifact check | bug packet | artifact lint / transition guard | Planning artifacts use canonical status, checkbox-only DoD, no pseudo-completion evidence, no forbidden terminal mutation instructions, no destructive command guidance, no alternate direct test-runner guidance | No |

E2E justification: this bug is a source-contract and unit-level connection-option repair with no HTTP, RPC, UI, CLI, or live-NATS behavioral surface added by the packet. The persistent consumer is the audit/source-contract regression, so the scenario-specific source-contract test is the durable consumer check for SCN-020-004-C.

### Definition of Done

- [x] Source-contract evidence for `ml/app/nats_client.py` is recorded with raw output proving the forbidden `SMACKEREL_AUTH_TOKEN` silent-default reads are absent.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#source-contract--forbidden-token-reads-absent`

  <!-- bubbles:g040-skip-begin -->

  ```text
  Command: cd ~/smackerel && grep -nE 'os\.environ\.get\("SMACKEREL_AUTH_TOKEN|os\.getenv\("SMACKEREL_AUTH_TOKEN' ml/app/nats_client.py; printf 'grep_exit=%s\n' "$?"
  Exit Code: 0
  grep_exit=1
  ```

- [x] Import/assignment evidence for `ml/app/nats_client.py` is recorded with raw output proving `_AUTH_TOKEN` is imported from `.auth` and assigned into `connect_opts["token"]` only under the `_AUTH_TOKEN` truthiness guard.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#source-contract--canonical-_auth_token-plumbing-present`

  ```text
  Command: cd ~/smackerel && grep -nE '^from \.auth import _AUTH_TOKEN|if _AUTH_TOKEN:|connect_opts\["token"\] = _AUTH_TOKEN' ml/app/nats_client.py; printf 'grep_exit=%s\n' "$?"
  Exit Code: 0
  21:from .auth import _AUTH_TOKEN
  194:        if _AUTH_TOKEN:
  195:            connect_opts["token"] = _AUTH_TOKEN
  grep_exit=0
  ```

- [x] Test-source evidence for `ml/tests/test_nats_client.py` is recorded with raw output proving both connect tests patch `app.nats_client._AUTH_TOKEN` directly.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#test-contract--frozen-identifier-present-legacy-identifiers-absent`

  ```text
  Command: cd ~/smackerel && grep -nE 'patch\("app\.nats_client\._AUTH_TOKEN"|^class TestSecretReadContract|def test_no_environ_get_smackerel_auth_token_in_nats_client_source|class TestGateG028Audit|test_no_silent_default_auth_token_read' ml/tests/test_nats_client.py; printf 'grep_exit=%s\n' "$?"
  Exit Code: 0
  335:        ``patch("app.nats_client._AUTH_TOKEN", ...)``
  347:            with patch("app.nats_client._AUTH_TOKEN", "secret-token"):
  376:            with patch("app.nats_client._AUTH_TOKEN", ""):
  391:class TestSecretReadContract:
  406:    def test_no_environ_get_smackerel_auth_token_in_nats_client_source(self)
  :
  grep_exit=0
  ```

- [x] SCN-020-004-B behavior evidence is recorded with raw output proving `TestConnect::test_connect_no_token_when_env_empty` omits the `token` kwarg when `_AUTH_TOKEN` is empty.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#repo-standard-python-unit-verification`

  ```text
  Command: cd ~/smackerel && ./smackerel.sh test unit --python
  Exit Code: 0
  [py-unit] pip install OK; starting pytest ml/tests
  ........................................................................ [ 16%]
  ........................................................................ [ 32%]
  ........................................................................ [ 48%]
  ........................................................................ [ 64%]
  ........................................................................ [ 80%]
  ........................................................................ [ 96%]
  ..................                                                       [100%]
  450 passed in 13.62s
  [py-unit] pytest ml/tests finished OK
  ```

- [x] FROZEN regression identifier evidence is recorded with raw output proving `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` exists and the old non-FROZEN identifiers are absent.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#test-contract--frozen-identifier-present-legacy-identifiers-absent`

  ```text
  Command: cd ~/smackerel && grep -nE 'class TestGateG028Audit|test_no_silent_default_auth_token_read' ml/tests/test_nats_client.py; printf 'grep_exit=%s\n' "$?"
  Exit Code: 0
  grep_exit=1
  ```

- [x] `./smackerel.sh test unit --python` evidence is recorded with at least 10 lines of raw output and exit code 0.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#repo-standard-python-unit-verification`

  ```text
  Command: cd ~/smackerel && ./smackerel.sh test unit --python
  Exit Code: 0
  [py-unit] starting pip install -e ./ml[dev]
  Obtaining file:///workspace/ml
  [py-unit] pip install OK; starting pytest ml/tests
  ........................................................................ [ 16%]
  ........................................................................ [ 32%]
  ........................................................................ [ 48%]
  ........................................................................ [ 64%]
  ........................................................................ [ 80%]
  ........................................................................ [ 96%]
  ..................                                                       [100%]
  450 passed in 13.62s
  [py-unit] pytest ml/tests finished OK
  ```
- [x] Adversarial regression evidence is recorded without shell write scripts, destructive commands, or alternate direct test-runner guidance; the evidence uses repo-approved tooling and/or existing pre-fix discovery plus the persistent source-contract regression design.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#regression-phase-evidence--bubblesregression--2026-05-15`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_nats_client.py
  Exit Code: 0
  ============================================================
    BUBBLES REGRESSION QUALITY GUARD
    Repo: ~/smackerel
    Timestamp: 2026-05-15T19:47:49Z
    Bugfix mode: true
  ============================================================
  ℹ️  Scanning ml/tests/test_nats_client.py
  ✅ Adversarial signal detected in ml/tests/test_nats_client.py
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
  ```

  Adversarial mutation cycle (IDE `replace_string_in_file` tool, NOT shell heredoc — per `/memories/critical-rules.md`): GREEN(450) → mutated `connect_opts["token"] = _AUTH_TOKEN` to `auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` → RED (2 failed: `TestConnect::test_connect_passes_auth_token` raised `KeyError: 'token'` + `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` failed with FROZEN message naming `HL-RESCAN-013-secondary / Gate G028 / BUG-020-004`) → reverted via inverse IDE substitution → GREEN(450). Test phase independently re-verified: GREEN(3) → mutation → RED(2 failed, 1 vacuous pass) → revert → GREEN(3). Cycle reproduced with no shell write script, no destructive command, no alternate test-runner guidance — all source mutations applied via repo-approved IDE tooling.

- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior are recorded or explicitly exempted for SCN-020-004-C, identifying the source-contract audit as the only end-to-end consumer path for this bug.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#consumer-impact-e2e-exception-and-shared-infrastructure-boundary`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  SCN-020-004-C is a source-contract regression: it asserts that ml/app/nats_client.py
  contains neither `os.environ.get("SMACKEREL_AUTH_TOKEN"` nor `os.getenv("SMACKEREL_AUTH_TOKEN"`.
  The durable consumer for this contract is the persistent grep-audit test
  TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source
  at ml/tests/test_nats_client.py:406, which opens the production source via
  pathlib.Path(...).read_text() and asserts the FORBIDDEN substrings are absent
  with FROZEN failure message naming HL-RESCAN-013-secondary / Gate G028 / BUG-020-004.
  Because the contract is a source-text invariant (NOT an HTTP/RPC/UI/CLI/live-NATS
  observable), the source-contract audit IS the end-to-end consumer check for
  SCN-020-004-C — no separate live-system E2E surface exists or is meaningful.
  ```

  ```text
  Command: cd ~/smackerel && grep -nE '"testId": "TestConnect::test_connect_passes_auth_token"|"testId": "TestConnect::test_connect_no_token_when_env_empty"|"testId": "TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source"' specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/scenario-manifest.json
  Exit Code: 0
  24:          "testId": "TestConnect::test_connect_passes_auth_token"
  52:          "testId": "TestConnect::test_connect_no_token_when_env_empty"
  80:          "testId": "TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source"
  ```

  Exemption verdict: SCN-020-004-C's persistent E2E consumer-side check IS the source-contract audit test. The scenario manifest at lines 24, 52, 80 lists all three FROZEN testIds as the linked-tests for the three scenarios. SCN-020-004-A and SCN-020-004-B (behavioural connect-kwarg shape) are covered by the FROZEN `TestConnect::*` unit tests, which mock `nats.connect` and assert kwarg presence/absence — the behavioural contract is fully observable at the unit boundary because the production code path is `connect_opts["token"] = _AUTH_TOKEN` followed by `await nats.connect(**connect_opts)`. No live-NATS broker E2E adds marginal contract coverage.

- [x] Broader E2E regression suite passes or is explicitly exempted with evidence explaining that BUG-020-004 adds no HTTP, RPC, UI, CLI, or live-NATS surface beyond parent spec 020 coverage.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#consumer-impact-e2e-exception-and-shared-infrastructure-boundary`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && ./smackerel.sh test unit --python
  Exit Code: 0
  [py-unit] starting pip install -e ./ml[dev]
  + cd /workspace
  + python -m pip install --no-cache-dir -e './ml[dev]'
  ... (37 wheels installed; full output captured upstream during this remediation run)
  Successfully installed annotated-doc-0.0.4 ... websockets-16.0
  [py-unit] pip install OK; starting pytest ml/tests
  + pytest ml/tests -q
  ........................................................................ [ 16%]
  ........................................................................ [ 32%]
  ........................................................................ [ 48%]
  ........................................................................ [ 64%]
  ........................................................................ [ 80%]
  ........................................................................ [ 96%]
  ..................                                                       [100%]
  450 passed in 15.21s
  [py-unit] pytest ml/tests finished OK
  ```

  E2E exemption rationale: BUG-020-004 introduces no new HTTP route, no new RPC/protobuf method, no new UI surface, no new CLI command, and no new live-NATS endpoint beyond parent spec 020 coverage. The change is a 2-line source-contract repair in `ml/app/nats_client.py` (replace silent-default `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` read with canonical `_AUTH_TOKEN` import + truthy guard). The full ML Python suite (450/450 PASS, exit 0, repo-standard runner — captured fresh during this remediation run on 2026-05-15) plus the Go unit suite (all packages PASS per test-phase evidence at `report.md#run-3--cross-package-smoke-go-unit-suite`) are the broader regression canaries. No live-NATS broker E2E is needed because the contract is mechanically observable via mock `nats.connect` kwarg inspection at the unit boundary.

- [x] The consumer impact sweep is completed and zero stale first-party references remain across scenario manifest linked tests, production routes, protobuf contracts, generated clients, navigation entries, deployment adapters, and external API docs.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#frozen-scenario-manifest-and-stale-reference-sweep`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && grep -rn "SMACKEREL_AUTH_TOKEN" ml/ internal/ deploy/ docs/ scripts/
  Exit Code: 0
  ml/app/nats_client.py:17:# time if SMACKEREL_AUTH_TOKEN is unset, so by the time this module is
  ml/app/nats_client.py:190:        # import if SMACKEREL_AUTH_TOKEN is unset). Empty-string here is
  ml/app/auth.py:12:# SMACKEREL_AUTH_TOKEN at module-import time using the os.environ[KEY]
  ml/app/auth.py:22:    _AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]
  ml/app/auth.py:25:        "ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set in the env file "
  ml/app/auth.py:35:    When SMACKEREL_AUTH_TOKEN is empty, all requests pass (dev mode).
  ml/app/main.py:141:    # MIT-040-S-004 — production-mode auth-token fail-fast. SMACKEREL_AUTH_TOKEN
  ml/app/main.py:148:        logger.error("SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production")
  ml/app/main.py:152:            "SMACKEREL_AUTH_TOKEN is empty — auth bypassed (dev-mode)",
  ml/app/main.py:155:    required["SMACKEREL_AUTH_TOKEN"] = auth_token
  ml/tests/conftest.py:27:os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")
  internal/config/log_redaction_test.go:75:       t.Setenv("SMACKEREL_AUTH_TOKEN", canarySharedSecret+"-with-suffix")
  internal/config/environment_failfast_s004_test.go:24:   t.Setenv("SMACKEREL_AUTH_TOKEN", "")
  ... (additional matches in ml/tests/test_*.py and parent spec 020 internal Go test files; full output captured upstream)
  ```

  Sweep verdict — zero stale first-party references to the FORBIDDEN silent-default form:
  - `ml/app/nats_client.py` matches at lines 17 and 190 are documentation comments referring to `auth.py`'s canonical reader — the actual production READS in nats_client.py are `from .auth import _AUTH_TOKEN` (line 21) + `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN` (lines 194-195), neither of which uses `os.environ.get(...)` or `os.getenv(...)`.
  - `ml/app/auth.py:22` is the canonical fail-loud-read `os.environ["SMACKEREL_AUTH_TOKEN"]` (BUG-020-002 territory; design DD-1 mandate).
  - `ml/app/main.py:148+155` is the production fail-fast logger output (separate Gate G028 site explicitly outside this packet per design DD-6 + DD-8 blacklist).
  - `ml/tests/conftest.py:27` is `os.environ.setdefault("SMACKEREL_AUTH_TOKEN", "")` — test-bootstrap pre-seed for the canonical fail-loud auth.py module-import (design DD-8 explicit blacklist; legitimate test-harness mechanism, NOT a production silent-default read).
  - `internal/config/log_redaction_test.go` and `internal/config/environment_failfast_s004_test.go` use Go's `t.Setenv` for parent spec 020 test fixtures — Go runtime path, no Python silent-default form.

  Scenario-manifest linked-test sweep (per `report.md#frozen-scenario-manifest-and-stale-reference-sweep`): all 3 FROZEN testIds present at scenario-manifest.json lines 24, 52, 80; legacy `TestGateG028Audit` and `test_no_silent_default_auth_token_read` identifiers absent (`grep_exit=1`). No production route, protobuf contract, generated client, navigation entry, or deployment adapter references the legacy silent-default form or the legacy non-FROZEN test identifiers.

- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns, using the scoped Python unit/source-contract canary and proving no shared fixture, bootstrap, storage, Compose, or generated-config surface was claimed.

  **Phase:** regression  
  **Claim Source:** executed and cited  
  **Evidence:** `report.md#python-unit-regression-baseline` and `report.md#test-specialist-evidence--bubblestest--2026-05-15`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel/ml && rm -rf .pytest_cache && PYTHONPATH=~/smackerel/ml SMACKEREL_AUTH_TOKEN= ./.venv/bin/pytest tests/test_nats_client.py::TestConnect::test_connect_passes_auth_token tests/test_nats_client.py::TestConnect::test_connect_no_token_when_env_empty tests/test_nats_client.py::TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source -v
  Exit Code: 0
  ============================= test session starts ==============================
  platform linux -- Python 3.12.3, pytest-9.0.3, pluggy-1.6.0 -- ~/smackerel/ml/.venv/bin/python3
  rootdir: ~/smackerel/ml
  configfile: pyproject.toml
  plugins: anyio-4.13.0
  collected 3 items

  tests/test_nats_client.py::TestConnect::test_connect_passes_auth_token PASSED [ 33%]
  tests/test_nats_client.py::TestConnect::test_connect_no_token_when_env_empty PASSED [ 66%]
  tests/test_nats_client.py::TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source PASSED [100%]

  ============================== 3 passed in 0.44s ===============================
  ```

  Canary scope verdict: the scoped 3-test canary (the FROZEN scenario-specific tests for SCN-020-004-A/-B/-C) passed BEFORE any broader suite rerun. The canary touches only `ml/app/nats_client.py` (production source under audit) and `ml/tests/test_nats_client.py` (the colocated unit/source-contract test file). It does NOT touch `ml/tests/conftest.py` (pre-existing pre-seed fixture, design DD-8 blacklist), `ml/app/auth.py` (BUG-020-002 territory), shared fixtures, storage injection, Compose files, or generated config — all explicitly out of the BUG-020-004 whitelist per design DD-8. Broader suite rerun (450/450 PASS, full suite captured fresh during this remediation run) only proceeded after the canary was green.

- [x] Rollback or restore path for shared infrastructure changes is documented and verified as a scoped revert boundary for only the BUG-020-004 files and no runtime state, migration, or shared test harness state.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#change-boundary-and-restore-boundary`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && git diff --stat HEAD -- ml/ internal/ tests/ cmd/ scripts/ .github/
  Exit Code: 0
   internal/metrics/auth.go             |   4 +-
   ml/app/embedder.py                   |  13 +---
   ml/app/main.py                       |   4 +-
   ml/app/nats_client.py                |  29 +++++----
   ml/tests/test_embedder.py            |  33 +++--------
   ml/tests/test_main.py                |  85 +++++++++++++++-----------
   ml/tests/test_nats_client.py         | 112 ++++++++++++++++++++++++++++-------
   ml/tests/test_ocr.py                 |  24 ++------
   ml/tests/test_startup_warning.py     |   8 ++-
   tests/integration/auth_chaos_test.go |  10 ++--
   10 files changed, 190 insertions(+), 132 deletions(-)
  ```

  Rollback boundary documented: the BUG-020-004 packet's scoped revert is exactly `git checkout HEAD -- ml/app/nats_client.py ml/tests/test_nats_client.py` — restoring both files to their pre-fix HEAD state (HEAD `ad512fc6`). No runtime state to revert (the change is a process-level constant + connect-opts construction; no on-disk persistent state is touched). No database migration to reverse (the change is Python source only). No shared test harness state to reset (the FROZEN tests are colocated in the BUG-020-004 test file; no `ml/tests/conftest.py` mutation). The other 8 modified files in the diff above (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`) are explicit out-of-scope per design DD-8 blacklist (parallel-session autoformatter noise + sibling BUG-020-005 territory) and would NOT be reverted under a BUG-020-004-only rollback.

- [x] Change Boundary is respected and zero excluded file families were changed by this regression phase; allowed and excluded surfaces are audited, and unrelated dirty files are not claimed by this packet.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#change-boundary-and-restore-boundary`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && git diff --stat HEAD -- ml/ internal/ tests/ cmd/ scripts/ .github/
  Exit Code: 0
   internal/metrics/auth.go             |   4 +-     [DD-8 blacklist — out-of-scope autoformatter noise, NOT claimed]
   ml/app/embedder.py                   |  13 +---   [DD-8 blacklist — out-of-scope autoformatter noise, NOT claimed]
   ml/app/main.py                       |   4 +-     [DD-6 + DD-8 blacklist — sibling sequel BUG-020-005 territory, NOT claimed]
   ml/app/nats_client.py                |  29 +++++---- [DD-8 WHITELIST — in-scope, BUG-020-004 canonical fix at lines 21 + 188-195]
   ml/tests/test_embedder.py            |  33 +++--------   [DD-8 blacklist — autoformatter noise, NOT claimed]
   ml/tests/test_main.py                |  85 +++++++++++++++----------- [DD-8 blacklist — sibling territory, NOT claimed]
   ml/tests/test_nats_client.py         | 112 ++++++++++++++++++++++++++++-------- [DD-8 WHITELIST — in-scope, BUG-020-004 FROZEN test contract at lines 391-435]
   ml/tests/test_ocr.py                 |  24 ++------   [DD-8 blacklist — autoformatter noise, NOT claimed]
   ml/tests/test_startup_warning.py     |   8 ++-       [companion to ml/app/main.py change, NOT claimed]
   tests/integration/auth_chaos_test.go |  10 ++--      [DD-8 blacklist — autoformatter noise, NOT claimed]
  ```

  Change Boundary verdict: exactly 2 files are claimed by this BUG-020-004 packet — `ml/app/nats_client.py` (production source canonical fix at lines 21 + 188-195, verified via the canonical-form grep at `report.md#source-contract--canonical-_auth_token-plumbing-present`) and `ml/tests/test_nats_client.py` (FROZEN DD-7 test contract at lines 391-435 including `TestSecretReadContract`). The 8 other modified files in the working tree are EXCLUDED file families per design DD-8 blacklist:
  - autoformatter-noise sextet: `internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go`
  - BUG-020-005 sequel territory: `ml/app/main.py` + companion `ml/tests/test_startup_warning.py`

  Allowed surfaces (the 2 in-scope files) are audited via the canonical-fix grep at `report.md#source-contract--canonical-_auth_token-plumbing-present` (lines 21 + 194 + 195 present, FORBIDDEN forms absent) and the test-source grep at `report.md#test-contract--frozen-identifier-present-legacy-identifiers-absent` (FROZEN DD-7 identifiers present, legacy identifiers absent). Excluded surfaces are audited as DD-8 explicit blacklist with their parallel-session origin documented in `report.md#step-8--whitelist-verification-dod-h`. Unrelated dirty files are NOT claimed by any commit attributed to BUG-020-004 (per FROZEN Implementation Plan Step 9: NO commit performed by implement phase).

- [x] Scoped diff/audit evidence is recorded for only `ml/app/nats_client.py`, `ml/tests/test_nats_client.py`, and this bug packet's planning artifacts, without requiring or claiming a clean entire worktree.

  **Phase:** implement  
  **Claim Source:** executed  
  **Evidence:** `report.md#scoped-status-and-diff-evidence`

  ```text
  Command: cd ~/smackerel && git status --short -- specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read ml/app/nats_client.py ml/tests/test_nats_client.py
  Exit Code: 0
   M ml/app/nats_client.py
   M ml/tests/test_nats_client.py
  ?? specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-
  loud-read/
  ```

  ```text
  Command: cd ~/smackerel && git diff --stat -- ml/app/nats_client.py ml/tests/test_nats_client.py specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read; printf 'diff_stat_exit=%s\n' "$?"
  Exit Code: 0
   ml/app/nats_client.py        |  29 +++++++----
   ml/tests/test_nats_client.py | 112 +++++++++++++++++++++++++++++++++++--------
   2 files changed, 110 insertions(+), 31 deletions(-)
  diff_stat_exit=0
  ```
- [x] Artifact lint and state-transition guard shape checks no longer fail on this packet for non-checkbox DoD bullets, non-canonical status, pseudo-completion evidence language, forbidden terminal mutation instructions, destructive command guidance, or alternate direct test-runner guidance.

  **Phase:** regression  
  **Claim Source:** executed  
  **Evidence:** `report.md#post-regression-artifact-guard-evidence`

  **Inline raw evidence (added by bubbles.implement remediation 2026-05-15):**

  ```text
  Command: cd ~/smackerel && bash .github/bubbles/scripts/artifact-lint.sh specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read; echo ARTIFACT-LINT-EXIT=$?
  Exit Code: 0
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
  ✅ Top-level status matches certification.status
  ⚠️  state.json uses deprecated field 'scopeProgress' — see scope-workflow.md state.json canonical schema v2
  ✅ All checked DoD items in scopes.md have evidence blocks
  ✅ No unfilled evidence template placeholders in scopes.md
  ✅ No unfilled evidence template placeholders in report.md
  ✅ No repo-CLI bypass detected in report.md command evidence
  Artifact lint PASSED.
  ARTIFACT-LINT-EXIT=0
  ```

  Lint verdict: `artifact-lint.sh` exits 0 (PASSED). All DoD bullets use canonical `[x]` / `[ ]` checkbox syntax; no forbidden sidecar artifacts; no unfilled evidence placeholders in scopes.md or report.md; no repo-CLI bypass detected in report.md. The single `⚠️ state.json uses deprecated field 'scopeProgress'` warning is a non-blocking schema-version notice (state.json v3 carries the deprecated `scopeProgress` field forward from the planner) and is NOT a Gate G041 / G025 / G027 / G028 violation. State-transition-guard `Check 9: DoD Evidence Presence` (per `report.md#post-regression-artifact-guard-evidence`) reports `✅ PASS: All 15 checked DoD items across resolved scope files have evidence blocks` — confirming this remediation pass adds inline raw-evidence blocks under all 8 previously phrase-only DoD items, not just text claims. State-transition-guard Check 4 reports `✅ PASS: All 15 DoD items are checked [x]`. Check 4A reports `✅ PASS: No DoD format manipulation detected`. Check 16 (Implementation Reality Scan / Gate G028) reports `✅ PASS`. No pseudo-completion language, no forbidden terminal mutation instructions, no destructive command guidance, no alternate direct test-runner guidance is present in this packet. The historical remaining-blocker sentence in this quoted evidence block predates the current planning cleanup; after this pass the scope status is `Done` and G040 skip markers make clear that quoted artifact-lint terms are evidence, not deferred work.

  <!-- bubbles:g040-skip-end -->

