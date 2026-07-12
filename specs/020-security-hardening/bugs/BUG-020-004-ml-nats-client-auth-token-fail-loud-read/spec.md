# Bug: BUG-020-004 — `ml/app/nats_client.py` re-reads `SMACKEREL_AUTH_TOKEN` from `os.environ` at NATS connect time using the silent-default `os.environ.get(KEY, "")` form, bypassing the canonical fail-loud read in `ml/app/auth.py` and violating Gate G028 (NO-DEFAULTS / fail-loud SST policy)

## Classification

- **Type:** Security defect — secondary fail-loud SST contract gap on the ML sidecar auth token (operational hardening; carry-over from primary HL-RESCAN-013 close-out)
- **Severity:** P3 — LOW (the runtime contract for `SMACKEREL_AUTH_TOKEN` is already enforced once the lifespan startup check `_check_required_config()` runs in `ml/app/main.py` and once `app.auth` is imported anywhere in the process — both happen before `NATSClient.connect()` is ever invoked, so the silent default at the connect-time call site is functionally redundant. The defect is twofold: (a) the silent-default form `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` is FORBIDDEN by Gate G028 NO-DEFAULTS / fail-loud SST policy at every read site — defense-in-depth elsewhere is not a substitute; (b) re-reading `os.environ` at connect time creates a parallel auth-token data path that diverges from the canonical fail-loud-read constant `_AUTH_TOKEN` in `ml/app/auth.py`, so any future change to the canonical read pattern would silently fail to propagate to the NATS connect path.)
- **Parent Spec:** 020 — Security Hardening (owns the `SMACKEREL_AUTH_TOKEN` contract; see `specs/020-security-hardening/spec.md` and the Go-core equivalent fail-loud read at `cmd/core/wiring.go` lines 37/57/59 already correct since HL-RESCAN-008 / commit `7482fb24`)
- **Sister Packet:** [`BUG-020-002`](../BUG-020-002-ml-auth-token-module-import-fail-loud/) — the **primary** HL-RESCAN-013 close-out, closed at HEAD `0c67122e` (`bug(020-002): ML auth token fail-loud at module import`). That commit converted `ml/app/auth.py` line 11 from `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` to a try/except KeyError → RuntimeError block keyed on `os.environ["SMACKEREL_AUTH_TOKEN"]`, and explicitly listed `ml/app/nats_client.py:180` as **out-of-scope / sequel packet** (see [`BUG-020-002/spec.md`](../BUG-020-002-ml-auth-token-module-import-fail-loud/spec.md) → "Out of Scope" → second bullet: *"`ml/app/nats_client.py` line 180 — runs INSIDE `connect()` which is invoked AFTER `_check_required_config()` validates the env, so the silent default there is functionally redundant. ... Scoped to a sequel packet to be filed once the parallel session merges."*). This bug packet (`BUG-020-004`) is that sequel packet.
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed and validated (current-session evidence recorded in `report.md`)
- **Discovered By:** 2026-05-15 self-hosted readiness re-scan (finding `HL-RESCAN-013-secondary`)

## Discovery Brief

