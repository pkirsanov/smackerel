# Open Items — handoff snapshot 2026-05-29

Captured at end-of-session after commits `b0fec4c5`, `0c5b6249`, `2886d516` shipped to `origin/main`. Update or delete this file when items close out; do NOT let it rot.

## Cross-spec packets awaiting foreign-owner acceptance

| Packet | Target spec | Current target status | Blocks |
|--------|-------------|----------------------|--------|
| [`packet-054-scheduler.md`](../../specs/061-conversational-assistant/cross-spec/packet-054-scheduler.md) | spec 054 notification-intelligence-handler | `done` (no statusCeiling) | spec 061 SCOPE-08 BS-004 e2e |
| [`packet-060-read-scopes.md`](../../specs/061-conversational-assistant/cross-spec/packet-060-read-scopes.md) | spec 060 bearer-auth-scope-claim | `done` | spec 061 SCOPE-05 DoD #11, SCOPE-06/07 e2e |
| [`packet-060-write-scope.md`](../../specs/061-conversational-assistant/cross-spec/packet-060-write-scope.md) | spec 060 bearer-auth-scope-claim | `done` | spec 061 SCOPE-08 BS-004 e2e |

Both target specs are `done` but neither has accepted the inbound packets (zero references in their artifacts). Foreign-owned — `bubbles.goal` cannot acknowledge for them. Next move: operator dispatch to spec 054/060 owners or operator approval to author additive entries on their behalf.

## Spec 063 — `product-to-planning` mode-vs-guard mismatches

Spec 063 sits at `status: in_progress`, planning-only, awaiting promotion to `specs_hardened`. The state-transition-guard rejects promotion for reasons that are framework-level (not spec-defect) per investigation in this session:

- **G027** hardcodes `fail "ALL scopes must be Done"` whenever `not_started_scopes > 0`. This is `done`-promotion logic applied to `specs_hardened` ceiling. Spec 063 has 13 intentionally Not-Started scopes (planning packet awaiting implementation).
- **G041** status-line parser doesn't handle the `**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** None` pipe-separated metadata convention used across the repo.
- **G022** phase-provenance markers — no repo precedent for how to stamp these on a planning-only spec.
- **G040** "Deferral language" false-positives on legitimately routed cross-spec packets.

Verdict: file framework issue against the `bubbles/` repo OR pivot to `full-delivery` mode when implementation begins. Do NOT attempt mechanical fixups to force the gate — anti-fabrication discipline (Gate G021) was already violated once on this spec.

## Spec 058 — stable, no open work

`status: done_with_concerns`, `certifiedAt: 2026-05-28T15:40:00Z`, ceiling=`done`, `reworkQueue: []`, `concerns: null`. The "B2-minimal" item referenced in the original handoff has no anchor in the repo and appears to be chat-only context. If real work remains, the operator must re-articulate it.

## Spec 062 — absent from disk and history

Confirmed missing from `specs/`, all stashes, all branches, and git log. If forward-looking-intelligence planning is to resume, it must be re-planned from scratch via `runSubagent(bubbles.workflow): mode=full-delivery`.

## stash@{0} — likely obsolete

`stash@{0}: On main: operator-WIP-20260529: 46+ files across specs 058/061/062 + assistant/telegram/intelligence/config`. Inspection shows it adds a parallel `internal/observability/otel/` package and `wireOTel` wrapper that have been **superseded** by today's `internal/assistant/tracing/` + `cmd/core/wiring.go` substrate (commit `2886d516`). The stash references a spec 062 surface that is no longer on disk.

Recommended disposition: drop after operator confirmation. Do NOT drop autonomously — 6342 lines is large enough that destructive removal needs a human nod.

## Spec 061 scope status

| Scope | Status | Notes |
|-------|--------|-------|
| SCOPE-01..04 | Done | Foundation + capability facade |
| SCOPE-05 | In Progress | Telegram adapter v1; blocked by packet-060-read |
| SCOPE-06 | In Progress | Retrieval Q&A e2e; blocked by packet-060-read |
| SCOPE-07 | In Progress | Weather e2e; blocked by packet-060-read |
| SCOPE-08 | In Progress | Notifications confirm flow; blocked by packet-054 + packet-060-write |
| SCOPE-09a/09b | Done | OTel substrate + span tree (this session) |
| SCOPE-10 | Not Started | Evaluation harness; depends on 05..09 |

`state.json.execution.currentScope` reads `SCOPE-10` but SCOPE-05..08 remain In Progress — the cursor jumped past unfinished scopes when 09b finished. Benign but worth knowing.

## Recently shipped (this session)

- `b0fec4c5` revert of fabricated spec 063 `specs_hardened` promotion (Gate G021)
- `0c5b6249` plan + B6 fixups for state-transition-guard on spec 063
- `2886d516` SCOPE-09a + SCOPE-09b OTel substrate + span tree (29 files, +2911 lines)

All three on `origin/main`.
