# Design: BUG-020-004 — ML NATS client `SMACKEREL_AUTH_TOKEN` fail-loud read

## Status

Finalized on 2026-05-15 for the implemented `nats_client.py` repair. Done-state certification is blocked by state-transition guard because the full bugfix-fastlane specialist phase set was not executed and a separate repo-wide Gate G028 finding remains in `ml/app/main.py`.

## Root Cause

`ml/app/nats_client.py` originally read `SMACKEREL_AUTH_TOKEN` inside `NATSClient.connect()` with `os.environ.get("SMACKEREL_AUTH_TOKEN", "")`. That was an older pattern from before Smackerel's Gate G028 / NO-DEFAULTS SST policy became binding and before `ml/app/auth.py` exposed the canonical `_AUTH_TOKEN` constant.

The runtime risk was bounded because `app.auth` now fails loudly when the env var is unset, and `app.main` also validates production-empty auth at lifespan startup. The policy defect was still real: connect-time code had a parallel token path and a forbidden silent-default read of an SST-managed value.

## Design Decision

The correct repair is a minimum-touch source-of-truth routing change:

1. `ml/app/nats_client.py` imports `from .auth import _AUTH_TOKEN`.
2. `NATSClient.connect()` uses `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN`.
3. `ml/tests/test_nats_client.py` patches `app.nats_client._AUTH_TOKEN` for token-present and token-empty behavioural cases.
4. `ml/tests/test_nats_client.py` contains a persistent source-contract regression test that reads the live `ml/app/nats_client.py` source and asserts there are zero `os.environ.get/getenv("SMACKEREL_AUTH_TOKEN"...)` call sites and exactly one canonical `_AUTH_TOKEN` import/use path.

This keeps empty-string dev-mode semantics intact: an explicitly empty `_AUTH_TOKEN` omits the NATS `token` kwarg, which matches the existing no-auth dev mode.

## Affected Files

| File | Change Type | Reason |
|---|---|---|
| `ml/app/nats_client.py` | existing dirty implementation observed | Current working tree already imports and consumes `_AUTH_TOKEN`; this close-out validates that state rather than re-editing it. |
| `ml/tests/test_nats_client.py` | edited | Added `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source`; behavioural tests already patch `app.nats_client._AUTH_TOKEN`. |
| `specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/*` | edited | Completed bug packet artifacts and recorded current-session evidence. |

## Regression Design

The regression suite has three scenario locks:

| Scenario | Test | Adversarial Value |
|---|---|---|
| SCN-020-004-A | `TestConnect::test_connect_passes_auth_token` | Fails if `_AUTH_TOKEN` is not wired into `connect_opts["token"]`. |
| SCN-020-004-B | `TestConnect::test_connect_no_token_when_env_empty` | Fails if empty-string dev-mode auth bypass starts passing a `token` kwarg. |
| SCN-020-004-C | `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` | Fails if the forbidden `SMACKEREL_AUTH_TOKEN` silent-default read returns or `_AUTH_TOKEN` plumbing is removed. |

The source-contract test is intentionally local to `ml/tests/test_nats_client.py` instead of a broad SST audit module. This keeps the bugfix narrow and avoids widening scope into unrelated ML env reads (`LLM_PROVIDER`, `LLM_MODEL`, `LLM_API_KEY`, `OLLAMA_URL`) that are explicitly outside this packet.

## Non-Goals

- Do not modify `ml/app/main.py`; its remaining auth-token env read is a separate sequel-audit concern and is the current repo-wide state-transition blocker.
- Do not modify unrelated dirty files in the current working tree.
- Do not change generated config, Compose, deploy adapters, or parent spec 020 artifacts.

## Validation Strategy

- `./smackerel.sh test unit --python` validates the ML unit suite.
- Gate G028 grep validates no forbidden token read remains in `ml/app/nats_client.py`.
- Positive grep validates the `_AUTH_TOKEN` import and assignment.
- `regression-quality-guard.sh` validates no silent-pass bailout patterns and confirms adversarial signal for bugfix mode.
- `artifact-lint.sh` validates the blocked-state bug packet artifacts.
- `state-transition-guard.sh` validates that the packet is not honestly promotable to `done` until the remaining guard blockers are resolved.# Design: BUG-020-004 — ML NATS client `SMACKEREL_AUTH_TOKEN` fail-loud read

## Status

Finalized on 2026-05-15. The design is implemented and validated for the current bug scope.

## Root Cause

`ml/app/nats_client.py` originally read `SMACKEREL_AUTH_TOKEN` inside `NATSClient.connect()` with `os.environ.get("SMACKEREL_AUTH_TOKEN", "")`. That was an older pattern from before Smackerel's Gate G028 / NO-DEFAULTS SST policy became binding and before `ml/app/auth.py` exposed the canonical `_AUTH_TOKEN` constant.

The runtime risk was bounded because `app.auth` now fails loudly when the env var is unset, and `app.main` also validates production-empty auth at lifespan startup. The policy defect was still real: connect-time code had a parallel token path and a forbidden silent-default read of an SST-managed value.

## Design Decision

The correct repair is a minimum-touch source-of-truth routing change:

