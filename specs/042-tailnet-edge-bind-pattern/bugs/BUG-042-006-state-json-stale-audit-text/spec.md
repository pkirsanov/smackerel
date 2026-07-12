# Bug: BUG-042-006 — Spec 042 `state.json` audit text is stale and contradicts the current Gate G028 fail-loud policy

## Classification

- **Type:** Documentation defect — stale audit narrative in append-only spec history (zero runtime impact)
- **Severity:** P2 — MEDIUM (live `deploy/compose.deploy.yml` is contract-compliant on HEAD; the defect is misleading historical narrative that contradicts the current binding policy and would cause a future agent or operator to mis-restore the FORBIDDEN form if they trusted the spec history as authoritative)
- **Parent Spec:** 042 — Tailnet-Edge Bind Pattern (Self-Hosted Compose Readiness)
- **Workflow Mode:** bugfix-fastlane
- **Status:** Reported (Confirmed via line-precise grep)
- **Discovered By:** 2026-05-15 self-hosted readiness re-scan (finding HL-RESCAN-007)

## Summary

`specs/042-tailnet-edge-bind-pattern/state.json` is the append-only audit history for the spec 042 bugfix-fastlane chain. Across at least 5 separate `phaseLog`-style records (4 in `execution.completedPhaseClaims[*].notes`, plus 4 more in `execution.pendingTransitionRequests[*]` `reason` / `closeReason` fields), the audit narrative explicitly **praises** the now-FORBIDDEN substitution form `${HOST_BIND_ADDRESS:-127.0.0.1}:` as "preserving spec 020's loopback default", "the simplest shape satisfying REQ-1 + REQ-2", and "safe defaults". Those narratives were technically accurate at the time they were written (2026-05-09), because the live `deploy/compose.deploy.yml` used the `:-127.0.0.1` form back then.

That decision was **reversed** by the BUG-029-003 close-out at HEAD commit `eec1437c` (2026-05-14): the live `deploy/compose.deploy.yml` now uses the fail-loud SST form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per **Gate G028 NO-DEFAULTS** policy; the `${VAR:-default}` substitution form is now explicitly FORBIDDEN by both `.github/instructions/smackerel-no-defaults.instructions.md` and `.github/copilot-instructions.md` for any SST-managed runtime value (including `HOST_BIND_ADDRESS`).

The spec 042 audit text was never reconciled with the policy reversal. Today, an agent or operator reading `specs/042-tailnet-edge-bind-pattern/state.json` would conclude that `${HOST_BIND_ADDRESS:-127.0.0.1}:` is the canonical, recommended pattern. That conclusion is **wrong on HEAD** and contradicts:

- The live file (`deploy/compose.deploy.yml` lines 128, 185, 243, 315 — all use `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`)
- The compose contract test (`internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallbackBind` rejects the `:-` form for smackerel-core/ml; `TestComposeContract_AdversarialOllamaLiteralBind` rejects it for ollama; `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` (BUG-042-005) rejects it for prometheus)
- The agent-binding instructions (`.github/instructions/smackerel-no-defaults.instructions.md`)
- The agent-binding copilot rules (`.github/copilot-instructions.md` "Tailnet-Edge Bind Pattern" section)

## Detection

| Aspect | Detail |
|---|---|
| Trigger | 2026-05-15 self-hosted readiness re-scan |
| Finding | HL-RESCAN-007 (lens: generic-only / SST-defaults; surface: `specs/042-tailnet-edge-bind-pattern/state.json`) |
| Severity | P2 (live file already compliant on HEAD; the defect is **stale audit narrative** that misleads future readers) |
| Audit method | Line-precise `grep -nE '127\.0\.0\.1\|loopback default\|loopback-only default\|preserves.*default'` against `specs/042-tailnet-edge-bind-pattern/state.json` returned 20+ matches; 5 distinct praise-narrative records identified across `execution.completedPhaseClaims[*].notes` and `execution.pendingTransitionRequests[*]` fields. Cross-referenced HEAD commit `eec1437c` (BUG-029-003) which reversed the substitution policy and `.github/instructions/smackerel-no-defaults.instructions.md` which codifies the reversal as binding agent policy. Confirmed live `deploy/compose.deploy.yml` already uses the fail-loud `:?` form on lines 128, 185, 243, 315. Confirmed Compose contract test `internal/deploy/compose_contract_test.go::TestComposeContract_AdversarialDefaultFallbackBind` (BUG-042-004) and `::TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` (BUG-042-005) actively reject the praised form. |