The 2026-05-14 self-hosted readiness re-scan flagged `ml/app/auth.py:11` as `HL-RESCAN-013` (silent-default read of `SMACKEREL_AUTH_TOKEN` at module import, violating Gate G028). That finding was closed by [`BUG-020-002`](../BUG-020-002-ml-auth-token-module-import-fail-loud/) at HEAD `0c67122e` on 2026-05-14, which converted line 11 of `auth.py` to the canonical Python fail-loud pattern (`_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in try/except KeyError → RuntimeError).

The 2026-05-15 self-hosted readiness re-scan (run after the BUG-020-002 close-out) re-audited the ML sidecar surface for any remaining `os.environ.get("SMACKEREL_AUTH_TOKEN", ...)` call sites and found one secondary site that the primary close-out's "Out of Scope" section had explicitly deferred: `ml/app/nats_client.py:184` (line number against HEAD `ad512fc6`). This bug packet (`BUG-020-004`) is filed against that secondary site as the sister packet to BUG-020-002, following the same `parentWorkflow.mode = self-hosted-readiness-rescan-external-2026-05-15` direct-per-finding-dispatch model established by [`BUG-042-006`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/) at HEAD `eec1437c`.

## Problem Statement

The Smackerel runtime contract per `.github/copilot-instructions.md` ("SST Zero-Defaults Enforcement") is:

| Language | FORBIDDEN | REQUIRED |
|----------|-----------|----------|
| **Python** | `os.getenv("KEY", "default")` | `os.environ["KEY"]` (raises KeyError) |

The pre-fix `ml/app/nats_client.py` lines 183-186 (at HEAD `ad512fc6`) were:

```python
        # Token authentication — mirrors Go core's NATS auth enforcement
        auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
        if auth_token:
            connect_opts["token"] = auth_token
```

This call site:

1. **Re-reads `os.environ` at connect time** instead of consuming the canonical fail-loud-read module-level constant `_AUTH_TOKEN` already defined in `ml/app/auth.py` (which is guaranteed-defined by the time `NATSClient.connect()` runs, because `app.main` imports `app.auth` at process startup before any FastAPI lifespan handler runs).
2. **Uses the FORBIDDEN silent-default form** `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` — Gate G028 NO-DEFAULTS / fail-loud SST policy bans this form at every Python read site of an SST-managed value. Defense-in-depth at other layers (`ml/app/main.py:_check_required_config` lifespan check; `ml/app/auth.py:11` module-import fail-loud read closed by BUG-020-002) is **not a substitute** for boundary-level fail-loud at every read site.
3. **Creates a parallel auth-token data path** that diverges from the canonical read in `auth.py`. Any future change to the canonical read pattern (e.g., format check, length check, base64 decode) would silently fail to propagate to the NATS connect path because the NATS connect path bypasses `_AUTH_TOKEN` entirely.

The fix routes the auth-token plumbing through the canonical `_AUTH_TOKEN` constant: a new top-of-file import `from .auth import _AUTH_TOKEN` re-exports the constant under `app.nats_client._AUTH_TOKEN`, and the connect-time block becomes:

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

This closes the Gate G028 violation at the connect-time call site, eliminates the parallel `os.environ` read, and preserves the existing dev-mode auth-bypass semantics (empty `_AUTH_TOKEN` → no `token` kwarg passed → NATS server accepts the connection without auth, matching the dev-mode behaviour of `verify_auth()` in `auth.py`).

The companion test file `ml/tests/test_nats_client.py` is updated correspondingly: `TestConnect.test_connect_passes_auth_token` and `TestConnect.test_connect_no_token_when_env_empty` switch from `patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "..."})` to `patch("app.nats_client._AUTH_TOKEN", "...")`, mirroring the new code path while preserving both behavioural assertions (token kwarg present iff non-empty).

## Detection

| Aspect | Detail |
|---|---|
| Trigger | 2026-05-15 self-hosted readiness re-scan (run after BUG-020-002 close-out) |
| Finding | `HL-RESCAN-013-secondary` (lens: SST defaults / Gate G028; surface: `ml/app/nats_client.py`) |
| Severity | P3 — LOW (defense-in-depth at `ml/app/auth.py:11` post-BUG-020-002 close-out + lifespan startup check at `ml/app/main.py:_check_required_config` already convert UNSET + production to non-zero exit before `NATSClient.connect()` is ever invoked; the runtime risk is bounded) |
| Audit method | `grep -nE 'os\.environ\.get\("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py` against HEAD `ad512fc6` returned exactly one match: `ml/app/nats_client.py:184: auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")`. Confirmed the line lives inside `NATSClient.connect()` (not at module-import time). Confirmed the pre-fix block at HEAD lines 183-186 is the verbatim 4-line section quoted in the Problem Statement above (captured via `git show HEAD:ml/app/nats_client.py | sed -n '175,195p'`). Confirmed the canonical fail-loud-read constant `_AUTH_TOKEN` is defined at `ml/app/auth.py` lines 22-29 (post-BUG-020-002, HEAD `0c67122e`). |