1. `ml/app/nats_client.py` imports `from .auth import _AUTH_TOKEN`.
2. `NATSClient.connect()` uses `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN`.
3. `ml/tests/test_nats_client.py` patches `app.nats_client._AUTH_TOKEN` for token-present and token-empty behavioural cases.
4. `ml/tests/test_nats_client.py` contains a persistent source-contract regression test that reads the live `ml/app/nats_client.py` source and asserts there are zero `os.environ.get/getenv("SMACKEREL_AUTH_TOKEN"...)` call sites and exactly one canonical `_AUTH_TOKEN` import/use path.

This keeps empty-string dev-mode semantics intact: an explicitly empty `_AUTH_TOKEN` omits the NATS `token` kwarg, which matches the existing no-auth dev mode.

## Affected Files

| File | Change Type | Reason |
|---|---|---|
| `ml/app/nats_client.py` | existing dirty implementation observed | Current working tree already imports and consumes `_AUTH_TOKEN`; this close-out validates that state rather than re-editing it. |
| `ml/tests/test_nats_client.py` | edited | Added `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source`; behavioural tests already patch `app.nats_client._AUTH_TOKEN`. |
| `specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/*` | edited | Completed bug packet artifacts and recorded current-session evidence. |

## Regression Design

The regression suite has three scenario locks:

| Scenario | Test | Adversarial Value |
|---|---|---|
| SCN-020-004-A | `TestConnect::test_connect_passes_auth_token` | Fails if `_AUTH_TOKEN` is not wired into `connect_opts["token"]`. |
| SCN-020-004-B | `TestConnect::test_connect_no_token_when_env_empty` | Fails if empty-string dev-mode auth bypass starts passing a `token` kwarg. |
| SCN-020-004-C | `TestSecretReadContract::test_no_environ_get_smackerel_auth_token_in_nats_client_source` | Fails if the forbidden `SMACKEREL_AUTH_TOKEN` silent-default read returns or `_AUTH_TOKEN` plumbing is removed. |

The source-contract test is intentionally local to `ml/tests/test_nats_client.py` instead of a broad SST audit module. This keeps the bugfix narrow and avoids widening scope into unrelated ML env reads (`LLM_PROVIDER`, `LLM_MODEL`, `LLM_API_KEY`, `OLLAMA_URL`) that are explicitly outside this packet.

## Non-Goals

- Do not modify `ml/app/main.py`; its remaining auth-token env read is a separate sequel-audit concern.
- Do not modify unrelated dirty files in the current working tree.
- Do not change generated config, Compose, deploy adapters, or parent spec 020 artifacts.

## Validation Strategy

- `./smackerel.sh test unit --python` validates the ML unit suite.
- Gate G028 grep validates no forbidden token read remains in `ml/app/nats_client.py`.
- Positive grep validates the `_AUTH_TOKEN` import and assignment.
- `regression-quality-guard.sh` validates no silent-pass bailout patterns and confirms adversarial signal for bugfix mode.
- `artifact-lint.sh` and `state-transition-guard.sh` validate the bug packet's Bubbles artifacts.# Design: BUG-020-004 — `ml/app/nats_client.py` `SMACKEREL_AUTH_TOKEN` fail-loud read at NATS connect time (HL-RESCAN-013-secondary)

> **Status:** Finalized by `bubbles.design` 2026-05-15. Inherits the discover-phase seed from `bubbles.bug` (HEAD `ad512fc6`). DD-1 through DD-9 below are FROZEN — the implement phase applies them verbatim. The frozen test contract (DD-7) names the exact test file path, parent class names, and method names that bubbles.plan and bubbles.implement MUST honour. The frozen file constraint set (DD-8) names the exhaustive whitelist of files the implement phase MAY touch and the exhaustive blacklist of files it MUST NOT touch. The Open Questions left by bubbles.bug have been resolved (see "Resolution of Discover-Phase Open Questions" below).

## Problem Recap

`ml/app/nats_client.py` `NATSClient.connect()` lines 183-186 (HEAD `ad512fc6`) read the `SMACKEREL_AUTH_TOKEN` env var via the FORBIDDEN silent-default form `os.environ.get("SMACKEREL_AUTH_TOKEN", "")`, bypassing the canonical fail-loud-read constant `_AUTH_TOKEN` defined in `ml/app/auth.py:22-29` (added by the primary HL-RESCAN-013 close-out, BUG-020-002, at HEAD `0c67122e`). This is the deferred-sequel call site that BUG-020-002's spec.md → "Out of Scope" explicitly named. See [`spec.md`](./spec.md) for the line-precise pre-fix evidence.

## Root Cause

