# Recipe: Multi-Train Status Rollup

**Persona:** DVS (`bubbles.train`).
**When to use:** You run ≥2 release trains and want a single view of where each one stands.

---

## Just tell super

```
> what's in prod and dev
> what's in prod
> release status
> all trains status
```

`bubbles.super` matches via `bubbles/intent-routes.yaml` and dispatches to `bubbles.train` with mode `release-train-status-all`.

---

## Direct invocation

```
/bubbles.workflow ship action:status scope:all-trains
/bubbles.train status --all-trains
```

Both produce a markdown table:

| TRAIN | PHASE | SLOT | FLAG_BUNDLE | RETENTION | PII | OPEN_FLAGS |
|-------|-------|------|-------------|-----------|-----|------------|
| experimental | active | none | config/feature-flags.experimental.yaml | 7d-daily | none | 3 |
| mvp | active | staging | config/feature-flags.mvp.yaml | 30d-daily | none | 7 |
| prod | active | prod | config/feature-flags.prod.yaml | 90d-monthly | encrypted-only | 12 |

Read-only. Never mutates state. No gates that can block.

---

## Underlying script

```
bash bubbles/scripts/release-train-rollup.sh [repo-root]
```

Reads `config/release-trains.yaml` plus `specs/*/state.json` (for the open-flag count). Exits 0 even when no trains file is present (informational message only).

---

## Quote

> *"Smoooth as silk, gentlemen. The train rolls on schedule."* — DVS
