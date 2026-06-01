# Route Packet Response — PKT-061-A — Provenance Gate Source Taxonomy + Canonical Refusal Taxonomy Amendment

| Field            | Value |
|------------------|-------|
| **Packet ID**    | PKT-061-A |
| **Original request** | [`PKT-061-A.md`](PKT-061-A.md) |
| **Routed from**  | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-10 |
| **Routed to**    | `specs/061-conversational-assistant` |
| **Status**       | `closed` |
| **Merged at**    | 2026-05-31T00:00:00Z |
| **Closed by**    | `bubbles.workflow mode=bugfix-fastlane` against spec 061 |
| **Unblocks**     | spec 064 SCOPE-13 (Telegram surface) |

---

## Disposition

**Accepted in full.** Spec 061 status remains `done` per the
framework's amend-in-place rule for small additive changes within a
certified ceiling. No ceiling bump required.

---

## Acceptance criteria (PKT-061-A §3) — evidence

1. **`SourceWeb` and `SourceToolComputation` pass through the gate** —
   `TestEnforce_PassthroughWithWebSource`,
   `TestEnforce_PassthroughWithToolComputationSource`,
   `TestEnforce_PassthroughWithMixedKinds` in
   `internal/assistant/provenance/gate_test.go`.
2. **`CanonicalRefusalBody` extended with 5 cause-specific bodies** —
   implemented in `internal/assistant/contracts/refusal.go` as the
   `RefusalCause` enum + `CanonicalRefusalBodyFor` function; exact
   packet §3.B body strings asserted by
   `TestEnforceRefusal_EachCauseHasExactBody`.
3. **Existing `SourceArtifact` behaviour unchanged (adversarial)** —
   every pre-amendment gate test in `gate_test.go` runs green with
   zero modifications; explicit backcompat assertion in
   `TestEnforce_PreExistingArtifactBehaviourUnchanged`.
4. **`specs/061-conversational-assistant/report.md` entry** — added
   under "Amendment — PKT-061-A (2026-05-31): Source taxonomy +
   canonical refusal taxonomy extension".
5. **Status remains `done`** — no `state.json` mutation required;
   amendment is additive within the certified ceiling. The contracts
   package already owned the canonical `SourceKind` enum, so the
   packet's optional contracts-ownership note (§1 constraints) is
   satisfied by extending that enum rather than mirroring the
   openknowledge taxonomy.

---

## Files modified (spec 061 territory)

- `internal/assistant/contracts/source.go` — taxonomy + ref types
- `internal/assistant/contracts/refusal.go` (new) — refusal taxonomy
- `internal/assistant/contracts/response_test.go` — extended exhaustive + golden coverage
- `internal/assistant/contracts/testdata/golden/thinking_web_source.json` (new)
- `internal/assistant/contracts/testdata/golden/thinking_tool_computation_source.json` (new)
- `internal/assistant/provenance/gate.go` — Kind validation + `EnforceRefusal`
- `internal/assistant/provenance/gate_test.go` — pass-through + adversarial + refusal-body tests
- `internal/telegram/assistant_adapter/render_sources.go` — explicit render branches for the two new kinds
- `specs/061-conversational-assistant/report.md` — amendment entry

## Files modified (spec 064 territory)

- `specs/064-open-ended-knowledge-agent/route-packets/PKT-061-A-RESPONSE.md` (this file)
- `specs/064-open-ended-knowledge-agent/scopes.md` — SCOPE-10 → Done
- `specs/064-open-ended-knowledge-agent/state.json` — `transitionRequests[0].status` → `closed`, `reworkQueue[0]` resolved

**Not touched (per packet constraints):** `internal/assistant/openknowledge/*`.

---

## Verification commands

```text
$ go test ./internal/assistant/contracts/... \
         ./internal/assistant/provenance/... \
         ./internal/telegram/assistant_adapter/...
ok      github.com/smackerel/smackerel/internal/assistant/contracts     0.059s
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.017s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      0.020s
```

Full-suite verification (`./smackerel.sh test unit --go`,
`./smackerel.sh lint`) blocked by a pre-existing duplicate
`package web` declaration in
`internal/assistant/openknowledge/web/provider_test.go` (untracked
spec 064 WIP). The duplicate is unrelated to this packet's diff and
lives in the openknowledge/* layer the packet forbids touching;
surfaced as finding in the closing RESULT-ENVELOPE.

---

## Next action

Spec 064 SCOPE-13 (Telegram surface) is unblocked. SCOPE-10 closes
with this packet; SCOPE-11/12/14/15/16/17/18 proceed independently per
the dependency map declared in PKT-061-A §4.
