#!/usr/bin/env bash
# =============================================================================
# evidence-admission-hardening-selftest.sh  (IMP-102 / SCOPE-1)
# =============================================================================
# Adversarial, hermetic selftest for the Check 9 "evidence admission" hardening
# fixes applied to state-transition-guard.sh + tool-call.schema.json.
#
# Each fixture is a REAL feature directory built the same way the transition
# guard's own selftest builds them (docs-only base -> autonomous-goal delivery
# contract), so every fixture RESOLVES a workflow mode and REACHES Check 9. A
# fixture that failed to resolve would print `E009-...` / `workflowMode:
# UNRESOLVED` and BLOCK for the wrong reason (tautology); every case therefore
# asserts the guard output does NOT contain E009/UNRESOLVED before asserting on
# Check-9 behavior.
#
# The seven hardening fixes exercised (guard Check 9, L2198-2505):
#   #1 truly-bare `-> Evidence: done` marker (no report.md ref / inline / link) -> BLOCKS
#   #2 plain `[report.md](...)` link where the linked report has <10 non-blank lines -> BLOCKS
#   #3 (ADVISORY, non-blocking) resolved >=10-line block with no command-output signature -> ACCEPTED + advisory
#   #4 tool-log line that is schema-invalid under additionalProperties:false -> NON-matching -> BLOCKS
#   #5 uppercase `- [X]` DoD item is scanned identically to `- [x]` -> BLOCKS when unevidenced
#   #6 tool-log entry with spec:"" names nothing -> NON-matching -> BLOCKS
#   #7 duplicated identical `- [x]` lines resolve to their OWN occurrence (Nth), not head-1 -> BLOCKS
#
# Non-tautology proof: fixtures #1 and #5 are ALSO run against the OLD guard
# (git 86fc700, before the fixes) and MUST PASS there — proving the new fixes,
# not some incidental structural failure, are what make them block now.
#
# Graceful degradation: SKIP+exit0 when python3 is absent; the #4 jsonschema
# case is skipped when `python3 -c 'import jsonschema'` fails (offline flows
# fall back to token matching, so the schema-authentication fix is inert then).
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OLD_GUARD_REF="86fc700"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

# Keep the guard fast: the delegated tail gates (G085-G095) have their own
# selftests in framework-validate; skipping them here isolates Check 9 signal.
export BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=1

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
tmp_root="$(mktemp -d "$selftest_tmp_base/bubbles-evidence-admission-selftest.XXXXXX")"
passed=0
failed=0

cleanup() {
  if [[ "$failed" -eq 0 ]] && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$tmp_root"
  else
    echo "Preserving selftest workspace: $tmp_root"
  fi
}
trap cleanup EXIT

# Graceful degradation — evaluated AFTER the EXIT trap is registered so a skip
# still tears down tmp_root. SKIP+exit0 when python3 is absent; the #4 schema
# case is gated separately on `import jsonschema`.
if ! command -v python3 >/dev/null 2>&1; then
  echo "evidence-admission-hardening-selftest: SKIP (python3 not installed)"
  exit 0
fi

HAVE_JSONSCHEMA=0
if python3 -c 'import jsonschema' >/dev/null 2>&1; then
  HAVE_JSONSCHEMA=1
fi

pass() { echo "PASS: $1"; passed=$((passed + 1)); }
fail() {
  echo "FAIL: $1"
  failed=$((failed + 1))
  if [[ -n "${2:-}" ]] && [[ -f "${2:-}" ]]; then
    echo "--- guard log excerpt: $2 ---"
    sed -n '1,220p' "$2"
    echo "--- end guard log excerpt ---"
  fi
}

run_capture() {
  local log_file="$1"
  shift
  set +e
  "$@" >"$log_file" 2>&1
  local status=$?
  set -e
  echo "$status"
}

log_has() { grep -Fq -- "$2" "$1"; }

# A resolved contract never prints an E009 blocking code or an UNRESOLVED mode.
assert_resolved() {
  local log_file="$1" label="$2"
  if log_has "$log_file" "E009-" || log_has "$log_file" "workflowMode: UNRESOLVED"; then
    fail "$label — fixture did NOT resolve a contract (E009/UNRESOLVED); Check-9 assertion would be tautological" "$log_file"
    return 1
  fi
  return 0
}