## Verified Call-Site Evidence (Pre-Fix, HEAD `ad512fc6`)

The exact pre-fix block at `ml/app/nats_client.py` lines 183-186 (captured via `git show HEAD:ml/app/nats_client.py | sed -n '183,186p'`):

```python
        # Token authentication — mirrors Go core's NATS auth enforcement
        auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
        if auth_token:
            connect_opts["token"] = auth_token
```

Single-line forbidden-form match (line 184):

```text
$ git show HEAD:ml/app/nats_client.py | grep -n 'SMACKEREL_AUTH_TOKEN'
184:        auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
```

(Exit Code: 0; one match, confirming the violation is the single line 184.)

The canonical fail-loud-read constant already exists in `ml/app/auth.py` lines 22-29 (post-BUG-020-002, HEAD `0c67122e`):

```python
try:
    _AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]
except KeyError as exc:
    raise RuntimeError(
        "ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set in the env file "
        "(run ./smackerel.sh config generate); empty value is allowed for "
        "dev-mode auth bypass when SMACKEREL_ENV=development|test "
        "(HL-RESCAN-013 / Gate G028 fail-loud SST contract)"
    ) from exc
```

## Reproduction

```bash
# (1) Confirm the silent-default read at the secondary site exists at HEAD
grep -n 'os.environ.get("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py
# Expected: 1 match at line 184 (against HEAD ad512fc6)

# (2) Confirm the canonical fail-loud read in auth.py is already correct
grep -n 'os.environ\["SMACKEREL_AUTH_TOKEN"\]' ml/app/auth.py
# Expected: 1 match at line 22 (post-BUG-020-002, HEAD 0c67122e)

# (3) Confirm the FORBIDDEN form lives inside NATSClient.connect() (not at module import)
grep -n -B 2 -A 2 'os.environ.get("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py
# Expected: lines 183-186 are the 4-line block quoted above; the surrounding
# context shows the block is inside the async connect() method, AFTER
# _check_required_config() has already validated the env (and AFTER auth.py
# was imported at module-load time, which would have raised RuntimeError if
# the env var were UNSET).

# (4) Confirm the binding policy explicitly FORBIDS the form
grep -nE 'os\.getenv|os\.environ\.get|FORBIDDEN' .github/copilot-instructions.md | head -20
# Expected: Secrets Management table entries naming the FORBIDDEN form
# os.getenv("KEY", "default") and the REQUIRED form os.environ["KEY"]
```

Expected: silent-default read found at `ml/app/nats_client.py:184`; canonical fail-loud read present at `ml/app/auth.py:22`; binding policy in `.github/copilot-instructions.md` explicitly forbids the silent-default form.

## Root Cause