The primary HL-RESCAN-013 close-out (BUG-020-002, commit `0c67122e`, 2026-05-14) converted `ml/app/auth.py:11` from the FORBIDDEN silent-default `_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` to the canonical Python fail-loud pattern `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in `try / except KeyError → raise RuntimeError from exc` at module-import time. That fix correctly hardened the highest-priority site (the canonical auth dependency consumed by every non-health route via `Depends(verify_auth)`) but explicitly **deferred** two other Gate G028 violations on the same env var to sequel packets. From [BUG-020-002's spec.md "Out of Scope" section](../BUG-020-002-ml-auth-token-module-import-fail-loud/spec.md) (verbatim, second bullet):

> `ml/app/nats_client.py` line 180 — runs INSIDE `connect()` which is invoked AFTER `_check_required_config()` validates the env, so the silent default there is functionally redundant. ... Scoped to a sequel packet to be filed once the parallel session merges.

This bug packet (`BUG-020-004`) is that deferred sequel for the connect-time call site. The 2026-05-15 home-lab readiness re-scan (run AFTER the BUG-020-002 close-out) re-audited the ML sidecar surface and found the deferred site at `ml/app/nats_client.py:184` (line number against HEAD `ad512fc6`) still pre-fix, exactly as BUG-020-002 had documented.

The reason BUG-020-002 deferred this site: the parallel session that produced today's working-tree autoformatter noise (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go`) had concurrent uncommitted edits to `ml/app/nats_client.py` (the multi-line `raise RuntimeError(...)` collapse at lines 153-156 and 168-170 visible in `git diff HEAD`). Bundling the fail-loud fix together with that autoformatter collapse would have mixed two unrelated changes; sequel-packet treatment keeps the change-boundary discipline clean.

## Approach

**Two-file minimum-touch surgical fix** scoped to `ml/app/nats_client.py` (production code) and `ml/tests/test_nats_client.py` (companion tests). A NEW grep-contract regression test is added to `ml/tests/test_nats_client.py` as a new `TestSecretReadContract` test class (per DD-7 below — the standalone-vs-shared-audit-module decision is resolved in favour of in-file standalone for this packet).

### Change 1 — `ml/app/nats_client.py` (the fix)

- **Add** at the top of the file (immediately after the existing third-party `nats` import block, before the existing `from .metrics import ...` / `from .url_validator import ...` / `from .validation import ...` first-party import block) a comment block + `from .auth import _AUTH_TOKEN` import. The comment names `HL-RESCAN-013` + `Gate G028` and explains the rationale for re-using the canonical constant rather than re-reading `os.environ`. The import uses the **relative form** (`from .auth import _AUTH_TOKEN`, NOT `from app.auth import _AUTH_TOKEN`) per Open Question Q-2 resolution below — matches the existing first-party import style at the top of the file (`from .metrics`, `from .url_validator`, `from .validation`).
- **Replace** lines 183-186 (the 4-line `# Token authentication ... \n auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "") \n if auth_token: \n connect_opts["token"] = auth_token` block) with an explanatory comment + 2-line `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN` block. The replacement comment names `HL-RESCAN-013 / Gate G028`, references the canonical fail-loud read in `auth.py`, and explicitly documents the empty-string dev-mode auth-bypass semantics.

The exact post-fix block (verbatim from the working-tree draft, which the implement phase MUST apply identically):

```python
        # Token authentication — mirrors Go core's NATS auth enforcement.
        # HL-RESCAN-013 / Gate G028: re-use the canonical fail-loud-read
        # _AUTH_TOKEN constant from auth.py (which raises RuntimeError at
        # import if SMACKEREL_AUTH_TOKEN is unset). Empty-string here is
        # the legitimate dev-mode auth-bypass signal — the NATS connect
        # call simply omits the `token` kwarg and the dev NATS server
        # accepts the connection without auth.
        if _AUTH_TOKEN:
            connect_opts["token"] = _AUTH_TOKEN
```

The exact post-fix top-of-file import block (verbatim from the working-tree draft, which the implement phase MUST apply identically):

```python
# HL-RESCAN-013 / Gate G028 (NO-DEFAULTS / fail-loud SST policy) — re-use
# the canonical fail-loud-read module-level constant from auth.py instead
# of re-reading os.environ here. auth.py raises RuntimeError at import
# time if SMACKEREL_AUTH_TOKEN is unset, so by the time this module is
# imported the constant is guaranteed to be defined (empty string is the
# dev-mode auth-bypass signal honoured by both verify_auth() and the
# NATS server's no-auth dev mode).
from .auth import _AUTH_TOKEN
```

### Change 2 — `ml/tests/test_nats_client.py` (the rewritten existing tests)

- **Rewrite** `TestConnect.test_connect_passes_auth_token` to switch the patch path from `patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "secret-token", ...})` to a nested `patch("app.nats_client._AUTH_TOKEN", "secret-token")` PLUS a `patch.dict("os.environ", {"NATS_MAX_RECONNECT_ATTEMPTS": "-1", "NATS_RECONNECT_TIME_WAIT_SECONDS": "2"})` (the SMACKEREL_AUTH_TOKEN entry is removed from the env-dict and replaced by the constant patch). The behavioural assertion `call_kwargs["token"] == "secret-token"` is preserved verbatim. The test docstring is augmented to name `HL-RESCAN-013` / `Gate G028` and to explain the patch-path switch rationale.
- **Rewrite** `TestConnect.test_connect_no_token_when_env_empty` symmetrically — switch from `patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "", ...})` to nested `patch("app.nats_client._AUTH_TOKEN", "")` + `patch.dict("os.environ", {"NATS_MAX_RECONNECT_ATTEMPTS": "-1", "NATS_RECONNECT_TIME_WAIT_SECONDS": "2"}, clear=False)`. The behavioural assertion `"token" not in call_kwargs` is preserved verbatim.

### Change 3 — `ml/tests/test_nats_client.py` (the NEW grep-contract regression class)

