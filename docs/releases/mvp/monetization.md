# Monetization — Smackerel MVP

## Posture

**Smackerel MVP is pre-revenue and not monetized.** This is a deliberate stance, not a TBD.

The product is a self-hosted personal tool delivered as an OSS-style runtime under the [`LICENSE`](../../../LICENSE) in this repo. The operator runs it on their own infrastructure. Smackerel charges nothing, hosts nothing, and collects nothing — there is no Smackerel-side service to monetize at MVP.

## Pricing tiers (MVP)

None.

## Revenue model (MVP)

None.

## Customer acquisition assumptions (MVP)

None — there is no "customer" relationship at MVP. There are users who run the software on their own hardware. Acquisition is implicit via the channels listed in [`marketing.md`](marketing.md).

## Unit economics (MVP)

| Metric | MVP value |
|--------|-----------|
| Smackerel-side hosting cost per user | $0 (user hosts themselves) |
| Smackerel-side support cost per user | $0 (community only) |
| Smackerel-side LLM inference cost per user | $0 (user-provided; local Ollama default, optional cloud routed via `LLM_PROVIDER`) |
| Smackerel-side revenue per user | $0 |
| Gross margin | n/a (no revenue, no Smackerel-side variable cost) |

## Path-to-revenue timeline

Monetization is a **RELEASE-V1+ question**, gated on three capability arrivals:

1. **Outbound Action capability** (RELEASE-V1 Gap B) — until Smackerel can act, not just observe, the value proposition does not cross the threshold where users will pay.
2. **Personal Productivity Sources** (RELEASE-V1 Gap A) — Gmail SDK / Graph / Notion / Apple Reminders etc. broaden the ingestion surface beyond what self-hosting hobbyists tolerate, expanding addressable audience.
3. **Native-mobile decision** (RELEASE-V1) — without a mobile app, the daily-driver use case is constrained; commercial seriousness depends on the mobile outcome.

Until those three converge, MVP-stage monetization questions are deferred. This packet does NOT speculate on future pricing tiers — that would violate the agent's anti-fabrication discipline. Any pricing/revenue claims are RELEASE-V2+ territory and require operator direction at that time.

## Investor / capital signaling

[`docs/INVESTOR_OVERVIEW.md`](../../INVESTOR_OVERVIEW.md) exists as a reference document; it does NOT imply an active fundraise at MVP. Its purpose is to document the surfaced product direction for operator and any future advisory conversations. No external capital is sought at MVP.

## Honest framing

The right phrase for the MVP gate is **"working personal product, no revenue model, no obligation to develop one"** — not "freemium," not "pre-launch," not "early access." Smackerel may remain pre-revenue indefinitely if operator chooses. RELEASE-V1 enables — but does NOT commit to — a monetization pivot.