## Reproduction

```bash
# Confirm the stale praise text exists
grep -nE '\$\{HOST_BIND_ADDRESS:-127\.0\.0\.1\}|loopback default|loopback-only default|preserves spec 020' \
  specs/042-tailnet-edge-bind-pattern/state.json

# Confirm the live file uses the FORBIDDEN-by-stale-audit-but-actually-REQUIRED fail-loud form
grep -nE 'HOST_BIND_ADDRESS' deploy/compose.deploy.yml

# Confirm the binding policy explicitly FORBIDS the praised form
grep -nE 'HOST_BIND_ADDRESS|FORBIDDEN' .github/instructions/smackerel-no-defaults.instructions.md
```

Expected: stale praise narrative is found in spec 042 `state.json`; live file uses `${HOST_BIND_ADDRESS:?...}` form; binding policy explicitly forbids `${HOST_BIND_ADDRESS:-...}` form.

## Stale Audit Lines (Evidence)

The following table enumerates every distinct `phaseLog`-style record in `specs/042-tailnet-edge-bind-pattern/state.json` whose narrative praises the now-FORBIDDEN `${HOST_BIND_ADDRESS:-127.0.0.1}:` substitution form. All cited line numbers are 1-based against the current file at HEAD `eec1437c`. All cited substrings are literal excerpts (verbatim, with original Unicode em-dashes preserved).

| # | Line | Field path | Stale literal excerpt |
|---|------|-----------|------------------------|
| 1 | 44 | `execution.completedPhaseClaims[3].notes` (regression specialist) | `(spec 042 supersedes spec 020 for the two app services while preserving loopback default via ${HOST_BIND_ADDRESS:-127.0.0.1} substitution). (Step 3) Design coherence: VERIFIED — substitution preserves spec 020's loopback behavior while enabling Pattern P5 tailnet-edge fronting.` |
| 2 | 52 | `execution.completedPhaseClaims[4].notes` (simplify specialist) | `(2) deploy/compose.deploy.yml — 220 lines; no opportunity (substitution form ${HOST_BIND_ADDRESS:-127.0.0.1}: is the simplest shape satisfying REQ-1 + REQ-2)` |
| 3 | 60 | `execution.completedPhaseClaims[5].notes` (stabilize specialist) | `smackerel-ml renders with host_ip=127.0.0.1 target=8081 published=41002 (proves Docker Compose ${VAR:-default} substitution preserves spec 020 loopback default for spec 042)` and `Infrastructure ✅ (compose render deterministic, loopback default preserved, tailnet-edge override path retained)` |
| 4 | 68 | `execution.completedPhaseClaims[6].notes` (security specialist) | `(1) Loopback-default safety — Compose substitution form ${HOST_BIND_ADDRESS:-127.0.0.1}: at deploy/compose.deploy.yml lines 109 (smackerel-core) + 155 (smackerel-ml) preserves spec 020's loopback-only default when variable is unset in BOTH app.env AND shell` and `A05 ✅ safe defaults (loopback-default)` |
| 5 | 212 | `execution.pendingTransitionRequests[*].reason` (validate→simplify routing packet, since closed) | `(2) confirm the compose substitution form ${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT} is the simplest shape that satisfies both REQ-1 (loopback default) and REQ-2 (tailnet-edge override)` |
| 6 | 222 | `execution.pendingTransitionRequests[*].reason` (simplify→stabilize routing packet, since closed) | `(3) confirm docker compose ... config rendering of deploy/compose.deploy.yml resolves smackerel-core and smackerel-ml ports to 127.0.0.1:<port>:<container-port> when HOST_BIND_ADDRESS is unset (default loopback behavior preserved)` |
| 7 | 226 | `execution.pendingTransitionRequests[*].closeReason` (stabilize transition close) | `docker compose render with HOST_BIND_ADDRESS unset in BOTH env file AND shell resolves smackerel-core to host_ip=127.0.0.1 published=41001 and smackerel-ml to host_ip=127.0.0.1 published=41002 (proves ${VAR:-default} substitution preserves spec 020 loopback default)` |
| 8 | 232 | `execution.pendingTransitionRequests[*].reason` (stabilize→security routing packet, since closed) | `(1) review the spec-042 surface for security implications of the loopback-default + tailnet-edge override pattern — specifically, the compose binding semantics ${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}` |
| 9 | 234 | `execution.pendingTransitionRequests[*].closeReason` (security transition close) | `(1) Loopback-default safety preserved (Compose substitution ${HOST_BIND_ADDRESS:-127.0.0.1}: resolves to 127.0.0.1 when unset in both env file AND shell — verified by stabilize phase render)` and `A05 ✅ safe defaults` |

