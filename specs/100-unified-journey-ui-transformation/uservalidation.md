# User Validation — Spec 100 (Unified Journey UI Transformation)

**Spec:** [spec.md](spec.md) · **Design:** [design.md](design.md) · **Scopes:** [scopes.md](scopes.md) · **Evidence:** [report.md](report.md)

This document records the user-facing acceptance checks a human operator can run
to confirm the nine findings are closed. It is filled with real evidence during
the validate phase.

---

## How to verify (operator steps)

All commands run from the repo root via the repo CLI (Docker-only).

```bash
./smackerel.sh check
./smackerel.sh lint
./smackerel.sh test unit
./smackerel.sh test e2e-ui
```

## Acceptance walkthrough (maps to the nine findings)

| # | What to check | Finding(s) | Expected |
|---|---------------|-----------|----------|
| 1 | Open `/` (signed in) → the top nav shows **Assistant · Search · Knowledge · Cards · Notifications · Settings**; "Assistant" → `/assistant`. | SR-03, SR-01, SR-11 | Assistant is one click away from the knowledge surface; `/` leads with an intent-first assistant entry. |
| 2 | Open `/cards` → the same app-shell nav is present above the card nav. | SR-03, SR-01 | Cards is no longer an island. |
| 3 | Open `/notifications` → it renders under the shared shell nav. | SR-07 | Notifications share the app-shell chrome. |
| 4 | Sign in from `/login` with no explicit destination. | SR-05 | You land on the assistant (`/assistant` → `/pwa/assistant.html`), not a blank keyword box. |
| 5 | Sign in from a deep link (`/login?next=/cards`). | SR-05 (regression) | You still land on `/cards`; hostile `next` still lands on `/`. |
| 6 | Install the PWA → the home screen shows **shortcuts** (Assistant, Capture, Search); the PWA home cross-links every feature. | SR-01, SR-13 | The PWA is navigable as one app. |
| 7 | On the PWA, there is no "paste Auth Token" landing; capture/share and photo actions work using your signed-in session. | SR-04 | One auth model (cookie); no bearer token in `localStorage`. |
| 8 | Share a URL to the PWA → the confirmation names what was saved and says it is saved and searchable, with a next action. | SR-08 | Trustworthy durable-capture ACK (P8). |
| 9 | Open the invites admin → it is at `/admin/invites` (product-level), not under `/cards`. | SR-06 | Admin is product-level; one-time token still shown once. |

## Evidence

_Filled during validate. See [report.md](report.md) for the raw command output
(≥10 lines per DoD item), the per-finding closure table, and the
`bubbles.validate` certification verdict._

## Checklist

- [x] Planning artifacts (spec/design/scopes/state/report/uservalidation) created and artifact-lint clean
- [ ] Step 1 — shared app-shell nav on the knowledge surface (SR-03/SR-01/SR-11)
- [ ] Step 2 — shared app-shell nav on `/cards` (SR-03/SR-01)
- [ ] Step 3 — notifications under the shared shell (SR-07)
- [ ] Step 4 — assistant is the default post-login landing (SR-05)
- [ ] Step 5 — explicit/hostile `next` regression preserved (SR-05)
- [ ] Step 6 — PWA shortcuts + cross-linked feature pages (SR-01/SR-13)
- [ ] Step 7 — one cookie auth model; no pasted-token landing (SR-04)
- [ ] Step 8 — strengthened durable-capture ACK (SR-08)
- [ ] Step 9 — invites admin at `/admin/invites` (SR-06)

## Sign-off

- [ ] Operator confirms the nine findings are closed (steps 1–9 above).
- [ ] No regression in the certified auth hardening (091/093/070/044/060) or the
      capture-as-fallback inviolable path (074).
