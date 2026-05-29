# Open Items — handoff snapshot 2026-05-29 (REFRESHED end-of-session)

Captured at end-of-session after all autonomously-resolvable items were closed.
Update or delete this file when remaining items close out.

## Session totals

Commits shipped this session (in order):
- `b0fec4c5` — revert fabricated spec 063 `specs_hardened` promotion (Gate G021)
- `0c5b6249` — plan + B6 fixups for state-transition-guard on spec 063
- `2886d516` — feat(061): SCOPE-09a + SCOPE-09b OTel substrate + span tree (29 files, +2911)
- `8cf22037` — docs(memory): initial open-items snapshot
- `4e442c1f` — spec(054): accept cross-spec packet from 061 SCOPE-08 (artifact-only)
- `1c054903` — spec(060): accept packet-060-read-scopes + packet-060-write-scope (artifact-only)
- (this commit) — refresh open-items memo + framework-issue draft + drop obsolete stash

All pushed to `origin/main`. Pre-commit pii-scan + gitleaks clean on every commit. No `--no-verify` used.

---

## RESOLVED this session

### Q1 — Cross-spec packets ✓

All three packets formally accepted:
- `packet-054-scheduler.md` (commit `4e442c1f`) — additive `Job.Source`/`Job.Originator` contract ratified on spec 054 side. **Deferred:** Go type introduction + migration + tests, routed to a dedicated spec 054 follow-up scope (no public `Job` struct exists in `internal/scheduler/` today).
- `packet-060-read-scopes.md` + `packet-060-write-scope.md` (commit `1c054903`) — three new auth-scope contracts accepted with name translation: `assistant.skill.retrieval` → `assistant:retrieval`, `assistant.skill.weather` → `assistant:weather`, `assistant.skill.notifications.write` → `assistant:notifications-write`. **Deferred:** surface-registry addition + `RequireScope` wiring lands with the introducing spec (spec 061), per spec 060's documented "surface registration lands with introducing spec" pattern.

**Downstream impact:** Spec 061 SCOPE-05 DoD #11 is now formally satisfied. SCOPE-06/07/08 e2e remain gated on the deferred code wiring (which is spec 061's responsibility to land, not 054/060's).

### Q3 — Obsolete stash@{0} ✓

Dropped (`3c0191fd6123dae949c86431d8a8d35449d7c845`). It added a parallel `internal/observability/otel/` package + `wireOTel` wrapper that have been superseded by `internal/assistant/tracing/` + `cmd/core/wiring.go` (commit `2886d516`). Also referenced spec 062 which is no longer on disk.

### Q5 — Spec 058 "B2-minimal" ✓ (resolved-as-stale)

Repo-wide grep finds zero references. Spec 058 is `done_with_concerns`, certified, `reworkQueue: []`, `concerns: null`. Treating as chat-only context that resolved with consolidation. **If real work remains, operator must re-articulate it** with concrete description.

---

## DEFERRED — needs operator action or large-scope dispatch

### Q2 — Spec 063 framework guard mismatches

**Long-term fix drafted:** [`framework-issue-state-guard-planning-mode.md`](framework-issue-state-guard-planning-mode.md) contains a ready-to-file issue body covering G027/G041/G022/G040 mismatches. Operator action: file against the `bubbles/` framework repo when convenient.

**Short-term unblock for spec 063:** when ready to implement, dispatch `bubbles.workflow mode=full-delivery specs=specs/063-knowledge-ai-enrichment`. Full-delivery has no `specs_hardened` ceiling — implementation phases land scopes Done naturally and the broken guard logic doesn't apply. Do NOT flip `workflowMode` in state.json manually; let the workflow dispatch handle the upgrade.

Spec 063 current state (unchanged this session, confirmed correct):
- `status: in_progress`
- `workflowMode: product-to-planning`
- `statusCeiling: specs_hardened`
- `certifiedCompletedPhases: [analyze, ux, design, plan]` (legitimate, post-revert)
- `certifiedAt: null` (correctly reverted)
- 13 scopes planned, 10 OQs resolved, 3 cross-spec packets routed AND now accepted ✓ (this session)

### Q4 — Spec 062 (forward-looking-intelligence)

Confirmed absent: not on disk, not in any stash, not in `git log --all -- 'specs/062*'`. If forward-looking-intelligence work is to resume, it must be re-planned from scratch via `bubbles.workflow mode=full-delivery` or `mode=product-to-planning`. Otherwise treat as dead.

### Spec 061 — remaining scopes

| Scope | Status | Notes |
|-------|--------|-------|
| SCOPE-01..04 | Done | Foundation + capability facade |
| SCOPE-05 | In Progress | Telegram adapter v1; **DoD #11 now satisfied** by packet-060-read acceptance. Other DoD items still open. |
| SCOPE-06 | In Progress | Retrieval Q&A e2e; needs spec 061-side `RequireScope("assistant:retrieval")` wiring + BS-002 e2e completion (BS-002 test already drafted by operator at `tests/e2e/assistant_bs002_test.sh`) |
| SCOPE-07 | In Progress | Weather e2e; needs spec 061-side `RequireScope("assistant:weather")` wiring + BS-003/BS-006 e2e |
| SCOPE-08 | In Progress | Notifications confirm flow; needs spec 061-side `RequireScope("assistant:notifications-write")` wiring (default-deny enforced) + spec 054 follow-up scope for scheduler binding + BS-004 e2e |
| SCOPE-09a/09b | Done | OTel substrate + span tree (this session) |
| SCOPE-10 | Not Started | Evaluation harness; operator authored §3.8.5 + Round 47 evidence in commit `dde5272c` (parallel work) |

**Execution-cursor drift (Q6):** `state.json.execution.currentScope == "SCOPE-10"` while SCOPE-05/06/07/08 remain In Progress. Benign — the in-progress scopes are externally-blocked and the cursor advanced to the next available scope when 09b finished. Not worth a state.json comment; self-explanatory from the scope matrix.

### Q7 — Operator parallel work

Untouched. As of memo-write time the operator had WIP on `internal/telegram/assistant_adapter/` + spec 061 report.md + two new e2e test files (`tests/e2e/assistant_bs002_test.sh`, `tests/e2e/assistant_bs007_test.sh`). These are SCOPE-06 acceptance work the operator is driving directly.

---

## Next-session recommended single move

The two highest-leverage items left:
1. **Spec 061 SCOPE-06/07/08 code wiring** to consume the now-accepted auth scopes (add `"assistant"` to `RegisteredScopeSurfaces` + `RequireScope` middleware on the three tool handler routes + adversarial regression for the write-scope default-deny). This is small (one-line surface registration + 3 middleware calls + ~3 tests) and unblocks BS-002/003/004/006/007/010 e2e completion.
2. **File framework issue** from `framework-issue-state-guard-planning-mode.md` against `bubbles/`.

Item 1 is product-repo work and within `bubbles.workflow mode=bugfix-fastlane` reach for spec 061. Item 2 is operator-only (framework repo access required).