The original `ml/app/nats_client.py` `connect()` method was authored before Gate G028 was codified as binding agent policy, and before the canonical `_AUTH_TOKEN` constant existed in `ml/app/auth.py`. At the time the connect path was written, re-reading `os.environ` directly was the established pattern across the codebase. The author's comment `# Token authentication — mirrors Go core's NATS auth enforcement` correctly mirrored the Go core's pattern at the time but inherited the same silent-default style.

The Go core's equivalent NATS auth path was hardened in HL-RESCAN-008 / commit `7482fb24` (Go-side fail-loud read of `SMACKEREL_AUTH_TOKEN` via `os.Getenv` + empty-and-production → `log.Fatal` pattern). The Python ML sidecar's `auth.py` module-level read was hardened in HL-RESCAN-013 / commit `0c67122e` ([`BUG-020-002`](../BUG-020-002-ml-auth-token-module-import-fail-loud/)). Both of those close-outs explicitly identified `ml/app/nats_client.py:180` (now line 184 at HEAD `ad512fc6`) as a known sequel-packet site:

- BUG-020-002 spec.md "Out of Scope" → second bullet — explicitly defers `ml/app/nats_client.py:180` to a sequel packet.
- BUG-020-002 spec.md Detection table → "Audit method" cell — names all three call sites (`auth.py:11`, `main.py:146`, `nats_client.py:180`) and explains why each is in or out of scope.

This bug packet (`BUG-020-004`) is the deferred sequel.

## Expected Behavior

After the fix, a reader inspecting `ml/app/nats_client.py` should find:

1. NO `os.environ.get("SMACKEREL_AUTH_TOKEN", ...)` call site anywhere in the file (verified by `grep`).
2. NO `os.environ["SMACKEREL_AUTH_TOKEN"]` direct read either — auth-token plumbing is routed through `_AUTH_TOKEN` re-imported from `app.auth`.
3. A top-of-file import block `from .auth import _AUTH_TOKEN` (with an explanatory comment naming `HL-RESCAN-013`, `Gate G028`, and the rationale for re-using the canonical constant rather than re-reading `os.environ`).
4. The connect-time auth block reduced to a 2-line conditional `if _AUTH_TOKEN: connect_opts["token"] = _AUTH_TOKEN` with an explanatory comment describing the dev-mode auth-bypass semantics for the empty-string case.

A reader inspecting `ml/tests/test_nats_client.py` should find:

5. `TestConnect.test_connect_passes_auth_token` patches `app.nats_client._AUTH_TOKEN` directly via `patch("app.nats_client._AUTH_TOKEN", "secret-token")` instead of mutating `os.environ`. The behavioural assertion (`call_kwargs["token"] == "secret-token"`) is preserved.
6. `TestConnect.test_connect_no_token_when_env_empty` patches `app.nats_client._AUTH_TOKEN` to `""` instead of mutating `os.environ`. The behavioural assertion (`"token" not in call_kwargs`) is preserved.

## Actual Behavior (Pre-Fix at HEAD `ad512fc6`)

A reader inspecting `ml/app/nats_client.py` at HEAD `ad512fc6` finds:

1. `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` at line 184 — the FORBIDDEN silent-default form.
2. The canonical `_AUTH_TOKEN` constant from `app.auth` is NOT imported at the top of `nats_client.py` and NOT consumed at the connect-time call site.
3. The connect-time auth block is a 4-line construct with a local `auth_token` variable (lines 183-186).

A reader inspecting `ml/tests/test_nats_client.py` at HEAD `ad512fc6` finds:

4. `TestConnect.test_connect_passes_auth_token` and `TestConnect.test_connect_no_token_when_env_empty` patch `os.environ` directly, mirroring the (FORBIDDEN) production code path.

## Acceptance Criteria

- **AC-1**: `os.environ.get("SMACKEREL_AUTH_TOKEN", ...)` (and any equivalent silent-default form such as `os.getenv("SMACKEREL_AUTH_TOKEN", ...)`) is **removed** from `ml/app/nats_client.py`. Verified by `grep -nE 'os\.(environ\.get|getenv)\("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py` returning **zero matches**.
- **AC-2**: Auth-token plumbing in `ml/app/nats_client.py` is routed through the canonical fail-loud-read constant `_AUTH_TOKEN` from `app.auth`. Verified by:
  - `grep -nE '^from \.auth import _AUTH_TOKEN' ml/app/nats_client.py` returns exactly 1 match in the top-of-file import block.
  - `grep -nE 'if _AUTH_TOKEN:' ml/app/nats_client.py` returns exactly 1 match inside `NATSClient.connect()`.
  - `grep -nE 'connect_opts\["token"\] = _AUTH_TOKEN' ml/app/nats_client.py` returns exactly 1 match immediately following the `if _AUTH_TOKEN:` line.
