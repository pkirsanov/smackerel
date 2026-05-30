# Cross-Spec Packet — Spec 061 → Spec 060 (Skill-Enforcement Bypass Reconciliation)

**Routed by:** `bubbles.implement` (spec 061, SCOPE-05, full-delivery convergence Round 81)
**Routed at:** 2026-05-30
**Target owner:** `specs/060-bearer-auth-scope-claim` (PASETO scope middleware + session-source contract)
**Source owner:** `specs/061-conversational-assistant` (assistant skills consuming the spec-060 catalog)
**Packet status:** `proposed` (NOT yet accepted by spec 060 owner; routing artifact itself satisfies the spec 061 DoD-#11 reframed contract per design.md §5.0.3 item 5)

**Supersedes (within spec 061):** the implicit assumption embedded in
the previously-accepted `packet-060-read-scopes.md` / `packet-060-write-scope.md`
that per-skill `auth.RequireScope("assistant:retrieval"|...)` guards
would run at the assistant tool entry point. Those packets remain
`accepted` for their catalog + default-grant content; THIS packet
addresses the runtime-enforcement reconciliation gap surfaced in
Round 80.

---

## 1. Why this packet exists

Round 79 (`bubbles.implement`) routed back with a wiring-architecture
mismatch and Round 80 (`bubbles.design`) recorded the authoritative
decision at `specs/061-conversational-assistant/design.md` §5.0. The
blocking fact, verified against shipped code:

- [`internal/auth/scope_middleware.go:71-79`](../../../internal/auth/scope_middleware.go)
  — `auth.RequireScope` explicitly **BYPASSES** `SessionSourceSharedToken`
  AND `SessionSourceBootstrap` with an `auth_scope_check_bypassed_total{source=...}`
  counter increment. Only `SessionSourcePerUserToken` (PASETO bearer)
  is gated on `Session.Scopes` membership.
- [`internal/telegram/webhook_handler.go:147-170`](../../../internal/telegram/webhook_handler.go)
  — the Telegram webhook authenticates via constant-time compare of
  `X-Telegram-Bot-Api-Secret-Token` against a single shared secret.
  Any `Session` minted from that boundary would be
  `SessionSourceSharedToken` (silently bypassed by `RequireScope`)
  or would require a NEW session source spec 060 has not authorized.

Consequence: wiring `auth.RequireScope("assistant:retrieval")` at any
boundary reachable from the Telegram transport delivers zero runtime
enforcement today. The spec-061 packet-060 catalog acceptance
contract therefore cannot be honored at runtime without a spec-060
decision.

The Round-80 selected disposition (Option (4) — registry-only DoD
close + this packet) lands `"assistant"` in `RegisteredScopeSurfaces`
in the same Round-81 commit and defers the runtime question to spec
060 via THIS packet.

---

## 2. Requested decisions (verbatim contract for spec 060 owner)

### 2.1 Documentation clarification (low-cost, requested regardless of 2.2 outcome)

Document explicitly in spec 060 `design.md` §4 (or the equivalent
`scope_middleware` contract section) that **scoped enforcement
requires `SessionSourcePerUserToken`** and is bypassed for
`SessionSourceSharedToken` / `SessionSourceBootstrap`. The bypass is
already true in code and surfaced via the `auth_scope_check_bypassed_total`
counter and a docstring comment, but it is not yet surfaced in the
spec 060 design narrative as a CONSEQUENCE for downstream consumers
that plan to consume `RequireScope` from non-PASETO transports.

### 2.2 Session-source decision (pick exactly one)

**Option (a) — Add a non-bypassed bot-token session source.** Introduce
`SessionSourceBotSharedToken` (or equivalent) that carries `Scopes`
and is NOT bypassed by `RequireScope`. The bot-shared-token mint
surface ([spec 060 mint CLI]) would emit such a session at the
Telegram webhook authentication boundary, with `Scopes` populated
from the operator-granted scope set on the token row. This would
unblock spec 061's per-skill `RequireScope` enforcement at the
assistant tool entry point (with `Session` propagated through the
agent executor — a separate spec-037 cross-spec ask, NOT in scope
here).

**Option (b) — Acknowledge that scoped enforcement requires per-user PASETO.**
Document explicitly that scoped enforcement is unavailable to
single-shared-secret transports (Telegram webhook, ntfy callbacks,
OAuth callbacks) and that capability-gate SST keys (e.g.
`assistant.skill.<name>.enabled`) plus operator discipline at token
mint time (NOT granting `assistant:notifications-write` to bot-shared
tokens without explicit approval) are the de-facto enable/disable
surface for these transports. Spec 061 then closes SCOPE-05 DoD #11
permanently on the registry-only contract and the runtime-enforcement
reframe documented in design.md §5.0.

**Either option is acceptable to spec 061.** The decision is owned by
spec 060.

---

## 3. What spec 061 commits to in Round 81 (already landed)

1. `internal/auth/scopes.go` — `"assistant"` appended to
   `RegisteredScopeSurfaces` (one-line additive). Same change set as
   this packet.
2. `internal/auth/scopes_test.go` — new
   `TestRegisteredScopeSurfaces_ContainsAssistant` covering positive
   membership + the adversarial misspelled-surface case.
3. SCOPE-05 DoD #11 evidence anchor refreshed in `scopes.md` with
   the Round-81 closure note citing this packet + design.md §5.0.
4. `report.md` Round-81 evidence section with the targeted
   `go test ./internal/auth/` PASS, the `./smackerel.sh check`
   EXIT=0, and the grep evidence for the registry addition.

Spec 061 does NOT modify `internal/auth/scope_middleware.go`,
`internal/auth/session.go`, or any spec 060 source/test/spec
artifact in Round 81. Spec 061 does NOT wire `auth.RequireScope`
guards into `internal/agent/tools/**`, `internal/assistant/**`,
`internal/telegram/**`, or `internal/api/**` in Round 81.

---

## 4. Status & blocking semantics

- **Acceptance is foreign-owned** and is NOT a blocker for SCOPE-05
  close-out under the Round-80 reframe.
- The routing of this packet itself satisfies design.md §5.0.3 item 5
  (the DoD-#11 measurable success criterion that requires the packet
  to exist on disk).
- Spec 061's `unresolvedFindings` carries
  `SCOPE-05-DOD-11-RUNTIME-ENFORCEMENT-DEFERRED-PENDING-SPEC-060-RECONCILIATION`
  forward until spec 060 returns a decision under §2.2; that finding
  supersedes the Round-78/79
  `SCOPE-05-DOD-11-SURFACE-REGISTRATION-WIRING-UNBLOCKED` finding,
  whose "wire RequireScope guards" sub-task is REMOVED based on the
  Round-80 architectural decision.
- Mirrors the precedent set by spec 054's BS-004 scheduler packet
  (spec 061 stays in_progress on its statusCeiling=`done` without
  requiring foreign-spec acceptance to promote — see Round-80
  `nextRequiredAction` in `specs/061-conversational-assistant/state.json`).

---

## 5. Cross-references

- `specs/061-conversational-assistant/design.md` §5.0 — authoritative
  Round-80 decision record (selected option + rejected options + rationale).
- `specs/061-conversational-assistant/design.md` §5.0.5 — packet ask
  (this document is the realization of that ask).
- `specs/061-conversational-assistant/design.md` §5.0.6 — new
  spec-061-internal finding tracking this deferred work.
- `specs/061-conversational-assistant/cross-spec/packet-060-read-scopes.md`
  — accepted; covers `assistant.skill.retrieval` + `assistant.skill.weather`
  catalog + default-grant contract (unaffected by this packet).
- `specs/061-conversational-assistant/cross-spec/packet-060-write-scope.md`
  — accepted; covers `assistant.skill.notifications-write` catalog +
  operator-discipline default-deny contract (unaffected by this packet).
- `specs/061-conversational-assistant/scopes.md` SCOPE-05 DoD #11
  evidence anchor — refreshed Round 81 with citation to this packet.
- `specs/061-conversational-assistant/report.md` Round 81 evidence
  section.