- **Append** a new top-level test class `TestSecretReadContract` to `ml/tests/test_nats_client.py` (placed after the existing `TestConnectReconnectContract` class). The class declares one test method `test_no_environ_get_smackerel_auth_token_in_nats_client_source` that opens `ml/app/nats_client.py` from disk, reads its full text, and asserts the FORBIDDEN substrings `os.environ.get("SMACKEREL_AUTH_TOKEN"` and `os.getenv("SMACKEREL_AUTH_TOKEN"` are BOTH absent. The assertion message names `HL-RESCAN-013-secondary`, `Gate G028`, and `BUG-020-004` so a future failure is self-attesting.

The grep-contract test is defined IN-FILE (not in a shared `ml/tests/test_sst_audit.py` module — see Open Question Q-1 resolution below) because:

1. The test is scoped to a single source file; a shared module would obscure the per-source-file scoping.
2. The test belongs with the behavioural tests for the same source file (co-location aids future maintainers searching for `ml/app/nats_client.py` regression coverage).
3. A future BUG-020-005 sequel packet for `ml/app/main.py:146` would add its own per-file `TestSecretReadContract` class to `ml/tests/test_main.py` — the per-file pattern scales without requiring a shared registry module.

## Design Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| **DD-1** | Re-use the canonical `_AUTH_TOKEN` constant from `app.auth` via `from .auth import _AUTH_TOKEN` instead of re-reading `os.environ` at NATS connect time | `auth.py` already raises `RuntimeError` at module-import time if `SMACKEREL_AUTH_TOKEN` is unset (BUG-020-002, HEAD `0c67122e`, lines 22-29). By the time `nats_client.py` is imported (which happens at FastAPI lifespan startup via `from .nats_client import NATSClient` in `ml/app/main.py:14`), the constant is guaranteed-defined or the process has already crashed loud. Re-reading `os.environ` at NATS connect time is therefore (a) FORBIDDEN by Gate G028 NO-DEFAULTS / fail-loud SST policy and (b) creates a parallel auth-token data path that bypasses any future format / length / decode check that might be added to the canonical read. Routing through `_AUTH_TOKEN` eliminates the parallel path. |
| **DD-2** | Preserve dev-mode bypass semantics — empty-string `_AUTH_TOKEN` (legitimate dev signal) → omit `token=` kwarg from `nats.connect()` | The pre-fix block at HEAD `ad512fc6` lines 185-186 (`if auth_token: connect_opts["token"] = auth_token`) already implemented the dev-mode bypass — empty-string env-var → no token kwarg → dev NATS server accepts the connection without auth. The fix preserves that semantic verbatim by using `if _AUTH_TOKEN:` (truthiness check; empty string is falsy in Python). This matches the dev-mode bypass already honoured by `verify_auth()` in `ml/app/auth.py:35` (`if not _AUTH_TOKEN: return`). The dev-vs-production branching that converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)` lives in `ml/app/main.py:_check_required_config` (out of scope for this packet — a future BUG-020-005 sequel). |
| **DD-3** | Test patching strategy — patch `app.nats_client._AUTH_TOKEN` directly via `unittest.mock.patch("app.nats_client._AUTH_TOKEN", value)` rather than mutating `os.environ` | Reflects the new architecture: `_AUTH_TOKEN` is now read at module import (in `app.auth`, then re-imported into `app.nats_client`), NOT at connect time. Patching `os.environ` in the test would NO LONGER affect the value `nats_client.py` consumes — the connect path consults the cached module-level constant. Patching the constant directly (a) is the architecturally correct test posture, (b) makes the test robust against future refactors that move the read further away from connect time, and (c) implicitly verifies the import is present (the patch path `app.nats_client._AUTH_TOKEN` ONLY resolves if `from .auth import _AUTH_TOKEN` is at the top of `nats_client.py`). The existing `TestConnectReconnectContract` class in the same test file already uses the same mock-`nats.connect` AsyncMock pattern; this packet's tests reuse that pattern. |
| **DD-4** | Adversarial regression guard — add a contract test asserting the FORBIDDEN substrings `os.environ.get("SMACKEREL_AUTH_TOKEN"` and `os.getenv("SMACKEREL_AUTH_TOKEN"` are absent from `ml/app/nats_client.py` source | The behavioural tests (DD-3) prove the right value is plumbed through, but they would NOT necessarily fail if a future regression introduced a vestigial `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` read elsewhere in the file (e.g., a copy-paste into a new method that does not affect `connect()`). The grep-contract test is the mechanical regression guard for the FORBIDDEN form: if any future agent re-introduces the substring anywhere in `nats_client.py` source, the test fails RED with a message naming `BUG-020-004` so the maintainer can navigate back to this packet. The test reads the source file from disk (not via `inspect.getsource`) so it catches comments and docstrings too. The two FORBIDDEN substrings cover both `os.environ.get(...)` and `os.getenv(...)` Python conventions. |
| **DD-5** | Tautology-free adversarial coverage — the grep-contract test (SCN-020-004-C) and the behavioural tests (SCN-020-004-A and SCN-020-004-B) cover orthogonal concerns and cannot both pass tautologically against a regression | SCN-020-004-A asserts `nats.connect(token="secret-token")` is called when `_AUTH_TOKEN="secret-token"` — proves the constant is plumbed through. SCN-020-004-B asserts `nats.connect()` is called WITHOUT a `token=` kwarg when `_AUTH_TOKEN=""` — proves the dev-mode bypass is preserved. SCN-020-004-C asserts the FORBIDDEN substring is mechanically absent from the source file — proves the silent-default form is gone. A regression that re-introduced `auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` and assigned it to `connect_opts["token"] = auth_token` would FAIL SCN-020-004-C (substring present) AND would FAIL one of SCN-020-004-A / SCN-020-004-B (because `_AUTH_TOKEN` patches would no longer affect the connect path). A regression that always passed `connect_opts["token"] = _AUTH_TOKEN or ""` would FAIL SCN-020-004-B (token kwarg present even when `_AUTH_TOKEN` is empty). No single-line regression silently passes all three. Per [`bubbles-test-integrity` skill](../../../../.github/skills/bubbles-test-integrity/SKILL.md) — adversarial-regression-coverage requirement satisfied. |
| **DD-6** | Out-of-scope deliberately — do NOT touch `ml/app/auth.py`; do NOT touch `ml/app/main.py:146` (`_check_required_config()`); do NOT bundle the working-tree autoformatter-only changes (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go`) into this packet | `ml/app/auth.py` is already correct on HEAD post-BUG-020-002 (lines 22-29 implement the canonical fail-loud try/except KeyError → RuntimeError pattern). This packet only re-imports the existing `_AUTH_TOKEN` constant. `ml/app/main.py:146` (`_check_required_config()`) uses the silent-default form `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` but its production branch already converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)`, so the runtime risk is bounded; converting it to the fail-loud form is a separate audit-cleanliness improvement scoped to a future BUG-020-005 sequel packet (would be filed once the parallel session merges). The 6 working-tree autoformatter-noise files (`internal/metrics/auth.go` godoc indentation; `ml/app/embedder.py` line-length; the four test files line-length; `tests/integration/auth_chaos_test.go` gofmt comment-alignment + trailing newline) are unrelated to `HL-RESCAN-013-secondary` — they originated from a separate parallel session and bundling them into this packet would mix two unrelated changes and violate change-boundary discipline. The implement phase MUST stage ONLY the two files in scope (`ml/app/nats_client.py` + `ml/tests/test_nats_client.py`) for the BUG-020-004 commit. |
| **DD-7** | FROZEN test contract — exact test file path, parent class names, and method names that bubbles.plan and bubbles.implement MUST honour | See "Frozen Test Contract" section below. The contract names exact symbols so traceability links from `scenario-manifest.json` `linkedTests[*].testId` resolve unambiguously and so bubbles.implement's test-evidence grep can target the exact symbols. |
| **DD-8** | FROZEN file constraint set — exhaustive whitelist (files the implement phase MAY touch) and exhaustive blacklist (files it MUST NOT touch) | See "Frozen File Constraint Set" section below. The whitelist is exactly two files; the blacklist is the working-tree autoformatter-noise sextet plus the explicit-out-of-scope production files. The implement phase MUST run `git diff --name-only HEAD` after applying the fix and verify the diff matches the whitelist exactly (no extras, no surprises). |
| **DD-9** | Grep-contract test placement — in-file `TestSecretReadContract` class in `ml/tests/test_nats_client.py` (NOT a shared `ml/tests/test_sst_audit.py` module) | Resolves Open Question Q-1 left by bubbles.bug. Per-file scoping wins because (a) the test is single-file scoped (asserts substring absence in ONE file) — a shared multi-file module would obscure that scoping, (b) co-location with the behavioural tests for the same source file aids future maintainers grepping for `ml/app/nats_client.py` regression coverage, (c) the per-file pattern scales without requiring a shared registry — a future BUG-020-005 sequel packet for `ml/app/main.py:146` would add its own per-file `TestSecretReadContract` class to `ml/tests/test_main.py`. The shared-audit-module alternative was rejected because it conflates the per-source-file regression-guard responsibility with a registry-style "audit all files" responsibility, and because there is no current need for a multi-file audit registry (each Gate G028 violation is closed in its own per-file packet). |

## Frozen Test Contract (DD-7)

The implement phase MUST honour these exact symbols. bubbles.plan MUST NOT rename them in `scopes.md` or `scenario-manifest.json` `linkedTests[*].testId` without a documented reason and a re-routed transition request back to bubbles.design.

| Scenario | Test File | Test Class | Test Method | New / Modified |
|----------|-----------|------------|-------------|----------------|
| SCN-020-004-A | `ml/tests/test_nats_client.py` | `TestConnect` | `test_connect_passes_auth_token` | **Modified** — patch path switches from `os.environ` to `app.nats_client._AUTH_TOKEN`; behavioural assertion `call_kwargs["token"] == "secret-token"` preserved |
| SCN-020-004-B | `ml/tests/test_nats_client.py` | `TestConnect` | `test_connect_no_token_when_env_empty` | **Modified** — patch path switches from `os.environ` to `app.nats_client._AUTH_TOKEN`; behavioural assertion `"token" not in call_kwargs` preserved |
| SCN-020-004-C | `ml/tests/test_nats_client.py` | `TestSecretReadContract` (NEW) | `test_no_environ_get_smackerel_auth_token_in_nats_client_source` | **New** — opens `ml/app/nats_client.py` from disk, asserts `os.environ.get("SMACKEREL_AUTH_TOKEN"` AND `os.getenv("SMACKEREL_AUTH_TOKEN"` are both absent. Failure message names `HL-RESCAN-013-secondary`, `Gate G028`, and `BUG-020-004`. |

**Test method signature contract (frozen — implement phase applies verbatim):**

```python
class TestSecretReadContract:
    """HL-RESCAN-013-secondary / Gate G028 / BUG-020-004 — adversarial
    grep-contract regression that mechanically locks the absence of
    the FORBIDDEN silent-default os.environ.get("SMACKEREL_AUTH_TOKEN", "")
    form (and the os.getenv equivalent) from ml/app/nats_client.py source.

    Reverting the fix in ml/app/nats_client.py — i.e. re-introducing
    `auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` anywhere
    in the file — would cause this test to FAIL with the failure
    message naming BUG-020-004, so a future maintainer can navigate
    back to this packet for context.
    """

    def test_no_environ_get_smackerel_auth_token_in_nats_client_source(self):
        # ... opens ml/app/nats_client.py from disk (path resolved relative
        # to this test file's location, NOT via inspect.getsource — so the
        # check catches comments and docstrings too).
        # ... asserts FORBIDDEN substrings 'os.environ.get("SMACKEREL_AUTH_TOKEN"'
        # AND 'os.getenv("SMACKEREL_AUTH_TOKEN"' are BOTH absent.
        # ... assertion failure message names HL-RESCAN-013-secondary,
        # Gate G028, BUG-020-004.
```

**Tautology-freedom contract:** the three sub-tests A/B/C cover orthogonal regression vectors per DD-5 above. No `pytest.skip`, no early-return-on-condition, no failure-condition bailout — the tests fail RED on every adversarial path.

## Frozen File Constraint Set (DD-8)

### Whitelist — files the implement phase MAY touch (exhaustive)

| File | Change Type | Rationale |
|------|-------------|-----------|
| `ml/app/nats_client.py` | edit | Add `from .auth import _AUTH_TOKEN` import block at top of file; replace lines 183-186 silent-default block with `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN` block |
| `ml/tests/test_nats_client.py` | edit | Rewrite `TestConnect.test_connect_passes_auth_token` and `TestConnect.test_connect_no_token_when_env_empty` patch paths; append new `TestSecretReadContract` class with `test_no_environ_get_smackerel_auth_token_in_nats_client_source` method |

### Blacklist — files the implement phase MUST NOT touch (exhaustive)

| File | Reason |
|------|--------|
| `ml/app/auth.py` | Already correct on HEAD post-BUG-020-002 (HEAD `0c67122e`); this packet only re-imports the existing `_AUTH_TOKEN` constant |
| `ml/app/main.py` | `_check_required_config()` line 146 silent-default is a separate validation path with its own contract (production branch already exits); scoped to future BUG-020-005 sequel packet |
| `ml/tests/conftest.py` | Pre-seeds `SMACKEREL_AUTH_TOKEN=""` for pytest collection (BUG-020-002, DD-3); already correct, no change needed |
| `ml/tests/test_auth_module_import_fail_loud.py` | BUG-020-002 adversarial suite for `auth.py` module-import-time fail-loud read; out of scope for this packet |
| `internal/metrics/auth.go` | Working-tree godoc indentation-only autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `ml/app/embedder.py` | Working-tree line-length-only autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `ml/tests/test_embedder.py` | Working-tree line-length-only autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `ml/tests/test_main.py` | Working-tree line-length-only autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `ml/tests/test_ocr.py` | Working-tree line-length-only autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `tests/integration/auth_chaos_test.go` | Working-tree gofmt comment-alignment + trailing-newline autoformatter noise from a separate parallel session; unrelated to HL-RESCAN-013-secondary |
| `cmd/core/wiring.go` | Go-side equivalent fail-loud read already correct (HL-RESCAN-008 / commit `7482fb24`); outside the Python ML sidecar boundary |
| `scripts/commands/config.sh` | SST emission of `SMACKEREL_AUTH_TOKEN` already correct (verified by BUG-020-002) |
| `.github/copilot-instructions.md` | Already documents the Python fail-loud pattern correctly; no change needed |
| `.github/instructions/smackerel-no-defaults.instructions.md` | Already documents Gate G028 correctly; no change needed |
| `specs/020-security-hardening/spec.md` / `design.md` / `scopes.md` / `state.json` / `report.md` / `uservalidation.md` | Foreign-owned parent-spec content; outside this bug packet's edit scope |

### Implement-phase whitelist verification command (frozen)

After applying the fix, the implement phase MUST run:

```bash
git diff --name-only HEAD -- ml/ internal/ tests/ cmd/ scripts/ .github/
# Expected output (exact, in this order or alphabetical):
#   ml/app/nats_client.py
#   ml/tests/test_nats_client.py
```

If the output contains any other file (especially any from the blacklist sextet), the implement phase MUST `git restore` the noise files BEFORE committing. The bug packet's `report.md` "Implementation Evidence" section MUST capture the verified `git diff --name-only HEAD` output as inline evidence.

## Resolution of Discover-Phase Open Questions (bubbles.bug → bubbles.design)

The discover-phase `design.md` initial artifact left two open questions for bubbles.design to resolve. Both are resolved here:

### Q-1: Standalone test class in `test_nats_client.py` vs shared `test_sst_audit.py` module?

**Resolved by DD-9: in-file `TestSecretReadContract` class in `ml/tests/test_nats_client.py`.**

Rationale: per-file scoping wins because the test is single-file scoped (asserts substring absence in exactly one source file). Co-location with the behavioural tests for the same source file aids maintainability. The per-file pattern scales — a future BUG-020-005 sequel for `ml/app/main.py:146` would add a `TestSecretReadContract` class to `ml/tests/test_main.py`. The shared-audit-module alternative was rejected for conflating per-file regression-guard responsibility with a registry-style "audit all files" responsibility.

### Q-2: Relative import `from .auth import _AUTH_TOKEN` vs absolute `from app.auth import _AUTH_TOKEN`?

**Resolved: relative form `from .auth import _AUTH_TOKEN`.**

Rationale: matches the existing first-party import style at the top of `ml/app/nats_client.py` (`from .metrics import llm_tokens_used, processing_latency, sanitize_model`, `from .url_validator import validate_fetch_url`, `from .validation import (...)`). The file's existing convention is relative imports for first-party `app/` modules. Mixing absolute and relative would be inconsistent. The working-tree draft already uses the relative form — this resolution ratifies that choice.

## Tech-agnostic Gherkin (BDD)

```gherkin
Feature: BUG-020-004 — ml/app/nats_client.py auth token plumbed through canonical fail-loud constant

  Background:
    Given the canonical fail-loud-read constant `_AUTH_TOKEN` is defined in `ml/app/auth.py` (BUG-020-002)
    And `ml/app/nats_client.py` re-imports `_AUTH_TOKEN` from `app.auth` at the top of the module
    And the legitimate dev-mode auth-bypass signal is `_AUTH_TOKEN == ""`

  Scenario: SCN-020-004-A — NATS connect passes auth token to the broker when constant is non-empty
    Given the canonical fail-loud-read constant `app.nats_client._AUTH_TOKEN` is patched to a non-empty value
    When the NATS client's connect coroutine is awaited
    Then the underlying `nats.connect` call is invoked exactly once with a `token` kwarg equal to the patched value

  Scenario: SCN-020-004-B — NATS connect omits auth token kwarg when constant is the empty-string dev-mode signal
    Given the canonical fail-loud-read constant `app.nats_client._AUTH_TOKEN` is patched to the empty string
    When the NATS client's connect coroutine is awaited
    Then the underlying `nats.connect` call is invoked exactly once with NO `token` kwarg
    And the dev-mode auth-bypass semantics are preserved (NATS server accepts the connection without auth)

  Scenario: SCN-020-004-C — `ml/app/nats_client.py` source contains no FORBIDDEN silent-default read of SMACKEREL_AUTH_TOKEN
    Given the working tree has the BUG-020-004 fix applied
    When the test reads `ml/app/nats_client.py` from disk
    Then the FORBIDDEN substring `os.environ.get("SMACKEREL_AUTH_TOKEN"` is absent from the source
    And the FORBIDDEN substring `os.getenv("SMACKEREL_AUTH_TOKEN"` is absent from the source
    And a regression that re-introduced either substring would cause the contract test to fail RED with a message naming `BUG-020-004`
```

## Validation Strategy

| Check | Command | Expected |
|-------|---------|----------|
| Pre-fix grep evidence (FORBIDDEN form present at HEAD `ad512fc6`) | `git show HEAD:ml/app/nats_client.py \| grep -n 'SMACKEREL_AUTH_TOKEN'` | exit 0; one match at line 184 |
| Post-fix grep evidence (FORBIDDEN form absent) | `grep -nE 'os\.(environ\.get\|getenv)\("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py` | exit 1 (no matches) |
| Post-fix import present | `grep -nE '^from \.auth import _AUTH_TOKEN' ml/app/nats_client.py` | exit 0; one match in top-of-file import block |
| Post-fix conditional present | `grep -nE 'if _AUTH_TOKEN:' ml/app/nats_client.py` | exit 0; one match inside `NATSClient.connect()` |
| Post-fix conditional body present | `grep -nE 'connect_opts\["token"\] = _AUTH_TOKEN' ml/app/nats_client.py` | exit 0; one match immediately after `if _AUTH_TOKEN:` |
| Test patch path adoption | `grep -nE 'patch\("app\.nats_client\._AUTH_TOKEN"' ml/tests/test_nats_client.py` | exit 0; at least 2 matches (one per modified `TestConnect` test) |
| Grep-contract test present | `grep -nE 'class TestSecretReadContract' ml/tests/test_nats_client.py` | exit 0; one match |
| Grep-contract test method present | `grep -nE 'def test_no_environ_get_smackerel_auth_token_in_nats_client_source' ml/tests/test_nats_client.py` | exit 0; one match |
| Python unit suite green | `./smackerel.sh test unit --python` | exit 0; all `ml/tests/test_nats_client.py` tests PASS; BUG-020-002 adversarial suite (`ml/tests/test_auth_module_import_fail_loud.py`) continues to PASS unchanged; no other ML test regresses |
| File constraint set verified | `git diff --name-only HEAD -- ml/ internal/ tests/ cmd/ scripts/ .github/` | output is exactly `ml/app/nats_client.py` + `ml/tests/test_nats_client.py` (no autoformatter-noise files staged) |
| BUG-020-002 adversarial suite still green | `./smackerel.sh test unit --python -- ml/tests/test_auth_module_import_fail_loud.py` | exit 0; all 3 tests PASS |

## Severity Disposition (carried from spec.md)

P3 — LOW. The runtime contract for `SMACKEREL_AUTH_TOKEN` is already enforced once `app.auth` is imported (BUG-020-002 fail-loud RuntimeError at module-import time) and once `_check_required_config()` runs (sys.exit(1) at lifespan startup if empty + production). By the time `NATSClient.connect()` runs, the env var is guaranteed defined or the process has already crashed. The fix is therefore primarily an **audit-cleanliness + defense-in-depth-at-every-boundary** improvement, not a runtime exposure fix. The packet remains worth filing because (a) Gate G028 NO-DEFAULTS / fail-loud SST policy bans the silent-default form at every Python read site, defense-in-depth elsewhere is not a substitute, and (b) the parallel auth-token data path bypasses any future change to the canonical read pattern.

## Cross-Agent Routing

- **Discover (complete):** `bubbles.bug` — created the bug packet skeleton (7 artifacts seeded with verified pre-fix evidence at HEAD `ad512fc6`). Routed to `bubbles.design` via `TR-BUG-020-004-001`.
- **Design (this entry):** `bubbles.design` — refined this design.md from the initial artifact to FROZEN (DD-1 through DD-9 + frozen test contract + frozen file constraint set + 2 open questions resolved). MUST NOT modify `ml/app/nats_client.py`, `ml/tests/test_nats_client.py`, or any blacklisted file. Routes to `bubbles.plan` next via new `TR-BUG-020-004-002`.
- **Plan:** `bubbles.plan` — finalizes scopes.md (already drafted by `bubbles.bug`); confirms the test-plan table; confirms the Tiered DoD; confirms `scenario-manifest.json` `linkedTests[*].testId` matches the FROZEN test contract above. Routes to `bubbles.implement` next.
- **Implement:** `bubbles.implement` — applies the two-file fix per the FROZEN file constraint set; runs the validation grep commands; captures `git diff --name-only HEAD` evidence proving no blacklist contamination. Routes to `bubbles.test` next.
- **Test → Validate → Audit → Finalize:** standard bugfix-fastlane chain.

## Acceptance Mapping

| AC (from spec.md) | Met by |
|-------------------|--------|
| AC-1 (`os.environ.get("SMACKEREL_AUTH_TOKEN", ...)` removed) | DD-1 + DD-4 + Validation Strategy → post-fix grep |
| AC-2 (Auth-token plumbing routes through `_AUTH_TOKEN`) | DD-1 + Approach → Change 1 + Validation Strategy → post-fix import / conditional / body grep |
| AC-3 (Tests patch `app.nats_client._AUTH_TOKEN` directly) | DD-3 + Approach → Change 2 + Validation Strategy → test patch path adoption grep |
| AC-4 (All ML unit tests pass green) | Validation Strategy → Python unit suite green |
| AC-5 (Gate G028 audit grep clean for `ml/app/nats_client.py`) | DD-1 + DD-4 + Validation Strategy → post-fix grep |

## Cross-References

- Bug packet root: `specs/020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/`
- Bug specification: [`spec.md`](./spec.md)
- Scope structure: [`scopes.md`](./scopes.md)
- Scenario contract registry: [`scenario-manifest.json`](./scenario-manifest.json)
- User validation checklist: [`uservalidation.md`](./uservalidation.md)
- Control-plane state: [`state.json`](./state.json)
- Sister packet (primary HL-RESCAN-013 close-out): [`specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/`](../BUG-020-002-ml-auth-token-module-import-fail-loud/)
- Sister packet (template, parent-workflow precedent): [`specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/)
- Canonical fail-loud-read constant: [`ml/app/auth.py`](../../../../ml/app/auth.py) lines 22-29
- Affected file (pre-fix at HEAD `ad512fc6`): [`ml/app/nats_client.py`](../../../../ml/app/nats_client.py) line 184
- Companion test file: [`ml/tests/test_nats_client.py`](../../../../ml/tests/test_nats_client.py)
- BUG-020-002 adversarial regression suite (must remain green): [`ml/tests/test_auth_module_import_fail_loud.py`](../../../../ml/tests/test_auth_module_import_fail_loud.py)
- Pytest pre-seed conftest (must remain unchanged): [`ml/tests/conftest.py`](../../../../ml/tests/conftest.py)
- Binding policy (workspace-facing): [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md) → "SST Zero-Defaults Enforcement"
- Binding policy (agent-facing): [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md)
- Adversarial-test-integrity skill: [`.github/skills/bubbles-test-integrity/SKILL.md`](../../../../.github/skills/bubbles-test-integrity/SKILL.md)
