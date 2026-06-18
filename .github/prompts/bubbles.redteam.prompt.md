---
agent: bubbles.redteam
description: Attack a finished result to falsify the "done" claim, run risk-gated voting validators, or run a bounded chaos-monkey probe against a live system. Off by default.
---

Route to the `bubbles.redteam` agent when the user wants the finished result attacked rather than checklisted — counterexamples that falsify a "done" claim, N independent validators voting on a high-risk change, or a bounded, armed chaos-monkey probe against a live/production system.

The capability is OFF BY DEFAULT. It resolves through `bubbles/scripts/adversarial-resolve.sh` (precedence: per-run directive → `BUBBLES_ADVERSARIAL*` env → `.github/bubbles-project.yaml` `adversarial:` block → framework default `off`). `bubbles.redteam` emits findings; it NEVER certifies — completion authority stays with `bubbles.validate`.

## Natural Language Triggers

- "red-team this" / "attack the result" / "try to break it"
- "prove it's actually done"
- "get 3 validators to vote on this"
- "chaos-monkey the prod system" / "this is my park now"
- "is this actually bulletproof"

## Example Outcomes

User: `red-team this scope before we certify`
- Mode 1 post-result falsification; route any counterexample through `bubbles.bug` → `bubbles.implement` → `bubbles.test` → `bubbles.validate`.

User: `get 3 validators on the payment change`
- Mode 2 voting ensemble (`passes: 3`), riskClass-gated; escalate on disagreement.

User: `chaos-monkey the live system`
- Mode 3 `production-adversarial-probe` — refuse unless armed + target allowlisted; bounded, read-only, restore-or-fix.
