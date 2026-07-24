# User Validation: 107 Proactive & Correlated Experience

## Checklist

- [x] Planning-fidelity baseline: the packet preserves the specification's single-controller origin, observe-first, honest-state, cross-frontend-parity, compose-over-105/106, and no-second-store contracts; this is not runtime acceptance.
- [x] Dependency baseline: `specs/078-*` (controller), `specs/106-*` (shell), `specs/105-*` (explorer), `specs/072-*` (WhatsApp), and `specs/074-*`/`061-*`/`073-*` (capture/turn) are recorded as entry-gate dependencies and are not claimed complete or modified by this packet.
- [x] Test-integrity baseline: every SCN-107 scenario has concrete unit, integration, E2E API, and E2E UI planning with no mocked live-stack path; all tests are PLANNED / not-yet-authored.
- [x] SST no-default baseline: `nudge_ref_ttl_hours=6`, `RAIL_MAX=6`, `what_changed_page_cap=25`, and the MVP snooze-reuses-`suppression_window_hours` decision are recorded as fail-loud keys reserved for implementation, edited in no config this phase.

## Goal

Every frontend leads with what Smackerel already found, connected, and decided:
an authenticated user lands on a Today cockpit fusing the digest, the day's
controller-permitted proactive cards, a what-changed summary, and an
ask-or-capture bar; sees an always-on correlation rail of real edges that
deep-links into the graph explorer; can ask-or-capture from one input anywhere;
and the same nudge and action render channel-appropriately on web, Telegram, and
WhatsApp with one honest budget, dedupe, and suppression truth across all of them.

## Acceptance Journeys

| Journey | Planned Evidence | Runtime Status |
|---|---|---|
| Land on the Today cockpit and act on a real card | SCN-107-001, 003, 004 | Not executed |
| Distinguish a quiet day, budget exhaustion, and a producer failure honestly | SCN-107-002, 008, 017 | Not executed |
| Act on a nudge from Telegram and WhatsApp with cross-channel suppression | SCN-107-005, 006, 007 | Not executed |
| See real correlations and enter the spec-105 explorer; honest no-related | SCN-107-010, 011 | Not executed |
| Ask or capture from the command palette; failed ask is never saved-as-idea | SCN-107-012, 013 | Not executed |
| See what changed without backlog guilt | SCN-107-014, 015 | Not executed |
| Every card originates from the one controller | SCN-107-016 | Not executed |
| Complete the proactive surface by keyboard, screen reader, and on mobile, with authorization respected | SCN-107-018, 019, 020 | Not executed |

## Planning Boundary

Checked items above validate packet fidelity only. They do not validate source,
tests, browser behavior, migration, deployment, or operated data. Runtime
acceptance is owned by `bubbles.implement`/`bubbles.test`/`bubbles.validate` at
pickup.
