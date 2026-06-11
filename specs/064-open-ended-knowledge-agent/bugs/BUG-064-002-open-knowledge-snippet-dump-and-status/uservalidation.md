# BUG-064-002 — User Validation

These items default to CHECKED `[x]` (validated by the bug's adversarial
regression tests in this session). The operator unchecks an item to report it as
still broken on the live home-lab stack after redeploy.

## Checklist

- [x] An open-ended NL question (e.g. the tide example) returns ONE synthesized answer, not a raw web-search snippet dump (DEFECT 1) — validated by T2.
- [x] The answer body does not repeat the same block 2+ times (DEFECT 2) — validated by T1/T2.
- [x] A delivered open-knowledge answer shows NO "thinking…" header; the status is terminal (DEFECT 3a) — validated by T3/T3b/T6.
- [x] The citation list is deduplicated and capped to the sources actually used (DEFECT 3b) — validated by T4.

> NOTE: these are validated in-repo against the un-redacted assembled body via Go
> unit/integration tests. The LIVE home-lab confirmation (real Telegram + searxng
> + GPU) requires the redeploy owned by `bubbles.devops`; after redeploy, re-send
> the tide prompt and confirm a single synthesized answer (highs/lows + ft where
> the model can extract them), no `thinking…` header, no triplicate, and a short
> capped source list.
