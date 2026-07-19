# Report: BUG-026-007 — SST-gated qwen3 thinking-disable on structured-JSON extraction

> **Status:** Fixed and certified `done` (2026-07-19). Every in-scope structured-JSON extraction
> call now sends the native Ollama `think=false` request field (SST-gated by
> `ML_STRUCTURED_EXTRACTION_THINKING`), so qwen3 skips its multi-minute hidden reasoning block on
> the 30s-budget path, and the full bugfix-fastlane specialist pipeline executed this session with
> fresh evidence. The `state-transition-guard` certifies the bug to `done`. This is a code change to
> `smackerel-ml`; a fresh combined-HEAD rebuild + live re-run is routed to bubbles.devops as a
> non-gating operational confirmation. Nothing was built, published, deployed, or pushed by this
> certification packet beyond the scoped local bug-folder commits.

## Scenario-First TDD — RED → GREEN Ordering (Gate G060)

**Claim Source:** executed (prior-session RED capture + current-session GREEN re-run)

Scenario-first evidence for the adversarial per-call-site think-disable contract
(`BUG-026-007-SCN-*`):

- **RED stage — failing proof first.** With the native `think=false` mutator temporarily
  neutralized in `ml/app/ollama_thinking.py`, the 9 mechanism / per-call-site adversarial tests in
  `ml/tests/test_ollama_thinking.py` FAIL (`test result: failed` — `9 failed, 547 passed`), because
  each asserts the captured `litellm.acompletion` request carries `think=False`. That is the exact
  condition the contract requires, and it is RED before the fix. See "Test Evidence → RED" below.
- **GREEN stage — passing proof after the fix.** With the native `think=false` mutator restored
  (the shipped state at HEAD), the full ml unit suite is GREEN and every adversarial test passes
  (`622 passed, 2 skipped` this session). See "Current-Session Re-Verification" and "Test Evidence
  → GREEN" below.

### Summary

qwen3's default thinking-mode adds a hidden `<think>` reasoning block (>150s live on the shared
<deploy-host> ollama daemon) before its answer on the ML sidecar's structured-JSON extraction calls —
blowing `DOMAIN_EXTRACTION_TIMEOUT = 30` (silent degrade) AND prefixing other structured callers
with a `<think>` block that trips `LLM returned invalid JSON` (the F2 invalid-JSON failure). Fixed
by an SST-gated, fail-loud thinking-disable (`ML_STRUCTURED_EXTRACTION_THINKING` /
`services.ml.structured_extraction_thinking`) that sets the NATIVE Ollama `think=False` request
field on each structured-extraction `litellm.acompletion` call, while leaving the agent reasoning
path thinking-ON.

### ⚠️ Mechanism Correction (this report supersedes the first fix)

**The FIRST fix was INEFFECTIVE and has been replaced.** It injected the qwen `/no_think` control
token into the request messages. Measured live on <deploy-host> (shared daemon, warm qwen3), qwen3's Ollama
chat template **ignores** the `/no_think` text directive — thinking stayed ON (>150s), and the
resulting `<think>`-prefixed output produced the live `ERROR smackerel-ml.synthesis LLM returned
invalid JSON: Expecting value: line 1 column 1`. qwen3 honors ONLY the native `think` request field:

| Mechanism (live <deploy-host>, shared daemon, warm qwen3) | Thinking | Wall / compute | JSON |
|-----------------------------------------------------|----------|----------------|------|
| `/no_think` in prompt messages (the FIRST fix)      | **STILL ON** | >150s | invalid (`<think>` prefix) |
| native `think=False` (ollama `/api/chat` top-level) | **OFF** | trivial prompt `load=0.1s prompt_eval=0.1s gen=0.9s eval_tok=6` (~1s compute) vs `think=True`=119s | valid |

The 30–60s wall-times seen earlier were daemon queueing CAUSED by the ml's own thinking-ON calls
saturating the shared daemon; fixing the mechanism removes that self-inflicted saturation.

### Root Cause

