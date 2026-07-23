# BUG-061-009 — Report

## Summary

Closes the last "saved as an idea" masking path: a band-high (matched + executed)
turn that cannot ground an answer is refused HONESTLY (`StatusUnavailable` +
`ErrNoGroundedAnswer` + "I don't have a sourced answer for that.") instead of the
misleading capture acknowledgement. Converts "saved as an idea" into a
band-LOW-only invariant (INV-HB-REFUSAL) with a cross-path invariant test so the
class cannot recur, and upgrades refusal-vs-answer distinguishability from a
user-visible "(saved as idea)" string to a structural (`Status`/`ErrorCause`)
contract.

## Completion Statement

SCOPE-01..05 implemented. INV-HB-REFUSAL is enforced end-to-end: the provenance
gate refuses OK-but-uncited turns into the honest `StatusUnavailable` +
`ErrNoGroundedAnswer` shape; `canonicalizeSuccessfulCaptureResponse(resp, band, …)`
emits the capture ack for `BandLow` only (band-high residual `StatusSavedAsIdea`
is converted to the honest refusal); the open_knowledge refusal renderer dropped
its `(saved as idea)` suffix and the 5 cause strings were reworded, making
refusal-vs-answer distinguishability structural. The dead
`provenance.EnforceRefusal` parallel refusal path was removed and the live
refused-turn path (facade source-assembler Override) is now the single
refusal-shaping path, directly unit-tested. Full Go suite + lint green.

## Test Evidence

### `./smackerel.sh test unit --go` — first run (3 EXPECTED failures pre-fix)

The whole module compiled; `internal/assistant` and `internal/assistant/provenance`
(the core changed packages) passed on the first run. Three expected failures
remained where ratified guards had to be updated for the new honest shape:

```
ok      github.com/smackerel/smackerel/internal/assistant       0.266s
--- FAIL: TestOpenKnowledgeAssembler_RefusedEnvelope (0.00s)
    wiring_assistant_openknowledge_test.go:152: refused body mismatch: ""
FAIL    github.com/smackerel/smackerel/cmd/core 1.098s
--- FAIL: TestAllErrorCauses_Exhaustive (0.00s)
    response_test.go:53: AllErrorCauses length 7 != declared 6
--- FAIL: TestGoldenCases_CoverEveryCombinationAxis (0.00s)
    response_test.go:468: ErrorCause "no_grounded_answer" not covered by any golden fixture
FAIL    github.com/smackerel/smackerel/internal/assistant/contracts     0.035s
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.020s
ok      github.com/smackerel/smackerel/internal/telegram        27.574s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter      0.026s
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter      (cached)
```

The three failures were the update points, not regressions: (1) the existing
assembler refused-test asserted the pre-fix `SourceAssembly{Body,Cause}` shape —
updated to assert the honest `Override` (multi-cause); (2) the closed-vocabulary
`ErrorCause` count needed `ErrNoGroundedAnswer` (6→7); (3) a golden snapshot for
the new `no_grounded_answer` fixture was created.

### `./smackerel.sh test unit --go` — second run (all green, exit 0)

```
ok      github.com/smackerel/smackerel/cmd/core 0.935s
ok      github.com/smackerel/smackerel/internal/assistant/contracts     0.043s
=== exit: 0 ===
```

Every package passed (exit 0); the two previously-failing packages (`cmd/core`,
`internal/assistant/contracts`) are green, and no other package regressed.

### `./smackerel.sh lint` (Go vet + ruff + web assets)

```
Building wheels for collected packages: smackerel-ml
Successfully built smackerel-ml
Installing collected packages: … smackerel-ml
Successfully installed … smackerel-ml-0.1.0 …
All checks passed!
```

### `./smackerel.sh check` (SST / config / scenario-lint)

```
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
```

## Artifact-lint

`bash .github/bubbles/scripts/artifact-lint.sh specs/061-conversational-assistant/bugs/BUG-061-009-…`

```
✅ Required artifact exists: spec.md / design.md / uservalidation.md / state.json / scopes.md / report.md
✅ Found DoD section in scopes.md
✅ All DoD bullet items use checkbox syntax in scopes.md
✅ Detected state.json status: in_progress
✅ report.md contains section matching: Summary / Completion Statement / Test Evidence
=== Anti-Fabrication Evidence Checks ===
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
=== artifact-lint exit: 0 ===
```

## Grounding-gap follow-up (SCOPE-05 diagnosis)

**Question:** why did `/ask how smackerel works as second brain or llm wiki?`
ground nothing (agent `status=success termination=final`, but zero sources →
refusal)?

**Evidence (config + observed behavior):**
- The open_knowledge agent's `tool_allowlist` is
  `[ internal_retrieval, web_search, unit_convert, calculator ]`
  (`config/smackerel.yaml`). So grounding a meta-question requires EITHER
  `web_search` returning citable pages OR `internal_retrieval` finding ingested
  smackerel-about-smackerel content.
- On the self-hosted env, `searxng_enabled: "${ENABLE_SEARXNG}"`
  (`config/smackerel.yaml` `environments.self-hosted`) — web grounding is
  **operator-gated**. If `ENABLE_SEARXNG` is unset / the `searxng` profile is not
  running on the deploy host, `web_search` has no working provider.
- A question about smackerel's own product almost certainly has **no ingested
  source** in the user's knowledge graph (the user has not captured smackerel's
  own docs), so `internal_retrieval` returns nothing.
- With no citable source, the agent synthesized from the LLM's own weights; the
  citeback verifier correctly rejected the uncited claims → empty sources → the
  provenance gate refused. **This is correct anti-fabrication.** The BUG-061-009
  fix makes that refusal *honest* ("I don't have a sourced answer for that.")
  instead of the misleading "saved as an idea."

**Conclusion:** the refusal itself is correct; the gap is that the user wants an
*answer*. That requires an operator/product action, not a facade change:
1. set `ENABLE_SEARXNG=true` (+ run the `searxng` profile) on the deploy host so
   `/ask` can ground meta-questions in web sources; and/or
2. ingest smackerel's own product docs so `internal_retrieval` can answer
   meta-questions; and/or
3. a product decision to let `open_knowledge` answer general-knowledge questions
   with an explicit "unsourced / general knowledge" caveat (weakens
   `requires_provenance` — needs owner sign-off).

**Routed as follow-up:** `BUG-061-010-open-knowledge-grounding-gap` (to be
created) — out of scope for BUG-061-009, which owns only the honest-refusal
invariant. This bug does NOT claim to make `/ask` answer; it claims the refusal
is now honest.
