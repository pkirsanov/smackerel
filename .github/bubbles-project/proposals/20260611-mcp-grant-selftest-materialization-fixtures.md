# Bubbles Framework Change Proposal

- Title: mcp-grant selftest T2a/T6 fixtures ignore v7.7 server-token materialization
- Slug: mcp-grant-selftest-materialization-fixtures
- Created: 2026-06-11
- Created From: smackerel
- Requested Upstream Repo: bubbles

## Summary

`.github/bubbles/scripts/mcp-grant-selftest.sh` fails 2 of 22 cases (T2a, T6)
in a downstream repo whose MCP server-token slug is non-default (e.g.
`smackerel` → materialized token `bubbles-smackerel`). The failures are in the
selftest's **own expectation fixtures**, not in the behavior under test:

```
FAIL: T2a inject appends sorted grants in canonical format
  expected: [tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright, context7, github]]
  actual:   [tools: [read, search, edit, agent, todo, web, execute, bubbles-smackerel, playwright, context7, github]]
FAIL: T6 inject(granted, empty-grants config) == canonical (grant removed)
  expected: [... tools: [read, search, edit, agent, todo, web, execute, bubbles, playwright] ...]
  actual:   [... tools: [read, search, edit, agent, todo, web, execute, bubbles-smackerel, playwright] ...]
mcp-grant-selftest: 20 passed, 2 failed
```

The `actual` output is **correct**: `bubbles_mcp_inject_to_stdout` materializes
the core server token `bubbles` → `bubbles-<repo-slug>`, exactly as the v7.7
feature documented in this same file's header (lines 5–14: "inject materializes
`bubbles` → `bubbles-<slug>`"). The materialization-aware cases T8, T9, T11,
T11b, T12, T12b all PASS because they expect the materialized token. Only T2a
(static `EXPECT_GRANTED_LINE`, line ~110) and T6 (static empty-grants
expectation) still hardcode the pre-materialization literal `bubbles`, so they
fail whenever the slug is not the framework default.

## Why This Must Be Upstream

`mcp-grant-selftest.sh` is framework-managed (listed in `.github/bubbles/.manifest`).
A local edit trips the framework-managed-file-drift check in `cli.sh doctor`, so
the fix must land upstream and ship via the standard refresh. This downstream
repo will not patch it locally.

## Current Downstream Limitation

`bash .github/bubbles/scripts/cli.sh framework-validate` reports
`Framework validation failed with 1 failing check(s)` solely because of this
selftest. The failure is a **false positive** — the actual mcp-grant
materialization + reconcile behavior is correct and fully covered by the
passing T8/T9/T11/T12 cases — but it makes the aggregate framework-validate
gate red in every downstream repo with a non-default server-token slug
(smackerel, and by inference wanderaide/guesthost/quantitativefinance).

## Proposed Bubbles Change

Make the T2a and T6 expectations materialization-aware, the same way T11 already
is, instead of hardcoding the literal `bubbles`. Options (upstream's choice):

1. **Derive the expected token** in T2a/T6 from the same materialization helper
   the production path uses (preferred — keeps the selftest honest about the
   real behavior), e.g. build `EXPECT_GRANTED_LINE` from the materialized core
   token rather than a static string.
2. **Pin the selftest slug** to the framework default inside the hermetic
   `WORK` sandbox so `bubbles` does not materialize during T2a/T6, matching
   their static fixtures. (Simpler, but reduces fidelity vs. real downstream
   behavior.)

Option 1 is recommended because it validates the materialized-token path under
the same code real repos exercise, closing the gap that let T2a/T6 drift from
T8/T9/T11.

## Evidence

- Failing run captured under smackerel `framework-validate` on 2026-06-11
  (framework v7.7.0). 20/22 selftest cases pass; only T2a and T6 fail.
- Root cause introduced by the v7.6.0 → v7.7.0 server-token-materialization
  feature (commit `0bc04cfb` / `634e1c4a` in smackerel's vendored copy): the
  feature updated the materialization-aware cases but left the two static
  canonical-line fixtures (T2a/T6) on the pre-materialization literal.
- Smackerel spec 082 and all product code are unaffected — this is purely a
  framework selftest fixture gap.