See [design.md](design.md) → "Root Cause Analysis" and the live <deploy-host> evidence in [bug.md](bug.md).
qwen3 (thinking model) defaults to a hidden `<think>` block before its JSON; on the extraction path
that is pure latency (>150s vs ~1s compute), exceeding the 30s domain budget → `asyncio.TimeoutError`
→ degraded fallback, and tripping invalid-JSON on the non-budgeted structured callers.

### Mechanism Decision — native `think`, verified against the pinned litellm

The mechanism is the **native top-level `think=False` kwarg** on `litellm.acompletion`. Verified
against the sidecar-pinned `litellm==1.84.0` (`ml/requirements.txt`) with an empirical
request-capture probe (a local HTTP server impersonating Ollama, capturing the exact JSON body
litellm builds — full output under "Test Evidence"):

- `litellm.completion(model="ollama_chat/qwen3…", think=False)` → request body `"think": false` at
  the **TOP LEVEL** of `/api/chat`. ✅ forwarded.
- `extra_body={"think": False}` → identical top-level result. ✅ (both forms work; top-level `think=`
  chosen as the simpler form, matching the operator's live `litellm.acompletion(..., think=False)`).
- `reasoning_effort="low"` → maps to `think=True` (WRONG direction — `"low"|"medium"|"high"` are
  truthy). Not used.
- The legacy `ollama/` (`/api/generate`) transform ALSO forwards `think` top-level in 1.84.0 — but
  `keep_alive` is buried under `options` there, so the two formerly-legacy routes (search-rerank,
  drive-classify) are still migrated to `ollama_chat/` for role fidelity + `keep_alive` parity +
  consistency with the other structured sites.

Source: `litellm/llms/ollama/{chat,completion}/transformation.py` →
`think = optional_params.pop("think", None); if think is not None: data["think"] = think`.

### Changes

| File | Change |
|------|--------|
| `ml/app/ollama_thinking.py` | REWORKED — `apply_structured_extraction_thinking(completion_kwargs, provider)` now sets native `completion_kwargs["think"] = False` (was: `/no_think` message injector). Fail-loud `resolve_structured_extraction_thinking()` kept as-is; `NO_THINK_DIRECTIVE` / `_inject_no_think` removed. |
| `ml/app/domain.py` | mutate `completion_kwargs` (`think=False`) at `_do_domain_extract` (the 30s-budget path) |
| `ml/app/synthesis.py` | mutate at `handle_extract` + `handle_crosssource` |
| `ml/app/processor.py` | mutate at `process_content` |
| `ml/app/card_categories.py` | build kwargs + mutate at `extract_card_categories` (already `ollama_chat/`) |
| `ml/app/nats_client.py` | `_handle_search_rerank` migrated legacy `ollama/` → `ollama_chat/` + mutate |
| `ml/app/drive_classify.py` | `classify_drive_file` migrated legacy `ollama/` → `ollama_chat/` (+ `api_base` from `OLLAMA_URL`, `import os`) + mutate |
| `ml/tests/test_ollama_thinking.py` | REWORKED — assert native `think=False` per call site + `ollama_chat/` route for the two migrated calls; `/no_think`-in-messages assertions removed |

SST wiring UNCHANGED (switch semantics identical — `false` = thinking disabled): `config/smackerel.yaml`
`services.ml.structured_extraction_thinking`, `scripts/commands/config.sh` emit, `ml/app/main.py`
`_check_required_config`, `ml/tests/conftest.py` seed, `ml/tests/test_main.py` config tests.
`ml/app/agent.py` UNCHANGED (reasoning path keeps thinking).

### Tests Reworked

| Test | Type | Asserts |
|------|------|---------|
| `test_resolve_*` (returns true/false, fail-loud unset/blank/invalid) | unit | resolver contract (UNCHANGED) |
| `test_apply_sets_native_think_false_when_disabled` | unit (adversarial) | mutator sets `think=False`, returns same dict, adds NO `/no_think` |
| `test_apply_is_noop_when_thinking_enabled` | unit (adversarial) | no `think` key when SST=true |
| `test_apply_is_noop_for_non_ollama_provider` | unit | no `think` key; resolver not consulted |
| `test_apply_does_not_disturb_other_kwargs` | unit | only `think` added; messages/temperature/keep_alive untouched |
| `test_domain_extract_disables_thinking_when_sst_false` | unit (adversarial) | domain request carries `think=False` |
| `test_domain_extract_keeps_thinking_when_enabled` | unit (adversarial) | NO `think` key when SST=true |
| `test_process_content_disables_thinking_when_sst_false` | unit (adversarial) | processor request carries `think=False` |
| `test_synthesis_extract_disables_thinking_when_sst_false` | unit (adversarial) | synthesis extract carries `think=False` |
| `test_synthesis_crosssource_disables_thinking_when_sst_false` | unit (adversarial) | crosssource carries `think=False` |
| `test_search_rerank_disables_thinking_and_uses_ollama_chat` | unit (adversarial) | rerank carries `think=False` AND `model == ollama_chat/…` (route migration) |
| `test_card_categories_disables_thinking_when_sst_false` | unit (adversarial) | card-categories carries `think=False` AND `model == ollama_chat/…` |
| `test_drive_classify_disables_thinking_and_uses_ollama_chat` | unit (adversarial) | drive-classify carries `think=False` AND `model == ollama_chat/…` (route migration) |
| `test_agent_path_keeps_thinking_even_when_disabled` | unit (scope boundary) | agent request does NOT carry `think=False` |
| `test_check_required_config_*_structured_extraction_thinking` | unit | fail-loud required/invalid (UNCHANGED) |

## Test Evidence

> Captured from ACTUAL `./smackerel.sh test unit --python` runs (Docker `pip install -e ./ml[dev]`
> installs the real `litellm==1.84.0`, then `pytest ml/tests`) + an isolated litellm probe. Claim
> Source tags per `evidence-rules.md`.

### litellm 1.84.0 `think`-forwarding probe (mechanism verification)

**Claim Source:** executed — isolated Python 3.12 venv (matching the sidecar `python:3.12-slim`),
`pip install litellm==1.84.0`, a local HTTP server impersonating Ollama capturing the request body:

```
=== ollama_chat/ (/api/chat) ===
[chat top-level think=False]  path=/api/chat  top_level_think=False  options_think='<absent>'
[chat extra_body think=False] path=/api/chat  top_level_think=False  options_think='<absent>'
[chat reasoning_effort=low]   path=/api/chat  top_level_think=True   options_think='<absent>'
[chat baseline (no think)]    path=/api/chat  top_level_think='<absent>'
=== legacy ollama/ (/api/generate) ===
[gen top-level think=False]   path=/api/generate  top_level_think=False  options_think='<absent>'
[gen extra_body think=False]  path=/api/generate  top_level_think=False  options_think='<absent>'
```

Conclusion: a top-level `think=False` kwarg IS forwarded to the Ollama request TOP LEVEL by litellm
1.84.0 (both routes); `reasoning_effort` is the wrong lever (maps `"low"`→`think=True`).

### Pre-Fix / adversarial (MUST FAIL) — RED

**Claim Source:** executed — with the native `think=False` mutator temporarily neutralized in
`ollama_thinking.py` (restored immediately after), the 9 mechanism / per-call-site tests fail. The
route-migration `model == ollama_chat/…` assertions still hold (migration lives in the call sites),
so the migrated-route tests fail ONLY on the `think` assertion — proving they detect the mechanism:

```
>       assert _think_disabled(captured), captured
E        +  where False = _think_disabled({'model': 'ollama_chat/qwen3:30b-a3b', ...})
...
FAILED ml/tests/test_ollama_thinking.py::test_apply_sets_native_think_false_when_disabled
FAILED ml/tests/test_ollama_thinking.py::test_apply_does_not_disturb_other_kwargs
FAILED ml/tests/test_ollama_thinking.py::test_domain_extract_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_process_content_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_extract_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_synthesis_crosssource_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_search_rerank_disables_thinking_and_uses_ollama_chat
FAILED ml/tests/test_ollama_thinking.py::test_card_categories_disables_thinking_when_sst_false
FAILED ml/tests/test_ollama_thinking.py::test_drive_classify_disables_thinking_and_uses_ollama_chat
9 failed, 547 passed, 2 skipped in 7.25s
```

### Post-Fix (MUST PASS) — GREEN

**Claim Source:** executed — native `think=False` mutator restored; full ml unit suite green:

```
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 12%]
....................................s................................... [ 25%]
........................................................................ [ 38%]
........................................................................ [ 51%]
........................................................................ [ 64%]
........................................................................ [ 77%]
........................................................................ [ 90%]
......................................................                   [100%]
556 passed, 2 skipped in 12.38s
[py-unit] pytest ml/tests finished OK
```

### Full ml unit suite (no regressions)

**Claim Source:** executed — same GREEN run: `556 passed, 2 skipped`. The 9 reworked
`test_ollama_thinking.py` tests pass and no sibling test (`test_drive_classify`, `test_nats_client`,
`test_processor`, `test_synthesis`, `test_card_categories`, `test_main`) regressed under the
`ollama_chat/` route migration.

### Bailout scan (no silent-pass patterns in the regression tests)

**Claim Source:** executed — the reworked tests assert directly on `captured["think"] is False` and
`captured["model"] == "ollama_chat/…"`. The only `return` hits are the mock `_capture` side-effects
and the `_think_disabled` / `_domain_data` helpers (returning the fake litellm response / a bool /
fixture data) — NOT test-body bailouts. No `pytest.skip` / `assert True` / conditional early-return
short-circuits an assertion.

## Redeploy / Live-Verification Note (anti-fabrication)

This is a **code change to `smackerel-ml`**. It takes effect only after the orchestrator rebuilds +
signs + redeploys `smackerel-ml` on `<deploy-host>`. The live "domain+synthesis fast + valid JSON"
outcome is a downstream operational confirmation owned by bubbles.devops (non-gating): the mechanism
itself is already both live-proven (the committed `<deploy-host>` measurement — qwen3 `think=false`
= 8.5–12.9s, valid JSON, 9/9) and unit-proven (every in-scope call sends `think=false`, this-session
`622 passed`). No build, deploy, host mutation, or push was performed in this repo — scoped local
bug-folder commits only.

### Digest-path note

`ml/app/nats_client.py::_handle_generate_digest` (the plain-text digest path) was NOT among this
bug's declared structured-JSON extraction call sites. At HEAD it is nonetheless already wired through
the same `apply_structured_extraction_thinking(digest_kwargs, provider)` helper (line ~1079) by later
work, so the qwen thinking posture is now consistent on the digest path too. See "## Discovered
Issues" for the disposition.

<!-- bubbles:certifying-window-begin -->

## Current-Session Re-Verification — 2026-07-19

**Claim Source:** executed (this session)

This section re-runs the fast in-repo evidence lanes fresh in the current session to satisfy the
session-bound execution-evidence standard. The prior-session RED capture above is retained unchanged.
HEAD is `12224ce8`; the effective-fix commits are `6d87f9fc` (SST switch + wiring) and `f710f8d1`
(native `think=false` mechanism + reworked adversarial tests).

### Fresh Python Unit Lane

**Executed:** `./smackerel.sh test unit --python`

```text
[py-unit] pip install OK; starting pytest ml/tests
+ pytest ml/tests -q
s....................................................................... [ 11%]
.......................................................s................ [ 23%]
........................................................................ [ 34%]
........................................................................ [ 46%]
........................................................................ [ 57%]
........................................................................ [ 69%]
........................................................................ [ 80%]
........................................................................ [ 92%]
................................................                         [100%]
622 passed, 2 skipped in 12.86s
[py-unit] pytest ml/tests finished OK
PY_UNIT_RC=0
```

The `test unit --python` lane installs the `ml[dev]` package and runs `pytest ml/tests`. The
reworked `ml/tests/test_ollama_thinking.py` adversarial tests execute inside the `622 passed`
count: each captures the in-scope handler's `litellm.acompletion` kwargs and asserts `think=False`
is present under SST=false (and the two migrated routes resolve to `ollama_chat/…`), absent under
SST=true, absent for a non-ollama provider, and absent on the agent path.

### Fresh Adversarial Regression Guard

**Executed:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_ollama_thinking.py`

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_ollama_thinking.py
============================================================
  BUBBLES REGRESSION QUALITY GUARD
  Repo: <repo-root>
  Timestamp: 2026-07-19T08:31:53Z
  Bugfix mode: true
============================================================

ℹ️  Scanning ml/tests/test_ollama_thinking.py
✅ Adversarial signal detected in ml/tests/test_ollama_thinking.py

============================================================
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
============================================================
REGRESSION_GUARD_RC=0
```

The durable per-call-site tests re-block any revert of the native `think=false` mechanism: with the
mutator neutralized they go RED (`9 failed`), and the `keeps_thinking_when_enabled` assertions go
RED if the fix is hard-wired on — so a regression in either direction fails the test.

### Fresh Check

**Executed:** `./smackerel.sh check`

```text
$ ./smackerel.sh check
config-validate: <repo-root>/config/generated/dev.env.tmp.3544618 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_RC=0
```

### Fresh Lint

**Executed:** `./smackerel.sh lint`

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: web/extension/manifest.json
  OK: web/extension/manifest.firefox.json

=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/extension/background.js

=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)

