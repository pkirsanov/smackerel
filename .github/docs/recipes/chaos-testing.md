# Recipe: Chaos Testing

> *"Worst case Ontario... nothing broke."*

---

## The Situation

You need to find out what happens when things go wrong — load spikes, malformed inputs, dependency failures, concurrent access.

## The Command

```
/bubbles.chaos  run chaos scenarios for 042-catalog-assistant
```

Or with hardening follow-up:

```
/bubbles.workflow  chaos-hardening for 042-catalog-assistant
```

**Phases:** chaos → harden → test → validate

## What Chaos Tests

- Resource exhaustion (memory, connections, disk)
- Concurrent access / race conditions
- Malformed inputs and boundary values
- Dependency failures (DB down, API timeout)
- Large payloads and edge-case data

## After Chaos

If chaos finds issues, the `harden` phase fixes them:

```
/bubbles.harden  fix issues found by chaos testing
```

Then re-run to confirm:

```
/bubbles.chaos  verify chaos fixes
```
