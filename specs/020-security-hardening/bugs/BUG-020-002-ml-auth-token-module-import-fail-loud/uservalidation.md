# User Validation — BUG-020-002 ml/app/auth.py module-import-time SMACKEREL_AUTH_TOKEN fail-loud (HL-RESCAN-013 / Gate G028)

## Checklist

- [x] AC-1 — Module-import-time read of `SMACKEREL_AUTH_TOKEN` raises a clear
  `RuntimeError` (not silent fallback) when the env var is unset; the message
  names the variable, names the fix path (`run ./smackerel.sh config generate`),
  and includes the `HL-RESCAN-013` + `Gate G028` breadcrumbs.
- [x] AC-2 — Module-import-time read tolerates an empty-string value as a
  legitimate dev-mode auth-bypass signal. The `_AUTH_TOKEN` constant equals `""`
  and `verify_auth()` returns immediately for any inbound request, preserving
  the current dev-stack ergonomics.
- [x] AC-3 — The original `KeyError` is preserved in the traceback chain
  (`raise RuntimeError(...) from exc`) so an operator sees both the underlying
  cause and the actionable wrapping message.
- [x] AC-4 — `ml/tests/conftest.py` pre-seeds `SMACKEREL_AUTH_TOKEN` via
  `os.environ.setdefault(...)` so a clean-shell `pytest ml/tests/` invocation
  succeeds at collection time. The seed is a no-op when a real value is
  exported in the shell or loaded from an env file.
- [x] AC-5 — A persistent in-tree adversarial test
  `ml/tests/test_auth_module_import_fail_loud.py::test_module_import_raises_when_env_var_unset`
  fails RED if anyone reverts line 22 of `ml/app/auth.py` from
  `os.environ["SMACKEREL_AUTH_TOKEN"]` back to the silent-default form. The
  RED→GREEN proof in `report.md` shows the test reporting `1 failed in 0.30s`
  against the surgically-reverted code.
- [x] AC-6 — All 3 cases in
  `ml/tests/test_auth_module_import_fail_loud.py` PASS against the post-fix
  code (UNSET → RuntimeError, EMPTY → succeeds with `_AUTH_TOKEN == ""`,
  REAL → succeeds with `_AUTH_TOKEN == "test-secret-real-value"`).
- [x] AC-7 — All 39 pre-existing tests in
  `ml/tests/test_auth.py` (10) + `ml/tests/test_main.py` (26) +
  `ml/tests/test_startup_warning.py` (3) PASS unchanged after the fix —
  proving the new module-import-time read and the new conftest pre-seed do
  not break any existing assertion.
- [x] AC-8 — The spec 040 MIT-040-S-004 production-vs-dev branching at
  `ml/app/main.py:147-150` (which converts empty + `SMACKEREL_ENV=production`
  to `sys.exit(1)` at lifespan startup) is still exercised by the
  `test_main_s004_production_env_fails_fast_when_auth_token_empty` test and
  PASSES unchanged. The two surfaces (module-import-time fail-loud vs
  startup production fail-fast) layer correctly.

## Acceptance Criteria Verification

| AC | Description | Result | Evidence Reference |
|----|-------------|--------|--------------------|
| AC-1 | UNSET → clear RuntimeError naming variable + fix path | PASS | `report.md` "Module-import RED proof" — RuntimeError traceback shown verbatim, exit code 1 |
| AC-2 | Empty string → dev-mode bypass preserved | PASS | `report.md` "Targeted adversarial suite" — `test_module_import_succeeds_with_empty_value` PASSED |
| AC-3 | Original KeyError preserved in chain | PASS | `report.md` "Module-import RED proof" — traceback shows `The above exception was the direct cause of the following exception` |
| AC-4 | conftest pre-seeds via `setdefault` | PASS | `report.md` "conftest pre-seed under pytest collection" — `cat ml/tests/conftest.py` block + `test_main.py` 26 PASSED proves pre-seed lands |
| AC-5 | Adversarial test fails RED on reversion | PASS | `report.md` "Red→Green proof" — `1 failed in 0.30s` against reverted code, `1 passed in 0.39s` against restored code |
| AC-6 | All 3 adversarial cases PASS post-fix | PASS | `report.md` "Targeted adversarial suite" — `3 passed in 0.29s` |
| AC-7 | Pre-existing canary suites unchanged | PASS | `report.md` "Targeted canary suites" — 10+26+3 = 39 tests PASSED with zero deltas |
| AC-8 | Spec 040 production-vs-dev branching unaffected | PASS | `report.md` `test_main.py` line `test_main_s004_production_env_fails_fast_when_auth_token_empty PASSED` |

