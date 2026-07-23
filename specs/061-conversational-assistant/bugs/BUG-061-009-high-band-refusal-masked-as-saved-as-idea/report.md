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
  (`config/smackerel.yaml` `environments.self-hosted`) is operator-gated — **and
  on the live deploy host `searxng` IS running** (observed
  `smackerel-home-lab-searxng-1` healthy, 30h uptime at deploy time). So
  `web_search` HAS a working provider; a disabled tool is NOT the cause here.
- A question about smackerel's own product almost certainly has **no ingested
  source** in the user's knowledge graph (the user has not captured smackerel's
  own docs), so `internal_retrieval` returns nothing.
- With no citable source, the agent synthesized from the LLM's own weights; the
  citeback verifier correctly rejected the uncited claims → empty sources → the
  provenance gate refused. **This is correct anti-fabrication.** The BUG-061-009
  fix makes that refusal *honest* ("I don't have a sourced answer for that.")
  instead of the misleading "saved as an idea."

**Conclusion:** the refusal itself is correct; the gap is that the user wants an
*answer*. Since `searxng` IS running, a disabled web tool is NOT the cause — the
meta-question still grounded nothing, which needs a deeper investigation, not a
facade change:
1. determine why `web_search` produced no accepted source for this query —
   either the agent did not select `web_search` for a product meta-question, or
   `searxng` returned no citable pages, or the citeback verifier rejected them;
   and/or
2. ingest smackerel's own product docs so `internal_retrieval` can answer
   meta-questions; and/or
3. a product decision to let `open_knowledge` answer general-knowledge questions
   with an explicit "unsourced / general knowledge" caveat (weakens
   `requires_provenance` — needs owner sign-off).

**Routed as follow-up:** `BUG-061-010-open-knowledge-grounding-gap` (to be
created) — out of scope for BUG-061-009, which owns only the honest-refusal
invariant. This bug does NOT claim to make `/ask` answer; it claims the refusal
is now honest.

## Deploy Evidence (local-operator on-host, home-lab)

Built, operator-cosign-signed, and applied on the deploy host per the BUG-061-008
recipe. Source SHA `2e84a1b4`.

**Build (`./smackerel.sh build --target self-hosted`) — exit 0:**

```
[4/7] docker push (capture stable digests)
  core: ghcr.io/pkirsanov/smackerel-core@sha256:dc8963683bc87f6d07b5460a009755c8cd400b5dd56ae20d0f3094307e570c32
  ml:   ghcr.io/pkirsanov/smackerel-ml@sha256:ef16adc279b908b3777e9afbf9558dbbdfe9b8611b741c12cccc5d39db4c1b23
[5/7] cosign sign (operator key)   — core + ml signed
[6/7] syft SBOM + cosign attest    — core + ml attested
[8/9] oras push bundle + cosign sign — config-bundle self-hosted-2e84a1b4… signed
[9/9] emit local-build-manifest    — local-build-manifest-2e84a1b4….yaml
___SMKBUILD_EXIT=0
```

**Apply (`promote.sh --target home-lab --product smackerel`, sudo -n) — exit 0:**

```
Verification for ghcr.io/pkirsanov/smackerel-core@… — The cosign claims were validated
Verification for ghcr.io/pkirsanov/smackerel-ml@…   — The cosign claims were validated
preconditions OK
▶ apply: pulling images by digest (core dc896368…, ml ef16adc2…)
▶ apply: running rollout strategy: recreate
  Container smackerel-home-lab-smackerel-core-1 Recreated → Started
  Container smackerel-home-lab-smackerel-ml-1   Recreated → Started
▶ verify: waiting for strict current-release health
  acceptance: core-digest=accepted
  acceptance: ml-digest=accepted
verify OK (strict current release accepted)
▶ apply: committing verified manifest pointer → apply OK
___SMKDEPLOY_EXIT=0
```

**Independent running verification (docker inspect):**

```
smackerel-home-lab-smackerel-core-1 :: running health=healthy restarts=0
   ghcr.io/pkirsanov/smackerel-core@sha256:dc8963683bc87f6d07b5460a009755c8cd400b5dd56ae20d0f3094307e570c32
smackerel-home-lab-smackerel-ml-1   :: running health=healthy restarts=0
   ghcr.io/pkirsanov/smackerel-ml@sha256:ef16adc279b908b3777e9afbf9558dbbdfe9b8611b741c12cccc5d39db4c1b23
```

Both new digests running, healthy, 0 restarts — matching the built+signed digests.

**Behavioral confirmation (operator-only):** the live Telegram `/ask` smoke test
is operator-verifiable (the prod assistant HTTP API requires a per-user PASETO
token; agents cannot send Telegram messages). The honest-refusal behavior is
proven at the unit level by the extended cross-path invariant test
(`TestExecutionErrorHonesty_*`), which exercises the exact open_knowledge
OK-but-uncited path and asserts `StatusUnavailable` + honest body, never the
capture acknowledgement.