# BLOCKING: guard exits non-zero, contract resolved, and the failure names the
# expected Check-9 reason.
assert_blocks_with() {
  local feature_dir="$1" needle="$2" label="$3"
  local log_file
  log_file="$tmp_root/$(basename "$feature_dir").log"
  local status
  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  assert_resolved "$log_file" "$label" || return 0
  if [[ "$status" -eq 0 ]]; then
    fail "$label — guard PASSED but must BLOCK" "$log_file"
    return 0
  fi
  if log_has "$log_file" "$needle"; then
    pass "$label (blocks; names Check-9 reason)"
  else
    fail "$label — blocked but WITHOUT the expected Check-9 reason: '$needle'" "$log_file"
  fi
}

# PASS: guard exits 0. `advisory_expect` = yes|no|any controls the advisory line.
assert_passes() {
  local feature_dir="$1" advisory_expect="$2" label="$3"
  local log_file
  log_file="$tmp_root/$(basename "$feature_dir").log"
  local status
  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  assert_resolved "$log_file" "$label" || return 0
  if [[ "$status" -ne 0 ]]; then
    fail "$label — guard BLOCKED but must PASS" "$log_file"
    return 0
  fi
  case "$advisory_expect" in
    yes)
      if log_has "$log_file" "Check-9 ADVISORY:"; then
        pass "$label (passes; emits Check-9 ADVISORY)"
      else
        fail "$label — passed but WITHOUT the expected 'Check-9 ADVISORY:' line" "$log_file"
      fi
      ;;
    no)
      if log_has "$log_file" "Check-9 ADVISORY:"; then
        fail "$label — passed but emitted an UNEXPECTED 'Check-9 ADVISORY:' line" "$log_file"
      else
        pass "$label (passes; no advisory)"
      fi
      ;;
    *) pass "$label (passes)" ;;
  esac
}

# CHECK-9-level accept: the tool-log evidence path COVERS a bare `- [x]` item
# (Check 9 case 4). Asserted at the Check-9 verdict rather than the full-guard
# exit code, because artifact-lint (Check 13) is deliberately NOT tool-log-aware
# and independently blocks any bare item — a pre-existing property of a DIFFERENT
# check, out of scope for this Check 9 evidence-admission hardening.
assert_check9_covers() {
  local feature_dir="$1" label="$2"
  local log_file
  log_file="$tmp_root/$(basename "$feature_dir").log"
  run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir" >/dev/null
  assert_resolved "$log_file" "$label" || return 0
  if log_has "$log_file" "has NO evidence block"; then
    fail "$label — Check 9 emitted 'has NO evidence block' (the valid tool-log entry was NOT accepted)" "$log_file"
    return 0
  fi
  if log_has "$log_file" "checked DoD items across resolved scope files have evidence blocks"; then
    pass "$label (Check 9 tool-log path COVERS the bare item)"
  else
    fail "$label — Check 9 all-evidenced verdict line absent" "$log_file"
  fi
}

# NON-TAUTOLOGY: the SAME fixture must PASS on the OLD guard (pre-fix), proving
# the new fix — not an incidental structural failure — is what blocks it now.
assert_old_guard_passes() {
  local feature_dir="$1" label="$2"
  local log_file
  log_file="$tmp_root/$(basename "$feature_dir").oldguard.log"
  local status
  status="$(run_capture "$log_file" bash "$OLD_GUARD" "$feature_dir")"
  if log_has "$log_file" "E009-" || log_has "$log_file" "workflowMode: UNRESOLVED"; then
    fail "$label — OLD guard did not resolve the contract (E009/UNRESOLVED); teeth-proof invalid" "$log_file"
    return 0
  fi
  if [[ "$status" -eq 0 ]]; then
    pass "$label (old guard PASSES -> new fix has teeth)"
  else
    fail "$label — OLD guard BLOCKED; fixture is not isolated to the new fix" "$log_file"
  fi
}

# -----------------------------------------------------------------------------
# Fixture builders — mirror the transition-guard selftest's proven pass fixture.
# -----------------------------------------------------------------------------
clone_framework_surface() {
  local destination_root="$1"
  mkdir -p "$destination_root"
  cp -R "$SCRIPT_DIR/.." "$destination_root/bubbles"
  cp -R "$SCRIPT_DIR/../../agents" "$destination_root/agents"
}