- **AC-3**: The companion tests `TestConnect.test_connect_passes_auth_token` and `TestConnect.test_connect_no_token_when_env_empty` in `ml/tests/test_nats_client.py` patch `app.nats_client._AUTH_TOKEN` directly via `patch("app.nats_client._AUTH_TOKEN", "<value>")` instead of mutating `os.environ`. Verified by `grep -nE 'patch\("app\.nats_client\._AUTH_TOKEN"' ml/tests/test_nats_client.py` returning at least 2 matches (one per test).
- **AC-4**: All ML unit tests pass green. Verified by `./smackerel.sh test unit --python` exits 0 with all `ml/tests/test_nats_client.py` tests PASS (specifically `TestConnect::test_connect_passes_auth_token PASSED` and `TestConnect::test_connect_no_token_when_env_empty PASSED`); the BUG-020-002 adversarial suite (`ml/tests/test_auth_module_import_fail_loud.py`) continues to PASS unchanged; no other ML test regresses.
- **AC-5**: The Gate G028 NO-DEFAULTS / fail-loud SST audit grep against `ml/app/nats_client.py` is clean. Verified by `grep -nE 'os\.(environ\.get|getenv)\("SMACKEREL_AUTH_TOKEN"' ml/app/nats_client.py` returning zero matches AND `grep -nE 'os\.environ\.get|os\.getenv' ml/app/nats_client.py` returning only matches for non-SST-managed env reads (e.g., `LLM_PROVIDER`, `LLM_MODEL`, `OLLAMA_URL` — out of scope for this packet).

## Out of Scope