## Bounded-Scope Validation

**Claim Source:** executed

The HL-RESCAN-013 finding identified ONE silent-default `os.environ.get(...)`
call in `ml/app/auth.py` (line 11 pre-fix). This packet converts that single
call into the canonical Python fail-loud pattern, adds the test infrastructure
needed to keep pytest collection working under the new contract, and locks
the contract with an adversarial test. The packet does NOT touch:

* `ml/app/main.py:146` — the module-import-time `os.getenv("SMACKEREL_ENV", "development")`
  read, which lands in a sequel rescan finding (not in this packet). The
  spec 040 production fail-fast branching at `ml/app/main.py:147-150` already
  converts empty + production to `sys.exit(1)` at startup, which the existing
  `test_main_s004_production_env_fails_fast_when_auth_token_empty` test
  exercises and continues to pass.
* `ml/app/nats_client.py` — owned by parallel-session WIP (cosmetic edits at
  lines 153 and 168). A separate sequel packet will own any
  `os.environ.get(...)` audit on this file.
* Foreign-owned spec 020 content (`spec.md`, `design.md`, `scopes.md` at the
  parent feature level) — read-only references only. Bug-local artifacts
  under `bugs/BUG-020-002-…/` are owned by `bubbles.devops` per the agent
  contract.
* Go-side core (`cmd/core/`, `internal/`) — already correct (HL-RESCAN-008
  closed the equivalent Go-side audit at commit `7482fb24`).

## Impact on Sanctioned Workflow

The change preserves both sanctioned developer workflows:

1. **`./smackerel.sh up` from a clean shell** — the env file generated by
   `./smackerel.sh config generate` exports `SMACKEREL_AUTH_TOKEN` (with the
   dev-mode placeholder empty value), so the ML sidecar imports cleanly. No
   change to the operator's `up` ergonomics.
2. **`./smackerel.sh test python_unit`** — the new
   `ml/tests/conftest.py` pre-seeds the variable for pytest collection, so a
   developer running pytest directly (without sourcing the env file) does
   not see a collection-time crash. No change to the operator's `test`
   ergonomics.

The change tightens the contract for production deployments (the operator
MUST set `SMACKEREL_AUTH_TOKEN` in `smackerel.yaml` before running
`./smackerel.sh config generate`, otherwise the service crashes loudly at
import time with an actionable error message). This is the intended
behavior under Gate G028 and is documented in
`.github/copilot-instructions.md` "Secrets Management" table.

## Sequel Surfaces Identified (referrals to other packets)

The following surfaces were intentionally scoped OUTSIDE this packet because
they are owned by parallel-session work or are independent rescan findings:

* HL-RESCAN-014 — closed by [`specs/020-security-hardening/bugs/BUG-020-003-helpers-unused-fail-soft-cleanup/`](../BUG-020-003-helpers-unused-fail-soft-cleanup/) — `cmd/core/helpers.go` unused-helper cleanup (`parseFloatEnv`, `parseJSONArrayEnv`, `parseJSONObjectEnv`).
* `ml/app/main.py:146` `os.getenv("SMACKEREL_ENV", "development")` — owned by
  a sequel packet because the existing production fail-fast branching at
  lines 147-150 already provides the operational guarantee; the silent-default
  read is a cosmetic Gate G028 cleanup that requires its own scoped packet.
* `ml/app/nats_client.py` — any `os.environ.get(...)` audit is owned by a
  sequel packet; the file is currently parallel-session WIP and MUST NOT be
  staged with this commit.

## Sign-off

* **Status:** SHIP_IT
* **Verifier:** bubbles.devops (HL-RESCAN-013 owner)
* **Audit verdict:** All 8 ACs verified PASS via executable evidence in
  `report.md`. The RED→GREEN proof confirms the adversarial test is
  non-tautological and will mechanically catch any regression on
  `ml/app/auth.py` line 22.
* **Generic-deployment compliance:** This change is 100% generic. No
  environment-specific content (no real hostnames, IPs, tailnet IDs, or
  operator-specific values) is introduced. The fail-loud message points at
  the generic `./smackerel.sh config generate` workflow and the abstract
  Gate G028 contract.
* **self-hosted posture:** The self-hosted deploy adapter overlay (in the
  separate `knb` repo per the standing user directive) MUST set
  `SMACKEREL_AUTH_TOKEN` in its bundled env file before applying. The
  `./smackerel.sh deploy-target self-hosted apply` flow is unaffected because
  the bundled env file is loaded into the container's env at start time,
  satisfying the new fail-loud read at module import.