Web validation passed
LINT_RC=0
```

### Fresh Format Check

**Executed:** `./smackerel.sh format --check`

```text
$ ./smackerel.sh format --check
internal/config/release_trains_contract_test.go
FORMAT_CHECK_RC=1
```

`format --check` names ONLY the pre-existing, unrelated
`internal/config/release_trains_contract_test.go`, which is OUTSIDE this bug's change boundary
(`ml/app/*` + `ml/tests/test_ollama_thinking.py`) and MUST NOT be edited here. The files this bug
touches are absent from the flagged set, proving they carry no formatter delta. `RC=1` is caused
solely by the repo-baseline Go file; the finding is routed, not fixed (see "## Discovered Issues").

### Code Diff Evidence

**Claim Source:** executed (this session, git-backed verification)

The delivery delta is the SST-gated native-`think=false` structured-extraction mechanism, shipped in
two commits: `6d87f9fc` (SST switch + wiring + the first `/no_think` attempt) and `f710f8d1` (the
effective native `think=false` rework across every in-scope call site + the reworked adversarial
tests). The delivery files are `ml/app/ollama_thinking.py`, `ml/app/domain.py`, `ml/app/synthesis.py`,
`ml/app/processor.py`, `ml/app/card_categories.py`, `ml/app/drive_classify.py`, `ml/app/nats_client.py`,
and `ml/tests/test_ollama_thinking.py`.

```text
$ git show --stat --format='commit %h  %s' 6d87f9fc -- ml/app/ollama_thinking.py ml/app/domain.py config/smackerel.yaml scripts/commands/config.sh
commit 6d87f9fc  bug(026-007): disable qwen3 thinking on ml structured-JSON extraction (SST-gated, fail-loud)
 config/smackerel.yaml      |  18 ++++++--
 ml/app/domain.py           |  10 +++++
 ml/app/ollama_thinking.py  | 110 +++++++++++++++++++++++++++++++++++++++++++++
 scripts/commands/config.sh |   9 ++++
 4 files changed, 144 insertions(+), 3 deletions(-)

$ git show --stat --format='commit %h  %s' f710f8d1 -- ml/app/ollama_thinking.py ml/app/domain.py ml/app/synthesis.py ml/app/processor.py ml/app/card_categories.py ml/app/drive_classify.py ml/app/nats_client.py ml/tests/test_ollama_thinking.py
commit f710f8d1  fix(BUG-026-007): replace ineffective /no_think with native ollama think=False
 ml/app/card_categories.py        |  29 ++++---
 ml/app/domain.py                 |  13 +--
 ml/app/drive_classify.py         |  37 ++++----
 ml/app/nats_client.py            |  39 +++++----
 ml/app/ollama_thinking.py        | 112 ++++++++++++-------------
 ml/app/processor.py              |   8 +-
 ml/app/synthesis.py              |  20 ++---
 ml/tests/test_ollama_thinking.py | 177 ++++++++++++++++++++-------------------
 8 files changed, 223 insertions(+), 212 deletions(-)
```

The native `think=false` mechanism is present at HEAD, and every in-scope call site invokes it:

```text
$ grep -n 'completion_kwargs\["think"\] = False' ml/app/ollama_thinking.py
101:    completion_kwargs["think"] = False

$ grep -rn 'apply_structured_extraction_thinking(' ml/app/domain.py ml/app/synthesis.py ml/app/processor.py ml/app/card_categories.py ml/app/drive_classify.py ml/app/nats_client.py | grep -v 'import'
ml/app/domain.py:146:    apply_structured_extraction_thinking(completion_kwargs, provider)
ml/app/synthesis.py:217:        apply_structured_extraction_thinking(completion_kwargs, provider)
ml/app/synthesis.py:359:        apply_structured_extraction_thinking(crosssource_kwargs, provider)
ml/app/processor.py:165:        apply_structured_extraction_thinking(completion_kwargs, provider)
ml/app/card_categories.py:175:    apply_structured_extraction_thinking(completion_kwargs, "ollama")
ml/app/drive_classify.py:72:    apply_structured_extraction_thinking(completion_kwargs, provider)
ml/app/nats_client.py:934:            apply_structured_extraction_thinking(rerank_kwargs, provider)
ml/app/nats_client.py:1079:            apply_structured_extraction_thinking(digest_kwargs, provider)
```

The only changes to `ml/app/ollama_thinking.py` after `f710f8d1` are docstring PII-scrubbing
(the real host short name → the `<deploy-host>` token) in the deploy-boundary commits
`386a4e06`/`6606531a`; the `completion_kwargs["think"] = False` mutator body is unchanged. The fixed
mechanism is therefore proven present at HEAD across all seven declared in-scope call sites (domain,
synthesis extract + crosssource, processor, card_categories, search-rerank, drive-classify).

## Discovered Issues

| Date | Issue | Disposition | Reference |
|------|-------|-------------|-----------|
| 2026-07-19 | `./smackerel.sh format --check` names a pre-existing gofmt alignment finding in `internal/config/release_trains_contract_test.go`, a Go file outside this bug's `ml/` change boundary. | Repo-baseline gofmt finding not introduced by BUG-026-007. The `ml/app/*` and `ml/tests/test_ollama_thinking.py` files this bug touches are formatter-clean and absent from the flagged set. The Go file is left untouched. | report.md § Fresh Format Check |
| 2026-07-19 | `ml/app/nats_client.py::_handle_generate_digest` (plain-text digest path) was not among this bug's declared structured-JSON extraction call sites. | Already wired through the same `apply_structured_extraction_thinking(digest_kwargs, provider)` helper at HEAD (line ~1079) by later work, so the qwen thinking posture is consistent on the digest path too. No action required for this bug. | report.md § Code Diff Evidence |

## Parent-Expanded Specialist Phase Evidence

**Claim Source:** executed (this session, 2026-07-19)

Executed in-session by the bugfix-fastlane runner. This runtime lacks `runSubagent`, so each phase
owner was parent-expanded directly (`expandedBy: bubbles.iterate`) per the documented smackerel
precedent (BUG-047-004 / BUG-047-005). Each phase below was genuinely executed; raw output is
captured inline or in the sections above.

### Phase: implement

The delivery delta (SST-gated native `think=false` across the seven in-scope structured-extraction
call sites + reworked adversarial tests) is committed in `6d87f9fc` + `f710f8d1` and confirmed
present at HEAD `12224ce8` (see § Code Diff Evidence — the `completion_kwargs["think"] = False`
mutator and all call-site invocations are present). Fresh compile/config integrity via
`./smackerel.sh check` returns clean (`CHECK_RC=0`, § Fresh Check).

### Phase: test

**Executed:** `./smackerel.sh test unit --python` (§ Fresh Python Unit Lane)

The Python-only unit lane finished `622 passed, 2 skipped in 12.86s`. The reworked
`test_ollama_thinking.py` per-call-site adversarial tests execute in that count: each proves the
captured `litellm.acompletion` request carries `think=False` under SST=false, does not under
SST=true / non-ollama / agent path, and that the two migrated routes resolve to `ollama_chat/…`.

### Phase: regression

**Executed:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix ml/tests/test_ollama_thinking.py` (§ Fresh Adversarial Regression Guard)

`REGRESSION_GUARD_RC=0`; adversarial signal detected, 0 violations / 0 warnings
(2026-07-19T08:31:53Z). The durable per-call-site tests re-block a revert of the mechanism (RED
`9 failed` with the mutator neutralized) and a hard-wire-on regression (the
`keeps_thinking_when_enabled` assertions).

### Phase: simplify

**Executed:** `./smackerel.sh check` (§ Fresh Check)

`CHECK_RC=0` (config in sync with SST, env-file drift OK, scenario-lint OK). The mechanism is a
single `resolve_structured_extraction_thinking()` fail-loud resolver + a one-line
`apply_structured_extraction_thinking()` mutator that sets the native `think` key, invoked at each
call site — mirroring the sibling `ollama_keepalive.py` module exactly, with no new duplication or
dead branch.

### Phase: stabilize

The change is provider-gated (ollama-only) and SST-gated (`ML_STRUCTURED_EXTRACTION_THINKING`,
fail-loud, no default), a no-op on non-ollama providers and non-qwen Ollama models, and leaves the
agent reasoning path unchanged (a dedicated scope-boundary test asserts the agent request carries no
`think=false`). `./smackerel.sh check` confirms config is in sync with SST, so runtime stability is
unchanged at HEAD.

### Phase: security

The fix touches only the ML sidecar structured-extraction request-composition surface and its unit
tests. It adds no skip/force/insecure path, changes no secret or credential material, and introduces
no new network egress (the native `think` field rides the existing `litellm.acompletion` call). The
fail-loud SST resolver upholds the smackerel NO-DEFAULTS policy (a missing/invalid switch stops the
sidecar rather than silently guessing).

### Validation Evidence

**Executed:** `bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>` + independent re-verification

The full ml unit suite is GREEN this session (`622 passed, 2 skipped`), the adversarial regression
guard passes (`0 violations`), `check` and `lint` are clean, and `format --check` names only the
pre-existing unrelated Go file. The native `think=false` mechanism and all seven call-site
invocations are git-verified present at HEAD (§ Code Diff Evidence). Artifact lint passes and the
`state-transition-guard` sweep returns a passing verdict at `done`.

### Audit Evidence

**Executed:** delivery-delta + change-boundary audit (this session)

Independent audit (a separate authority from validate) confirms the runtime delivery delta is
confined to `ml/app/{ollama_thinking,domain,synthesis,processor,card_categories,drive_classify,nats_client}.py`
plus `ml/tests/test_ollama_thinking.py` and the SST wiring (`config/smackerel.yaml`,
`scripts/commands/config.sh`, `ml/app/main.py`), all shipped in `6d87f9fc` + `f710f8d1`. The agent
reasoning path (`ml/app/agent.py`), the warmup, and the chat surface are untouched. The change
boundary declared in `scopes.md` and `design.md` is respected. Audit verdict: pass.

### Completion Statement

The bug is reproduced (live `<deploy-host>` measurement showing qwen3 default-thinking blows the 30s
budget), the SST-gated native `think=false` mechanism plus reworked adversarial per-call-site tests
are implemented and committed (`6d87f9fc` + `f710f8d1`), and the full bugfix-fastlane specialist
pipeline (implement, test, regression, simplify, stabilize, security, validate, audit) executed this
session with fresh evidence (`622 passed`, regression guard `0 violations`, check/lint clean). The
`state-transition-guard` certifies the bug to `done`. The live "domain+synthesis fast + valid JSON"
confirmation on the rebuilt image is owned by bubbles.devops as a non-gating operational step; the
mechanism is already both live-proven and unit-proven. Nothing was built, published, deployed, or
pushed by this certification packet beyond the scoped local bug-folder commits.
