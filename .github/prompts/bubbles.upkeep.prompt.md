---
agent: bubbles.upkeep
---

Run recurring operational upkeep for the due calendar task — scheduled backup verifications, restore drills, BCDR drills, patch cycles, and secret rotations from the per-repo upkeep calendar. Record each outcome in the append-only ledger, route failures to the right owner, and keep restore/BCDR drills in an isolated ephemeral namespace.
