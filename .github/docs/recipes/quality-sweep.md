# Recipe: Quality Sweep

> *"The shit winds are coming, Randy."*

---

## The Situation

Implementation is done but you want a thorough quality check — find gaps, harden weak spots, ensure nothing slipped through, and keep looping until certification is genuinely clean.

## The Command

```
/bubbles.workflow  delivery-lockdown for 042-catalog-assistant
```

Or just use natural language:
```
/bubbles.workflow  do a full quality sweep on the catalog feature until everything is green
```

**Parent workflow:** delivery-lockdown

**Built-in child workflows:**
- `test-to-doc` for full required test execution and initial verification
- `harden-gaps-to-doc` for the deterministic quality sweep, including chaos
- `validate-to-doc` for final validation, audit, and docs sync

If those child workflows discover a legitimate new supported scenario, planning artifacts and tests must be updated before the run continues. If they discover a defect, the workflow creates or updates a tracked bug and adds the regression test before running `bugfix-fastlane` inline.

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
/bubbles.workflow  harden-gaps-to-doc for 042-catalog-assistant
```

## Alternative: Stochastic Sweep

Don't know what to check? Let the system randomly pick:

```
/bubbles.workflow  stochastic-quality-sweep
```

When a stochastic sweep turns up real work, keep the remediation inside workflow orchestration:

```
/bubbles.workflow  fix all found
/bubbles.workflow  address the rest
```

Those follow-ups now preserve the active sweep context when continuation state is available, so the system keeps the workflow-owned fix/finalize chain instead of collapsing into raw `/bubbles.implement` or `/bubbles.test` advice.

For repeated passes from one specialist angle, constrain the trigger pool instead of using a deterministic batch mode:

```
/bubbles.workflow  stochastic-quality-sweep triggerAgents: stabilize maxRounds: 10
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
