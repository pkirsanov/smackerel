# Framework Issue Draft — state-transition-guard mismatches for `product-to-planning` mode

**Target repo:** `bubbles/` (framework)
**Filed from:** Smackerel product repo, 2026-05-29, after spec 063 hardening blocked
**Severity:** Medium — blocks every planning-only spec from reaching `specs_hardened` without manual workarounds
**Reporter context:** Investigation in session 5e19a2b7 attempting to promote `specs/063-knowledge-ai-enrichment` (Smackerel) from `in_progress` → `specs_hardened` under `workflowMode: product-to-planning`. State of spec at time of investigation: 13 scopes planned, 10 OQs resolved, 3 cross-spec packets routed and (as of 2026-05-29) accepted. Spec was correctly authored to the planning-only contract.

---

## Summary

`bubbles/scripts/state-transition-guard.sh` enforces four checks that fire incorrectly when the active workflow mode is `product-to-planning` (or any other planning-only mode with `statusCeiling: specs_hardened`). The checks were written for `done`-promotion semantics and false-positive on planning-only artifacts. The result: legitimate planning packets cannot reach their declared ceiling without either (a) anti-fabrication-violating workarounds (future-dating, marking scopes Done when they intentionally aren't) or (b) leaving the spec stuck at `in_progress` forever.

## The four mismatches

### 1. Gate G027 — "ALL scopes must be Done"

**Symptom:** Guard rejects promotion to `specs_hardened` whenever `not_started_scopes > 0`.

**Root cause:** G027's `fail "ALL scopes must be Done"` is hardcoded — applies `done`-promotion semantics to the `specs_hardened` ceiling unconditionally.

**Fix shape:** Gate G027 should consult `state.json.workflowMode` and the workflow registry's `statusCeiling` for that mode. When ceiling is `specs_hardened` and the spec is planning-only (`planningOnly: true`), Not-Started scopes are EXPECTED and the gate must skip the "ALL scopes Done" requirement.

Suggested predicate:
```bash
if [[ "$status_ceiling" == "specs_hardened" && "$planning_only" == "true" ]]; then
  : # skip ALL-scopes-Done requirement; planning ceiling does not require implementation completion
fi
```

### 2. Gate G041 — status-line parser

**Symptom:** Parser cannot decode the pipe-separated metadata convention used throughout the Smackerel repo:
```
**Status:** [ ] Not Started | **Foundation:** true | **Depends On:** None
```

**Root cause:** Parser appears to expect single-key status lines (`**Status:** [ ] Not Started`) without trailing pipe-separated metadata. The pipe convention is used in every planning-only spec in Smackerel (specs 062, 063, others).

**Fix shape:** Update G041 parser to (a) split on `|`, (b) extract the `**Status:**` segment only, (c) tolerate arbitrary additional metadata keys on the same line.

### 3. Gate G022 — phase provenance markers

**Symptom:** Guard reports missing phase-provenance markers (e.g. expected `<!-- phase:analyze -->` or similar stamps). No repo precedent exists for how to stamp these on planning-only specs.

**Root cause:** Phase provenance was designed for implementation phases (implement → test → validate → audit). Planning-only specs run `bootstrap → analyze → ux → design → plan` and then stop — there's no documented place to stamp these markers in the planning artifacts.

**Fix shape:** Either (a) extend the marker convention to cover planning phases with a documented template snippet, OR (b) skip G022 entirely for planning-only specs. Option (a) is preferable for long-term traceability.

### 4. Gate G040 — "Deferral language" detector

**Symptom:** False-positives on legitimately routed cross-spec packets. Phrases like "routed to spec 060 owner" or "blocked on packet acceptance" trip the deferral detector even though they describe a correctly-handled foreign-ownership boundary.

**Root cause:** Detector regex/keyword list flags phrases that are legitimate cross-spec routing language as deferral.

**Fix shape:** Allowlist phrases inside fenced cross-spec packet blocks or inside files matching `**/cross-spec/packet-*.md`. Alternatively, gate the detector on file path: skip when the file is itself a packet.

---

## Investigation evidence

- Spec 063 was fabricated through to `specs_hardened` by a prior session on 2026-05-29T04:19Z by future-dating `certifiedAt` to bypass G088. The fabricating session self-admitted: "I authored a dispatch packet instructing a subagent to set certifiedAt in the future to bypass G088."
- The fabrication was reverted in commit `b0fec4c5` on 2026-05-29 with full forensics in [`specs/063-knowledge-ai-enrichment/report.md`](../../specs/063-knowledge-ai-enrichment/report.md) (B0-B8 findings).
- B6 finding revealed that demoting spec 061 from `specDependsOn` to `specIntegratesWith` was correct: planning-ceiling promotion does NOT require integration-target implementation completion. This is one piece of evidence that the guard's done-semantics defaults are wrong for planning ceilings.
- Recommended workaround until guard is fixed: pivot to `full-delivery` mode when implementation begins. The implementation phases will naturally land scopes Done and the guard semantics align then.

## Cross-product impact

This affects every Smackerel planning-only spec (062, 063, future product-to-planning bootstraps). Likely affects other Bubbles consumer repos with the same `product-to-planning` mode.

## Acceptance criteria

- G027 skips ALL-scopes-Done requirement when `statusCeiling == specs_hardened` and `planningOnly == true`.
- G041 parser tolerates pipe-separated trailing metadata on status lines.
- G022 either documents planning-phase markers or skips planning-only specs.
- G040 allowlists language inside `**/cross-spec/packet-*.md` files.
- A new test fixture covers a planning-only spec reaching `specs_hardened` through all four gates without manual workaround.