# emit_pass_fixture <feature_dir>
# Builds a coherent single-file feature that PASSES the guard on a supported
# autonomous-goal delivery contract (docs-only base + delivery mutation). The
# three baseline DoD items reference report.md via BARE refs (the long-standing
# accepted convention), so Check 9 is satisfied by the base; each adversarial
# case then adds ONE probe item OUTSIDE the Definition of Done section so the
# guard's DoD-section checks (G041/G068) never see it and ONLY Check 9's
# whole-file `- [x]` scan evaluates it.
emit_pass_fixture() {
  local feature_dir="$1"
  local scenario_test="$feature_dir/tests/docs-scenario-regression.e2e.spec.ts"
  local broader_test="$feature_dir/tests/docs-broader-regression.e2e.spec.ts"

  mkdir -p "$feature_dir/tests"
  printf 'export const docsScenarioRegression = true;\n' >"$scenario_test"
  printf 'export const docsBroaderRegression = true;\n' >"$broader_test"

  cat <<'EOF' >"$feature_dir/spec.md"
# Evidence Admission Fixture Spec

## Purpose

Exercise the Check 9 evidence-admission hardening on a minimal but coherent
delivery artifact set.
EOF

  cat <<'EOF' >"$feature_dir/design.md"
# Evidence Admission Fixture Design

## Approach

Use a supported delivery workflow mode so the transition guard evaluates state
integrity, artifact integrity, and Check 9 evidence admission without requiring
implementation-heavy runtime proof.
EOF

  cat <<'EOF' >"$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Baseline evidence-admission validation path is available for the fixture.
EOF

  cat <<'EOF' >"$feature_dir/scopes.md"
# Scope 01: Evidence Admission Fixture

**Status:** Done

### Goal

Keep the fixture small while exercising the real transition guard against a
coherent delivery feature directory.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Scenario-specific regression row required by the guard. | `selftest:scenario-regression` | Yes |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader regression row required by the guard. | `selftest:broader-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF

  bubbles_sed_inplace "s|__SCENARIO_TEST__|$scenario_test|g" "$feature_dir/scopes.md"
  bubbles_sed_inplace "s|__BROADER_TEST__|$broader_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' >"$feature_dir/report.md"
# Report

### Summary

Evidence-admission transition-guard selftest fixture.

### Completion Statement

The temporary fixture is shaped to satisfy a supported delivery contract while
exercising the guard's state, artifact, and Check 9 evidence-admission checks.

### Test Evidence

```text
$ bash bubbles/scripts/agent-ownership-lint.sh
Agent ownership lint passed.
$ ls -la __FEATURE_DIR__/tests
total 16
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 3 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   41 Mar 27 00:00 docs-broader-regression.e2e.spec.ts
-rw-r--r-- 1 selftest selftest   42 Mar 27 00:00 docs-scenario-regression.e2e.spec.ts
```
EOF

  bubbles_sed_inplace "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

  cat <<'EOF' >"$feature_dir/state.json"
{
  "version": 3,
  "status": "docs_updated",
  "workflowMode": "docs-only",
  "execution": {
    "completedPhaseClaims": ["docs"]
  },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["01-evidence-admission-fixture"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "docs_updated"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "docs-only"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "docs",
      "completedAt": "2026-03-27T10:00:07Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:00:09Z"
}
EOF

  # Convert the docs-only base into a supported autonomous-goal DELIVERY contract
  # (byte-identical to the transition-guard selftest's mutate_delivery_contract).
  python3 - "$feature_dir/state.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["status"] = "in_progress"
data["workflowMode"] = "autonomous-goal"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = "autonomous-goal"

execution = data.setdefault("execution", {})
execution["completedPhaseClaims"] = ["test", "validate", "audit", "docs"]

certification = data.setdefault("certification", {})
certification["status"] = "in_progress"
certification["certifiedCompletedPhases"] = ["test", "validate", "audit", "docs"]

data["executionHistory"] = [
    {"phase": "test", "agent": "bubbles.test", "phasesExecuted": ["test"],
     "runStartedAt": "2026-03-27T10:00:00Z", "runCompletedAt": "2026-03-27T10:00:47Z",
     "completedAt": "2026-03-27T10:00:47Z"},
    {"phase": "validate", "agent": "bubbles.validate", "phasesExecuted": ["validate"],
     "runStartedAt": "2026-03-27T10:01:13Z", "runCompletedAt": "2026-03-27T10:02:31Z",
     "completedAt": "2026-03-27T10:02:31Z"},
    {"phase": "audit", "agent": "bubbles.audit", "phasesExecuted": ["audit"],
     "runStartedAt": "2026-03-27T10:03:02Z", "runCompletedAt": "2026-03-27T10:06:08Z",
     "completedAt": "2026-03-27T10:06:08Z"},
    {"phase": "docs", "agent": "bubbles.docs", "phasesExecuted": ["docs"],
     "runStartedAt": "2026-03-27T10:07:19Z", "runCompletedAt": "2026-03-27T10:11:44Z",
     "completedAt": "2026-03-27T10:11:44Z"},
]

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

# append_probe <scopes.md> <body...>  — adds items UNDER a probe heading that
# TERMINATES the Definition of Done section, so ONLY Check 9's whole-file scan
# evaluates them (DoD-section checks stop at the heading).
append_probe() {
  local scopes_file="$1"
  shift
  {
    printf '\n### Evidence Admission Probe\n\n'
    printf '%s\n' "$@"
  } >>"$scopes_file"
}

# -----------------------------------------------------------------------------
# Build the shared framework surface + fixtures.
# -----------------------------------------------------------------------------
mkdir -p "$tmp_root/specs"
clone_framework_surface "$tmp_root"
git -C "$tmp_root" init -q
export BUBBLES_REPO_ROOT="$tmp_root"

# Materialize the OLD (pre-fix) guard inside the CLONE so its sibling fragments +
# schema resolve; used ONLY for the non-tautology teeth-proof.
# A shallow clone (actions/checkout default fetch-depth: 1) does not carry that
# commit, so the teeth-proof degrades to SKIP rather than failing the selftest.
OLD_GUARD="$tmp_root/bubbles/scripts/state-transition-guard.sh"
OLD_GUARD_AVAILABLE=1
if git -C "$REPO_ROOT" rev-parse --verify --quiet "${OLD_GUARD_REF}^{commit}" >/dev/null 2>&1; then
  git -C "$REPO_ROOT" show "$OLD_GUARD_REF:bubbles/scripts/state-transition-guard.sh" >"$OLD_GUARD"
else
  OLD_GUARD_AVAILABLE=0
fi

# ---- CONTROL PASS (a): bare `- [x]` item + inline fenced command block ----
control_inline_dir="$tmp_root/specs/950-c9-control-inline"
emit_pass_fixture "$control_inline_dir"
append_probe "$control_inline_dir/scopes.md" \
  '- [x] IMP102 control inline fenced command output evidence block follows' \
  '' \
  '```text' \
  '$ echo running control inline probe' \
  'control inline probe output line one' \
  'control inline probe output line two' \
  'control inline probe output line three' \
  'control inline probe output line four' \
  'control inline probe output line five' \
  'control inline probe output line six' \
  'control inline probe output line seven' \
  'control inline probe output line eight' \
  'Exit Code: 0' \
  '```'

# ---- CONTROL PASS (b): resolver marker link to a real >=10-line fenced block ----
control_resolver_dir="$tmp_root/specs/951-c9-control-resolver"
emit_pass_fixture "$control_resolver_dir"
append_probe "$control_resolver_dir/scopes.md" \
  '- [x] IMP102 control real command output block via resolver -> Evidence: [realtest](report.md#realtest)'
cat <<'EOF' >>"$control_resolver_dir/report.md"

### Realtest

```text
$ cargo test --package evidence-admission
   Compiling evidence-admission v0.1.0
    Finished test profile in 4.12s
     Running unittests
running 6 tests
test admission::accepts_real_block ... ok
test admission::rejects_bare_marker ... ok
test result: ok. 6 passed; 0 failed; 0 ignored
Exit Code: 0
```
EOF

# ---- ADVISORY PASS (#3): resolver marker link to a 12-line PROSE block ----
advisory_prose_dir="$tmp_root/specs/952-c9-advisory-prose"
emit_pass_fixture "$advisory_prose_dir"
append_probe "$advisory_prose_dir/scopes.md" \
  '- [x] IMP102 prose attestation evidence -> Evidence: [notes](report.md#notes)'
cat <<'EOF' >>"$advisory_prose_dir/report.md"

### Notes

The documentation route ownership was reconciled across the spec and design
surfaces during this iteration and the reviewer confirmed the wording matches
the ratified naming policy for the affected navigation entries.
The attestation records that the maintainer audited each downstream reference
and verified the described behavior against the product principles document.
No runtime terminal capture applies to this purely documentary review item,
so the evidence here is an intentional narrative attestation rather than a
transcript, which the hardening accepts under the advisory allowance while
still surfacing the would-fail count for a future blocking policy.
Every referenced surface was cross-checked twice by the reviewing maintainer.
The reconciliation left the navigation ordering unchanged for both locales.
This paragraph provides the twelfth non-blank prose line for the fixture block.
EOF

# ---- BLOCKING (IMP-027 SCOPE-3): SAME prose block, but an EXECUTION claim ----
# Paired deliberately with the ADVISORY fixture above. The evidence block below
# is byte-identical in SHAPE (>=10 non-blank prose lines, no command output);
# the ONLY difference is that the DoD item asserts an execution outcome instead
# of a documentary one. That isolation is what proves the new rule keys on the
# CLAIM TYPE and not on the block, and it is what makes README's guarantee
# ('a narrative "all tests pass" with no terminal output is rejected as
# fabrication') literally true in code.
prose_execution_dir="$tmp_root/specs/958-c9-prose-execution-claim"
emit_pass_fixture "$prose_execution_dir"
append_probe "$prose_execution_dir/scopes.md" \
  '- [x] Full integration and e2e suites pass with zero failures -> Evidence: [suites](report.md#suites)'
cat <<'EOF' >>"$prose_execution_dir/report.md"

### Suites

The integration suite and the end-to-end suite were both exercised against the
ephemeral stack for this iteration and the maintainer reviewed the results in
detail before recording this attestation for the scope.
Every scenario enumerated in the test plan was walked through and the observed
behavior matched the specification in each case without deviation.
The reviewer additionally confirmed that no scenario was skipped and that the
suites covered each boundary condition named in the design document.
Coverage was inspected and judged sufficient for the behavior under change.
No regressions were observed in any previously passing area of the product.
This paragraph provides the twelfth non-blank prose line for the fixture block.
EOF

# ---- CHECK 43 (IMP-027 SCOPE-3, EV-2): receipt staleness on the transition path ----
# The guard resolves the receipt log at REPO root (`git rev-parse --show-toplevel`
# from the feature dir), which is correct for production but means a fixture
# cannot simply drop a log beside its own spec — it would resolve to the shared
# selftest tmp root and read another fixture's log. Each receipt fixture is
# therefore given its OWN git root via `git init`, making it genuinely hermetic.
#
# The FRESH and STALE fixtures are IDENTICAL except that the stale one's input
# file is mutated AFTER its receipt is written. That isolation is what proves
# Check 43 compares the recorded hashes rather than merely noticing a log.
_emit_receipt_fixture() {
  local dir="$1" mutate="$2"
  emit_pass_fixture "$dir"
  mkdir -p "$dir/.specify/runtime" "$dir/src"
  git -C "$dir" init --quiet >/dev/null 2>&1 || return 1
  printf 'original content\n' >"$dir/src/mod.rs"
  local h
  h="$(sha256sum "$dir/src/mod.rs" | awk '{print $1}')"
  printf '{"ts":"2026-07-28T00:00:00Z","cmd":"cargo test","spec":"001-x","inputClosure":[{"path":"src/mod.rs","sha256":"%s"}]}\n' \
    "$h" >"$dir/.specify/runtime/tool-calls.jsonl"
  [[ "$mutate" == "mutate" ]] && printf 'MUTATED after the receipt was captured\n' >"$dir/src/mod.rs"
  return 0
}

receipt_fresh_dir="$tmp_root/specs/959-c43-receipt-fresh"
_emit_receipt_fixture "$receipt_fresh_dir" keep

receipt_stale_dir="$tmp_root/specs/960-c43-receipt-stale"
_emit_receipt_fixture "$receipt_stale_dir" mutate

# ---- CHECK 43 clone detection (IMP-027 SCOPE-8, EV-3) ----
# Emits two receipts sharing one stdoutHash. The ONLY difference between the
# two fixtures is whether the two receipts name the SAME command:
#   same cmd  -> an honest re-run of a deterministic suite  -> MUST PASS
#   diff cmd  -> one captured output reused for a second claim -> MUST BLOCK
# Without the same-cmd fixture the check would look correct while silently
# failing every project that runs its test suite twice.
_emit_clone_fixture() {
  local dir="$1" second_cmd="$2"
  emit_pass_fixture "$dir"
  mkdir -p "$dir/.specify/runtime"
  git -C "$dir" init --quiet >/dev/null 2>&1 || return 1
  {
    printf '{"ts":"2026-07-28T00:00:00Z","cmd":"cargo test","spec":"001-x","exitCode":0,"stdoutHash":"deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234"}\n'
    printf '{"ts":"2026-07-28T00:05:00Z","cmd":"%s","spec":"001-x","exitCode":0,"stdoutHash":"deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234"}\n' "$second_cmd"
  } >"$dir/.specify/runtime/tool-calls.jsonl"
  return 0
}

clone_diffcmd_dir="$tmp_root/specs/961-c43-clone-different-commands"
_emit_clone_fixture "$clone_diffcmd_dir" 'npm run lint'

clone_samecmd_dir="$tmp_root/specs/962-c43-clone-same-command-rerun"
_emit_clone_fixture "$clone_samecmd_dir" 'cargo test'

# The same command is routinely SPELLED two ways across a session: one directory
# named relatively then absolutely, and an optional trailing filter argument
# that narrows a run without making it a different claim. Both re-runs produce
# identical stdout because they ARE the same command, so neither may be
# reported as a clone. Comparing raw argv strings accused both.
_emit_clone_fixture_pair() {
  local dir="$1" first_cmd="$2" second_cmd="$3"
  emit_pass_fixture "$dir"
  mkdir -p "$dir/.specify/runtime"
  git -C "$dir" init --quiet >/dev/null 2>&1 || return 1
  {
    printf '{"ts":"2026-07-28T00:00:00Z","cmd":"%s","spec":"001-x","exitCode":0,"stdoutHash":"deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234"}\n' "$first_cmd"
    printf '{"ts":"2026-07-28T00:05:00Z","cmd":"%s","spec":"001-x","exitCode":0,"stdoutHash":"deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234deadbeefcafe1234"}\n' "$second_cmd"
  } >"$dir/.specify/runtime/tool-calls.jsonl"
  return 0
}

clone_respelled_dir="$tmp_root/specs/963-c43-clone-same-command-respelled"
_emit_clone_fixture_pair "$clone_respelled_dir" \
  'bash bubbles/scripts/some-guard.sh --repo-root . --phase mvp' \
  'bash bubbles/scripts/some-guard.sh --repo-root /abs/path --phase mvp'

clone_optarg_dir="$tmp_root/specs/964-c43-clone-same-command-optional-arg"
_emit_clone_fixture_pair "$clone_optarg_dir" \
  'bash bubbles/scripts/artifact-lint.sh specs/095-x' \
  'bash bubbles/scripts/artifact-lint.sh specs/095-x SCN-095-CI01'

# ---- BLOCKING (#1): truly-bare `-> Evidence: done` marker ----
bare_marker_dir="$tmp_root/specs/953-c9-bare-marker"
emit_pass_fixture "$bare_marker_dir"
append_probe "$bare_marker_dir/scopes.md" \
  '- [x] IMP102 fabricated completion with a truly bare marker -> Evidence: done'

# ---- BLOCKING (#2): plain link to a report file with <10 non-blank lines ----
thin_report_dir="$tmp_root/specs/954-c9-thin-report"
emit_pass_fixture "$thin_report_dir"
# A side report file (NOT report.md, so Check 11 keeps scanning the fat report.md)
# with fewer than 10 non-blank lines. fix #2 requires the linked report to carry
# >=10 non-blank lines.
cat <<'EOF' >"$thin_report_dir/thin-report.md"
# Thin Report

### Summary

Deliberately thin evidence surface.

### Test Evidence

Only a few lines here.
EOF
append_probe "$thin_report_dir/scopes.md" \
  '- [x] IMP102 plain link to a near-empty report [thin](thin-report.md)'

# ---- BLOCKING (#5): uppercase `- [X]` item with no evidence ----
uppercase_dir="$tmp_root/specs/955-c9-uppercase"
emit_pass_fixture "$uppercase_dir"
append_probe "$uppercase_dir/scopes.md" \
  '- [X] IMP102 uppercase checkbox carrying no evidence marker present anywhere'

# ---- BLOCKING (#7): duplicated identical `- [x]` lines; only the FIRST evidenced ----
duplicate_dir="$tmp_root/specs/956-c9-duplicate"
emit_pass_fixture "$duplicate_dir"
append_probe "$duplicate_dir/scopes.md" \
  '- [x] IMP102 duplicated evidence admission fixture identical line body' \
  '' \
  '```text' \
  '$ echo running duplicate probe' \
  'duplicate probe output line one' \
  'duplicate probe output line two' \
  'duplicate probe output line three' \
  'duplicate probe output line four' \
  'duplicate probe output line five' \
  'duplicate probe output line six' \
  'duplicate probe output line seven' \
  'Exit Code: 0' \
  '```' \
  '' \
  '- [x] IMP102 duplicated evidence admission fixture identical line body'

# -----------------------------------------------------------------------------
# Tool-log fixtures (#4, #6) + a positive tool-log control. They live under the
# SHARED framework-clone root (so Check 13/23 resolve the framework surface like
# every other fixture) and share ONE tool-calls.jsonl at the repo root — exactly
# where the guard resolves the log from (the feature's git toplevel is tmp_root).
# The structured-log matcher is spec-scoped and evaluates the spec match BEFORE
# the token match, so each entry can only cover its OWN spec — no cross-fixture
# bleed. Each probe item is a BARE `- [x]` with no markdown/inline evidence, so
# ONLY the structured tool-log path (Check 9 case 4) can cover it.
# -----------------------------------------------------------------------------
TOOLLOG_PROBE='- [x] IMP102 pytest evidence admission passed via structured tool log'

toollog_ok_dir="$tmp_root/specs/957-c9-toollog-ok"
emit_pass_fixture "$toollog_ok_dir"
append_probe "$toollog_ok_dir/scopes.md" "$TOOLLOG_PROBE"

toollog_emptyspec_dir="$tmp_root/specs/958-c9-empty-spec"
emit_pass_fixture "$toollog_emptyspec_dir"
append_probe "$toollog_emptyspec_dir/scopes.md" "$TOOLLOG_PROBE"

toollog_forged_dir="$tmp_root/specs/959-c9-forged-key"
emit_pass_fixture "$toollog_forged_dir"
append_probe "$toollog_forged_dir/scopes.md" "$TOOLLOG_PROBE"

# Shared structured evidence log at the repo root (the feature git toplevel).
#   OK  -> a VALID, spec-scoped, exit0, token-matching entry COVERS 957 (positive control).
#   #6  -> spec:"" names nothing; fix #6 skips it, so 958 stays unevidenced -> BLOCKS.
#   #4  -> an UNKNOWN extra key is schema-invalid under additionalProperties:false;
#          fix #4 skips it, so 959 stays unevidenced -> BLOCKS.
mkdir -p "$tmp_root/.specify/runtime"
{
  printf '%s\n' '{"ts":"2026-03-27T10:00:00Z","sessionId":"s-ok","spec":"957-c9-toollog-ok","cmd":"pytest evidence admission passed","exitCode":0}'
  printf '%s\n' '{"ts":"2026-03-27T10:00:00Z","sessionId":"s-empty","spec":"","cmd":"pytest evidence admission passed","exitCode":0}'
  printf '%s\n' '{"ts":"2026-03-27T10:00:00Z","sessionId":"s-forged","spec":"959-c9-forged-key","cmd":"pytest evidence admission passed","exitCode":0,"forgedExtraKey":"tampered"}'
} >"$tmp_root/.specify/runtime/tool-calls.jsonl"

# -----------------------------------------------------------------------------
# Assertions
# -----------------------------------------------------------------------------
echo "=== CONTROL PASS cases ==="
assert_passes "$control_inline_dir" no \
  "CONTROL (a): inline fenced command block (cmd + exit 0 + >=10 lines) passes with no advisory"
assert_passes "$control_resolver_dir" no \
  "CONTROL (b): resolver link to a real >=10-line fenced command block passes with no advisory"

echo ""
echo "=== ADVISORY PASS case (fix #3) ==="
assert_passes "$advisory_prose_dir" yes \
  "ADVISORY (#3): resolved 12-line prose block accepted AND emits Check-9 ADVISORY"

echo ""
echo "=== CHECK 43 receipt staleness (IMP-027 SCOPE-3, EV-2) ==="
assert_passes "$receipt_fresh_dir" any \
  "CHECK 43 (fresh): receipt whose inputClosure still matches the tree passes"
assert_blocks_with "$receipt_stale_dir" \
  "Evidence receipt(s) are STALE" \
  "CHECK 43 (stale): receipt whose input changed after capture BLOCKS"
assert_blocks_with "$clone_diffcmd_dir" \
  "Evidence receipt CLONE" \
  "CHECK 43 (clone): one stdout hash cited by TWO DIFFERENT commands BLOCKS"
assert_passes "$clone_samecmd_dir" any \
  "CHECK 43 (re-run): same stdout hash from the SAME command is honest, passes"
assert_passes "$clone_respelled_dir" any \
  "CHECK 43 (re-spelled): same command with an equivalent --repo-root value passes"
assert_passes "$clone_optarg_dir" any \
  "CHECK 43 (optional arg): same command and subject with an extra filter argument passes"

echo ""
echo "=== BLOCKING FAIL cases ==="
assert_blocks_with "$prose_execution_dir" \
  "asserts an EXECUTION outcome but its evidence block" \
  "BLOCK (IMP-027 SCOPE-3): prose-only block backing an EXECUTION claim"
assert_blocks_with "$bare_marker_dir" \
  "has a bare Evidence marker with no report.md reference or inline evidence block" \
  "BLOCK (#1): truly-bare '-> Evidence: done' marker"
assert_blocks_with "$thin_report_dir" \
  "links report.md but it is missing or has <10 non-blank lines" \
  "BLOCK (#2): plain link to a report with <10 non-blank lines"
assert_blocks_with "$uppercase_dir" \
  "has NO evidence block" \
  "BLOCK (#5): uppercase '- [X]' item with no evidence"
assert_blocks_with "$duplicate_dir" \
  "has NO evidence block" \
  "BLOCK (#7): duplicated identical '- [x]' lines, only the first evidenced"
assert_blocks_with "$toollog_emptyspec_dir" \
  "has NO evidence block" \
  "BLOCK (#6): tool-log entry with spec:\"\" is non-matching"

if [[ "$HAVE_JSONSCHEMA" -eq 1 ]]; then
  assert_blocks_with "$toollog_forged_dir" \
    "has NO evidence block" \
    "BLOCK (#4): schema-invalid tool-log line (unknown key) is non-matching"
else
  echo "SKIP: BLOCK (#4) schema-invalid tool-log line — python 'jsonschema' not importable"
fi

echo ""
echo "=== Tool-log POSITIVE control (proves #4/#6 are non-tautological) ==="
# Same bare `- [x]` probe as #4/#6, but here the log carries a VALID, spec-scoped
# entry. Check 9 case 4 COVERS the item — the SAME item Check 9 REJECTS for the
# empty-spec / schema-invalid entries above. That accept-vs-reject difference is
# the discriminator that proves fixes #4/#6 have teeth (the item is not blocked
# by Check 9 unconditionally). The full guard still blocks this fixture on the
# unrelated, not-tool-log-aware artifact-lint (Check 13), so this pins the
# Check-9 verdict rather than the exit code.
assert_check9_covers "$toollog_ok_dir" \
  "CONTROL (tool-log): a VALID spec-scoped entry makes Check 9 COVER the bare item"

echo ""
echo "=== NON-TAUTOLOGY: #1 and #5 must PASS on the OLD guard ($OLD_GUARD_REF) ==="
if [[ "$OLD_GUARD_AVAILABLE" -eq 1 ]]; then
  assert_old_guard_passes "$bare_marker_dir" \
    "NON-TAUTOLOGY (#1): bare-marker fixture passes the OLD guard"
  assert_old_guard_passes "$uppercase_dir" \
    "NON-TAUTOLOGY (#5): uppercase-checkbox fixture passes the OLD guard"
else
  echo "SKIP: NON-TAUTOLOGY (#1/#5) — old-guard commit $OLD_GUARD_REF is absent from this clone (shallow checkout); fetch full history to run the teeth-proof"
fi

echo ""
echo "=============================================================="
echo "evidence-admission-hardening-selftest: $passed passed / $failed failed"
echo "=============================================================="
if [[ "$failed" -gt 0 ]]; then
  exit 1
fi
exit 0