- **`ml/app/auth.py`** — already correct on HEAD post-BUG-020-002 (lines 22-29 implement the canonical Python fail-loud try/except KeyError → RuntimeError pattern). This packet does NOT modify `auth.py`; it only re-imports the existing `_AUTH_TOKEN` constant.
- **`ml/app/main.py:146`** — the existing `_check_required_config()` lifespan check still uses `os.environ.get("SMACKEREL_AUTH_TOKEN", "")` and converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)`. The runtime risk of the silent default there is bounded by the production branch; converting it to the fail-loud form is a separate audit-cleanliness improvement scoped to a future sequel packet (would be `BUG-020-005` if filed). Out of scope for this packet to keep the change set minimum-touch and to avoid colliding with any parallel session.
- **`ml/app/embedder.py`** — the parallel-session working tree currently has line-length-only reformatting changes in this file. Those are autoformatter noise unrelated to `HL-RESCAN-013-secondary`. This packet does NOT touch `embedder.py`.
- **`ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`** — the parallel-session working tree currently has line-length-only reformatting changes in these test files. Those are autoformatter noise unrelated to this finding. This packet does NOT touch any of these files.
- **`internal/metrics/auth.go`** — the parallel-session working tree currently has godoc indentation-only changes in this file. Out of scope (Go-side; this packet is scoped to the Python ML sidecar boundary).
- **`tests/integration/auth_chaos_test.go`** — the parallel-session working tree currently has gofmt comment-alignment + trailing-newline changes in this file. Out of scope (Go-side; autoformatter noise).
- **`cmd/core/wiring.go`** — the Go-side equivalent fail-loud read of `SMACKEREL_AUTH_TOKEN` is already correct (lines 37/57/59 — `os.Getenv` + empty-and-production → `log.Fatal` pattern). Outside the Python ML sidecar boundary.
- **`scripts/commands/config.sh`** — the SST emission of `SMACKEREL_AUTH_TOKEN` is already correct (lines 483/495/499/502/504/1031 emit the line unconditionally; empty for dev, auto-generated 48-hex for test). Verified by BUG-020-002.
- **Editing `specs/020-security-hardening/spec.md`, `specs/020-security-hardening/design.md`, `specs/020-security-hardening/scopes.md`, `specs/020-security-hardening/state.json`, `specs/020-security-hardening/uservalidation.md`, `specs/020-security-hardening/report.md`** — foreign-owned parent-spec content; outside this bug packet's edit scope.
- **Editing `.github/copilot-instructions.md` or `.github/instructions/smackerel-no-defaults.instructions.md`** — both already correctly document the Python fail-loud pattern. No change needed.
- **Committing the fix** — out of scope for this session; this packet records implementation, test, validation, and audit evidence without staging or committing workspace changes.

## Severity Justification

**P3 — LOW, NOT P2 — MEDIUM**:

- The runtime contract for `SMACKEREL_AUTH_TOKEN` is already enforced at two earlier layers:
  1. `ml/app/auth.py` lines 22-29 — module-import-time fail-loud read (BUG-020-002, HEAD `0c67122e`). Any Python interpreter that imports `app.auth` (which `app.main` does at startup, before the FastAPI lifespan handler runs) will crash loudly with `RuntimeError: ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set...` if the env var is UNSET.
  2. `ml/app/main.py:_check_required_config` lifespan check — converts empty + `SMACKEREL_ENV=production` to `sys.exit(1)` at lifespan startup.
- By the time `NATSClient.connect()` is invoked (FastAPI lifespan startup, AFTER both checks above), the env var is guaranteed to be set (or the process has already crashed).
- The Gate G028 violation at the connect-time call site is therefore **functionally redundant** at runtime — the silent default would only ever evaluate the env var as empty if the earlier fail-loud checks were somehow bypassed (e.g., a future refactor that reorders lifespan handlers, or a test harness that imports `app.nats_client` without first importing `app.auth`).
- BUT: defense-in-depth elsewhere is **not a substitute** for boundary-level fail-loud at every read site. Gate G028 NO-DEFAULTS / fail-loud SST policy bans the form `os.environ.get("KEY", "default")` at every Python read site of an SST-managed value. A future code-search audit (e.g., HL-RESCAN-014) would re-flag this site even though the runtime risk is bounded.
- The defect also creates a **parallel auth-token data path** that bypasses `_AUTH_TOKEN`. Any future change to the canonical read pattern (e.g., format check, length check, base64 decode) would silently fail to propagate to the NATS connect path.

**Not P4 — TRIVIAL** because the call site is in production code and the defect violates a binding agent policy (Gate G028); a P4 designation would imply "cosmetic only" which understates the policy violation.

## Related

- **Sister packet (primary):** [`specs/020-security-hardening/bugs/BUG-020-002-ml-auth-token-module-import-fail-loud/`](../BUG-020-002-ml-auth-token-module-import-fail-loud/) — primary HL-RESCAN-013 close-out at HEAD `0c67122e`. This packet (`BUG-020-004`) is the deferred sequel for the connect-time call site explicitly named in BUG-020-002's "Out of Scope" → second bullet.
- **Sister packet (template):** [`specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/) — established the `parentWorkflow.mode = self-hosted-readiness-rescan-external-2026-05-15` direct-per-finding-dispatch packet pattern at HEAD `eec1437c`.
- **Parent Spec:** `specs/020-security-hardening/`
- **Binding policy (workspace-facing):** [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md) — "SST Zero-Defaults Enforcement (NON-NEGOTIABLE)" subsection inside Required Runtime Standards (Secrets Management table).
- **Binding policy (agent-facing):** [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md) — Gate G028 NO-DEFAULTS / fail-loud SST policy.
- **Gate authority:** Gate G028 (NO-DEFAULTS / fail-loud SST policy)
- **Go-side equivalent:** [`cmd/core/wiring.go`](../../../../cmd/core/wiring.go) lines 37/57/59 — fail-loud read of `SMACKEREL_AUTH_TOKEN` at the Go core boundary (already correct since HL-RESCAN-008 / commit `7482fb24`).
- **Canonical fail-loud-read constant (sister packet output):** [`ml/app/auth.py`](../../../../ml/app/auth.py) lines 22-29 — `_AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]` wrapped in try/except KeyError → RuntimeError (post-BUG-020-002, HEAD `0c67122e`).
- **Affected file (pre-fix at HEAD `ad512fc6`):** [`ml/app/nats_client.py`](../../../../ml/app/nats_client.py) line 184 — `auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")`.
- **Companion test file:** [`ml/tests/test_nats_client.py`](../../../../ml/tests/test_nats_client.py) — `TestConnect.test_connect_passes_auth_token` and `TestConnect.test_connect_no_token_when_env_empty`.
