# Recipe: Quality Sweep

> *"The shit winds are coming, Randy."*

---

## The Situation

Implementation is done but you want a thorough quality check — find gaps, harden weak spots, ensure nothing slipped through, and keep looping until certification is genuinely clean.

## The Command

```
/bubbles.workflow implement action:full-delivery target:spec for 042-catalog-assistant
```

Or just use natural language:
```
/bubbles.workflow  do a full quality sweep on the catalog feature until everything is green
```

**Parent workflow:** full-delivery

**Built-in mapped workflows:**
- `test-to-doc` for full required test execution and initial verification
- `harden-gaps-to-doc` for the deterministic quality sweep, including chaos
- `validate-to-doc` for final validation, audit, and docs sync

If those mapped workflows discover a legitimate new supported scenario, planning artifacts and tests must be updated before the run continues. If they discover a defect, the active runner creates or updates a tracked bug and executes `bugfix-fastlane` directly through its phase owners.

## What Each Phase Does

### Harden
Looks for fragile code, missing error handling, edge cases, and compliance gaps. The uncomfortable truth-teller.

### Gaps
Finds what nobody noticed — missing test types, undocumented endpoints, spec coverage holes, orphaned code.

### Test
Runs all test suites, verifies coverage, fixes any new failures from hardening.

### Validate
Checks the built-in quality gates, verifies evidence integrity, and enforces artifact ownership routing.

If validate, harden, gaps, stabilize, or security finds missing planning or design content, those agents route the change to `bubbles.plan`, `bubbles.design`, or `bubbles.analyst` instead of patching foreign-owned artifacts themselves.

### Docs
Updates documentation to match the hardened state.

## Alternative: Deterministic Sweep Only

If you want the standalone quality bundle without the full no-loose-ends certification loop:

```
/bubbles.workflow validate action:harden-and-gaps finalize:docs for 042-catalog-assistant
```

## Alternative: Stochastic Sweep

Don't know what to check? Let the system randomly pick:

```
/bubbles.workflow validate action:stochastic scope:portfolio
```

The stochastic parent now does exactly two things per round before execution: pick a spec and pick a trigger. After that it executes the mapped trigger-owned end-to-end workflow mode and **waits for it to complete** before starting the next round.

The active authorized runner resolves the mapped mode in its own top-level runtime and invokes each phase owner directly. The mode mapping is mandatory; no second workflow runner is spawned.

**Rounds are synchronous.** The sweep MUST NOT batch-select all rounds first and then produce a findings table — that is a scoreboard, not a sweep. Each round executes a mapped workflow mode through its phase owners, waits for terminal envelopes, records `executionModel: direct-authorized-runner`, and only then proceeds.

That mapped mode is not allowed to stop at a diagnosis. If the trigger finds a legitimate bug, regression, gap, or improvement, it must run the full finding-owned closure workflow before returning a terminal result:

- Planning workflow: `bubbles.analyst` → `bubbles.ux` when UI or a user-visible journey is implicated → `bubbles.design` → `bubbles.plan`
- Delivery workflow: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` → finalize/certification

**Built-in trigger-owned workflow map:**
- `chaos` → `chaos-hardening`
- `harden` → `harden-to-doc`
- `gaps` → `gaps-to-doc`
- `simplify` → `simplify-to-doc`
- `stabilize` → `stabilize-to-doc`
- `test` → `test-to-doc`
- `devops` → `devops-to-doc`
- `validate` → `reconcile-to-doc`
- `improve` → `improve-existing`
- `security` → `security-to-doc`
- `regression` → `regression-to-doc`

When a stochastic sweep turns up real work, keep the remediation inside workflow orchestration:

```
/bubbles.workflow  fix all found
/bubbles.workflow  address the rest
```

Those follow-ups now preserve the active sweep context when continuation state is available, so the system keeps the workflow-owned fix/finalize chain instead of collapsing into raw `/bubbles.implement` or `/bubbles.test` advice.

The sweep is not allowed to stop at a scoreboard. Each round must either finish through the mapped trigger-owned workflow mode or emit a workflow-owned continuation packet that preserves the non-terminal mapped-mode outcome. A summary-only finish is invalid while routed or blocked work remains.

The same rule applies outside stochastic sweeps: when a finding-capable phase discovers real work, the active runner must execute the same planning-to-delivery closure substream before reporting a terminal result.

For repeated passes from one specialist angle, constrain the trigger pool instead of using a deterministic batch mode:

```
/bubbles.workflow validate action:stochastic scope:portfolio triggerAgents: stabilize maxRounds: 10
```

Use that pattern for requests like "do 10 rounds of stabilize" or similar single-specialist sweeps. Those are round-based stochastic passes, not `stabilize-to-doc` or other deterministic spec-batch workflows.

Like bottle kids — you never know where they'll hit, but they always find something.

## Alternative: Retro-Guided Sweep

Want the targeting to be data-driven but the remediation path to be deterministic?

```
/bubbles.workflow  <feature> mode: retro-quality-sweep
```

This starts with `bubbles.retro` to identify hotspots, then runs a fixed cleanup-and-hardening chain on those areas: `simplify → harden → gaps → implement → test → regression → stabilize → devops → security → validate → audit → docs`.

## Individual Quality Tools

```
/bubbles.harden         # Deep hardening pass
/bubbles.gaps           # Find missing pieces
/bubbles.chaos          # Break things on purpose
/bubbles.security       # Security scan
/bubbles.retro hotspots # Find bug magnets, hidden coupling, and bus factor risks
```
