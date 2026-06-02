# Recipe: Production Incident Response (Incident Fastlane)

**Persona:** Shitty Bill diagnoses (`bubbles.stabilize`), DVS rolls back (`bubbles.train`), Tommy Bean executes (`bubbles.devops`).
**When to use:** Production is broken or actively degrading. Speed matters but ownership boundaries still hold.

---

## Just tell super

```
> prod is broken
> rollback prod
> production incident
> we need to rollback
```

`bubbles.super` matches via `bubbles/intent-routes.yaml` and dispatches `bubbles.stabilize` under workflow mode `incident-fastlane`.

---

## Direct invocation

```
/bubbles.workflow incident-fastlane
```

---

## The chain (mode `incident-fastlane`)

```
1. bubbles.stabilize       diagnose + classify severity
                           (G124: each finding tagged incident|high|medium|low)
        │
        │  if any finding severity=incident:
        ▼
2. bubbles.train           pointer-swap rollback to last-good slot
                           (existing DVS authority — stabilize NEVER rolls back inline)
        │
        ▼
3. bubbles.devops          execute redeploy via repo CLI
        │
        ▼
4. bubbles.validate        confirm rollback target is live
        │
        ▼
5. bubbles.docs            post-incident notes (optional)
```

Status ceiling: `incident_mitigated`.

---

## Why the boundaries hold even in an incident

Speed is not an excuse for skipping ownership. If stabilize rolled back inline, you'd lose:

- The audit trail in the train's manifest commit history.
- Tommy's repo-CLI invocation (which is what actually mutates production).
- Validate's confirmation step (which catches the rollback-didn't-take case).

The fastlane gets you all three in one operator command without inventing a new flow.

---

## Severity tags (G124)

| Tag | When | What stabilize does |
|-----|------|--------------------|
| `incident` | User-facing degradation in prod right now | Emit `route_required` → bubbles.train rollback |
| `high` | Stability risk, not user-visible yet | Route to bubbles.implement / bubbles.devops normally |
| `medium` | Latent risk, schedule next cycle | Record in observations |
| `low` | Cosmetic | Optional |

---

## Gate reference

| Gate | Enforces |
|------|----------|
| G124 | At least one severity tag declared per finding when running `incident-fastlane` |
| G117 | Manifest commits + ledger entries from the rollback are append-only |
| G110 | The rollback target slot still references a declared train |

---

## Quote

> *"This is good. This is real good. You're gonna want to write this down, boys."* — Mr. Lahey
