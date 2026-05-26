---
name: bubbles-evidence-capture
description: Capture valid execution evidence for Bubbles report.md sections and DoD items. Use when recording test runs, build results, lint results, curl/HTTP checks, database queries, UI verification, or any claim that a command produced a specific outcome. Triggers include writing a "Test Evidence" or "Verification" section in report.md, pasting terminal output, or building an evidence block linked from a DoD checkbox.
---

# Bubbles Evidence Capture

## Goal
Produce evidence blocks that pass the Bubbles ≥10-line raw-output standard and survive the state-transition guard, the artifact lint, and audit review.

## When to use
- Recording per-DoD-item evidence inline in `scopes.md`
- Filling out `report.md` evidence sections (Test Evidence, Verification, Round-trip, Freshness)
- Capturing bug reproduction (before-fix and after-fix) evidence
- Recording specialist sub-agent run output

## Required evidence shape
For every command/tool whose outcome you cite:

````markdown
**Executed:** YES (in current session)
**Command:** `<exact command line>`
**Exit Code:** <actual exit code observed>
**Output:**

```
<raw terminal output, ≥10 lines, copy/pasted verbatim>
```

**Result:** PASS / FAIL
````

Rules:
- The command line must be the literal command run, not a sanitized or paraphrased version.
- The output block must contain ≥10 lines of real output. If the command emits fewer lines, run a richer flag set (`-v`, `--verbose`, `--explain`), or include the full stdout+stderr including framing lines, exit indicator, and any post-summary lines so the block is honest and ≥10 lines.
- Do not paste expected output. Paste observed output.
- Do not invent line counts, durations, or row counts.

## Categories that require live execution
| DoD claim | Required action | Forbidden substitute |
|-----------|-----------------|----------------------|
| Unit tests pass | Run the project test command | Reading the test file |
| Integration tests pass | Run integration command against live test stack | Reading code |
| API endpoint works | HTTP request against the running stack (with a per-request timeout) | Reading the router |
| Database state correct | Run a SQL query against the live test DB | Reading the migration |
| UI displays correctly | Open browser / capture DOM / browser-automation snapshot | Reading the UI component source |
| Build succeeds | Run the project build command | Reading build config |
| Lint passes | Run the lint command | Reading the lint config |
| Coverage threshold met | Run coverage report | Computing it mentally |

## Anti-patterns that auto-reject evidence
- Evidence section shorter than 10 lines
- Identical evidence blocks reused across multiple DoD items
- All timestamps in evidence are identical (parallel runs not proven)
- Output appears to be a summary (e.g., "12 passed, 0 failed") with no surrounding test runner framing
- Evidence claims "Exit Code: 0" but no command line is shown
- Evidence references a prior session's terminal

## PII discipline (NON-NEGOTIABLE before commit)
Real home paths (`/home/<username>/...`) leak personal data and are blocked by `pii-scan.sh` / `gitleaks`. Before committing evidence:
1. Search the new evidence for `/home/<user>/...` paths
2. Replace with `~/<repo>/...` (tilde form) in every occurrence
3. Re-stage and commit

This is a project-local rule but reliably blocks pushes when violated.

## Authoritative modules
- `agents/bubbles_shared/evidence-rules.md` — full Evidence Provenance Taxonomy, Uncertainty Declaration Protocol
- `agents/bubbles_shared/quality-gates.md` — canonical test categories that govern which evidence type is required
- `agents/bubbles_shared/completion-governance.md` — per-DoD evidence linkage rules
- `bubbles/scripts/artifact-lint.sh` — section presence enforcement
