# Spec: BUG-007-003 — Surface gkeepapi schema drift via warnings

> **Parent spec:** [007 spec](../../spec.md)
> **Bug:** [bug.md](bug.md)

## Expected Behavior

`ml/app/keep_bridge.py::serialize_note` MUST log a `WARNING` whenever an exception is caught from `gkeepapi` attribute access, while preserving the existing resilient fallback (empty list / zero timestamp) so the sync does not abort.

### Behavioral Contract

| Failure point | Fallback (unchanged) | New observability requirement |
|---|---|---|
| `gnote.labels.all()` raises | `labels = []` | exactly one `logger.warning` with context `labels` + exception type + message |
| `gnote.collaborators.all()` raises | `collaborators = []` | exactly one `logger.warning` with context `collaborators` + exception type + message |
| `gnote.items` iteration raises | `list_items = []` | exactly one `logger.warning` with context `list_items` + exception type + message |
| `timestamps.updated` raises | `modified_usec = 0` (left over from initialization) | exactly one `logger.warning` with context `timestamps.updated` + exception type + message |
| `timestamps.created` raises | `created_usec = 0` (left over from initialization) | exactly one `logger.warning` with context `timestamps.created` + exception type + message |

### Invariants

- `serialize_note` MUST NOT raise. All exceptions from `gkeepapi` attribute access remain locally absorbed.
- Fallback dict shape is unchanged.
- No metric or alert is introduced in this bug (out of scope — observability is delivered via `logger.warning` only).
- The log level MUST be `WARNING` (not `INFO`, not `ERROR`). `WARNING` matches Smackerel's convention for "external API surface drift; degraded data but not a fatal failure".

## Acceptance Criteria

(Mirror of bug.md acceptance criteria; canonical list lives there.)

## Out of Scope

- Restructuring `serialize_note` into per-field helpers.
- Adding Prometheus counters / NATS metrics for schema drift (would be a follow-up observability feature).
- Replacing `gkeepapi` with the official Takeout path (already covered by other scopes in spec 007).
- Changing the timestamp `0` fallback semantics (cursor poisoning is documented but out of scope for this bug — it requires a separate design decision).
