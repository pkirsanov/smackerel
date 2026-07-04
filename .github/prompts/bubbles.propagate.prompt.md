---
agent: bubbles.propagate
---

Propagate changes across release trains — forward-merge fixes/features along declared train chains, backport under the approval guard, and audit propagation drift. Own propagation-policy.yaml + propagation-ledger.yaml, route the actual cherry-pick/merge execution to bubbles.devops, and record each decision in the append-only ledger.