These nine excerpts span four `completedPhaseClaims` records (regression, simplify, stabilize, security) and five transition-queue fields (one routing-packet `reason` + one `closeReason` for the simplify, stabilize, and security transitions). Per the user's classification, this constitutes "5+ stale phaseLog notes entries" (the count is 9 distinct excerpts across 9 distinct fields).

## Root Cause

`state.json` is the **append-only** version-3 control-plane audit history for a spec's lifecycle. It was designed to be a tamper-evident, time-ordered record of what each agent observed, claimed, and certified at each phase. By design, historical entries are NEVER deleted or rewritten — they reflect the truth at the moment they were written.

When the spec 042 bugfix-fastlane chain ran (2026-05-09), `deploy/compose.deploy.yml` used the `${HOST_BIND_ADDRESS:-127.0.0.1}:` form. Every audit narrative recorded by the regression / simplify / stabilize / security / validate specialists was an honest, accurate observation of that file at that time.

Subsequently:

- `BUG-042-003` (2026-05-13, HEAD `ded2fe5d`) added ollama coverage and tightened the contract.
- `BUG-042-004` (2026-05-14, HEAD `da263ffe`) added adversarial coverage for the `${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form on `smackerel-core` and `smackerel-ml`. **This is the moment the substitution policy reversal began** — the test suite started actively rejecting the form spec 042's audit had praised.
- `BUG-042-005` (2026-05-14, HEAD `6cdabe62`) extended the same adversarial coverage to `prometheus`.
- `BUG-029-003` (2026-05-14, HEAD `eec1437c` — current HEAD) closed self-hosted readiness re-scan finding HL-RESCAN-012 by sweeping ALL remaining `${VAR:-default}` substitutions out of the dev `docker-compose.yml` and authoring a new persistent in-tree contract test (`internal/deploy/dev_compose_default_fallback_test.go`) that locks the no-defaults policy on the dev compose file. This commit also strengthened the binding agent rule in `.github/instructions/smackerel-no-defaults.instructions.md` and `.github/copilot-instructions.md` to explicitly forbid the `${VAR:-default}` form for `HOST_BIND_ADDRESS` (and any SST-managed value).

The spec 042 `state.json` audit history was never updated to reconcile the prior praise narrative with the new policy. The audit-history-immutability contract correctly prevents anyone from deleting or rewriting the substance of the historical entries — but it does NOT prevent (and in fact REQUIRES) appending a new reconciliation entry that explicitly supersedes the prior narrative and annotating each stale entry with a leading supersession marker.

The defect is therefore a **missing reconciliation entry plus missing supersession annotations** on a set of historically-correct-but-now-misleading narratives, NOT a fault in any of the original agents who wrote those narratives.

## Expected Behavior

A reader of `specs/042-tailnet-edge-bind-pattern/state.json` at any commit at or after HEAD `eec1437c` should be able to determine, without leaving the file, that:

1. The historical narratives praising `${HOST_BIND_ADDRESS:-127.0.0.1}:` were accurate at the time they were written but were later **superseded** by a policy reversal in BUG-029-003 (HEAD `eec1437c`).
2. The current binding form is `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` per Gate G028 NO-DEFAULTS policy.
3. Each stale narrative carries a leading `[SUPERSEDED by BUG-029-003 — see reconciliation entry below]` (or equivalent) marker so it cannot be silently mis-cited as current authority.
4. A single reconciliation entry at the tail of `execution.completedPhaseClaims` (or in a `policyReconciliationLog` companion field, design choice deferred to bubbles.design) explicitly documents the policy reversal, the supersession scope, and the citation chain.

## Actual Behavior

A reader of `specs/042-tailnet-edge-bind-pattern/state.json` at HEAD `eec1437c` encounters 9 distinct stale excerpts across 4 `completedPhaseClaims` records and 5 transition-queue fields, all of which praise the now-FORBIDDEN form as "loopback default", "the simplest shape", "preserves spec 020's loopback-only default", or "safe defaults". No supersession marker. No reconciliation entry. No back-link to BUG-029-003 or to Gate G028.

## Acceptance Criteria

- **AC-1**: A new entry titled `spec_042_audit_reconciliation_post_BUG-029-003` is appended to `execution.completedPhaseClaims` (or to a clearly-named adjacent field — design decision deferred to bubbles.design). The entry explicitly documents the policy reversal: cites BUG-029-003 (HEAD commit `eec1437c`), cites Gate G028 / `.github/instructions/smackerel-no-defaults.instructions.md` as the new binding authority, names the new fail-loud form `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`, names the now-FORBIDDEN form `${HOST_BIND_ADDRESS:-127.0.0.1}:`, and explicitly supersedes the 9 excerpts in the per-line table above.
- **AC-2**: Each of the 9 stale excerpts (in the 4 `completedPhaseClaims[*].notes` records AND in the 5 transition-queue fields) is annotated with a leading `[SUPERSEDED by BUG-029-003 — see reconciliation entry below]` (or equivalent) tag inserted at the **start of the relevant `notes` / `reason` / `closeReason` string**. The historical substance of the original narrative is preserved verbatim; only the leading marker is added. (The exact marker wording and insertion convention is finalized by bubbles.design; this AC pins the requirement, not the wording.)
- **AC-3**: After all annotations and the reconciliation entry are written, `python3 -c 'import json; json.load(open("specs/042-tailnet-edge-bind-pattern/state.json"))'` exits 0 (the file remains valid JSON).
- **AC-4**: After the fix, `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/` exits 0 (or fails ONLY with pre-existing advisory warnings unrelated to this fix). No new artifact-lint violations are introduced.
- **AC-5**: A persistent regression contract test (static-file lint) asserts that `specs/042-tailnet-edge-bind-pattern/state.json` contains the reconciliation entry AND that no `notes` / `reason` / `closeReason` string in the file praises the `${HOST_BIND_ADDRESS:-127.0.0.1}:` form without a leading `[SUPERSEDED by BUG-029-003 ...]` marker. The test is adversarial: removing the reconciliation entry OR stripping the supersession marker from any one of the 9 excerpts MUST cause the test to FAIL RED.
- **AC-6**: No historical narrative substance is deleted or rewritten. The append-only audit-immutability contract is preserved. (Verified by `git diff` showing every original narrative substring is still present in the file, just with an added prefix.)

## Out of Scope

- **Touching `deploy/compose.deploy.yml`** — already correct on HEAD; using the fail-loud `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` form per BUG-029-003.
- **Touching `internal/deploy/compose_contract_test.go`** — already correctly enforces the fail-loud form via the BUG-042-003 / BUG-042-004 / BUG-042-005 adversarial sub-tests.
- **Modifying `.github/instructions/smackerel-no-defaults.instructions.md`** — already correctly forbids the `${VAR:-default}` form per Gate G028.
- **Modifying any other spec** — this fix is scoped to `specs/042-tailnet-edge-bind-pattern/state.json` reconciliation only. (Other specs with similar stale narratives — e.g., spec 044's `state.json` — were already reconciled by HEAD `b715d143` HL-RESCAN-007 close-out and serve as the precedent template for this fix.)
- **Re-writing or deleting any historical phaseLog substance** — the audit-immutability contract is non-negotiable.
- **Modifying the spec 042 `report.md` evidence sections** — those evidence captures reflect the file as it was at the time of the spec 042 chain; reconciliation belongs in `state.json` (the audit history surface), not in the per-phase evidence captures.

## Severity Justification

**P2 — MEDIUM, NOT P1 — HIGH**:

- Live `deploy/compose.deploy.yml` is already compliant on HEAD; no runtime risk.
- Compose contract test actively rejects the praised form for all 4 operator-facing services (smackerel-core, smackerel-ml, ollama, prometheus); no test-surface risk.
- `.github/instructions/smackerel-no-defaults.instructions.md` and `.github/copilot-instructions.md` are the binding agent authorities and already explicitly forbid the praised form; no future agent who consults the binding instructions can mis-restore the form.
- The risk is **only** that a future agent or operator who consults the spec 042 `state.json` audit history as authoritative documentation (without cross-referencing the binding instructions or the live file) would conclude the praised form is canonical. That mis-conclusion would be caught at PR review (the compose contract test would fail) or at agent-policy review (smackerel-no-defaults skill would block) — but the misleading historical narrative still represents a real defect against the spec 042 audit history's role as a tamper-evident truth surface.

**Not P3 — LOW** because the volume (9 distinct excerpts) and the prominence of the misleading narrative (it spans 4 of the 8 `completedPhaseClaims` records, including the security and stabilize specialist verdicts that an operator might reasonably trust as authoritative) elevates this above a single isolated typo or comment defect.

## Related

- **Parent Spec:** `specs/042-tailnet-edge-bind-pattern/`
- **Policy reversal source of truth:** `specs/029-devops-pipeline/bugs/BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/` (HEAD commit `eec1437c`)
- **Binding policy (agent-facing):** [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md)
- **Binding policy (workspace-facing):** [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md) "Tailnet-Edge Bind Pattern" subsection inside Required Runtime Standards
- **Gate authority:** Gate G028 (NO-DEFAULTS / fail-loud SST policy)
- **Compose contract enforcement:** [`internal/deploy/compose_contract_test.go`](../../../../internal/deploy/compose_contract_test.go)
  - `TestComposeContract_AdversarialDefaultFallbackBind` (BUG-042-004) — rejects `:-` form for `smackerel-core` + `smackerel-ml`
  - `TestComposeContract_AdversarialOllamaLiteralBind` (BUG-042-003) — rejects literal + `:-` for `ollama`
  - `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` (BUG-042-005) — rejects literal + `:-` for `prometheus`
- **Precedent:** spec 044 stale-audit reconciliation HL-RESCAN-007 close-out at HEAD `b715d143` (`fix(044): close HL-RESCAN-007 — mark stale audit text as superseded + scrub stray philipk PII`) — the established `[SUPERSEDED 2026-05-14 by spec 042 hardening; ...]` annotation pattern this fix replicates.
- **Live file (correct on HEAD):** [`deploy/compose.deploy.yml`](../../../../deploy/compose.deploy.yml) lines 128, 185, 243, 315 — all use `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
