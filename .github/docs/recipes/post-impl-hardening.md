# Recipe: Post-Implementation Hardening

> *"We gotta tighten this up before the shit winds come, Randy."*

---

## The Situation

Implementation and tests pass, but you want the code cleaned up, stable, secure, regression-free, and certified clean before shipping.

## What Happens Automatically

Every delivery mode (`full-delivery`, `bugfix-fastlane`, `feature-bootstrap`, etc.) now includes a **mandatory post-implementation hardening sequence**:

```
implement → test → regression → simplify → stabilize → security → docs → validate → audit → chaos
```

### The Hardening Sequence

| Phase | Agent | What It Does |
|-------|-------|-------------|
| **regression** | Steve French | Cross-spec regression scan, test baseline comparison, conflict detection |
| **simplify** | Donny | Code reuse, dead code removal, duplication cleanup |
| **stabilize** | Shitty Bill | Performance, infrastructure, config, reliability checks |
| **security** | Cyrus | OWASP scan, dependency audit, code security review |

Each phase that produces findings triggers an inline fix cycle (`implement → test`) before the next phase proceeds.

## Manual Hardening

If you want to run just the hardening sequence on existing code:

```
# Full hardening pipeline
/bubbles.workflow  stabilize-to-doc for 042-catalog

# Just regression + stabilize
/bubbles.regression  check for regressions in catalog feature
/bubbles.stabilize  stabilize the catalog feature

# Retro-guided deterministic quality sweep
/bubbles.workflow  retro-quality-sweep for 042-catalog

# Quality sweep with all hardening agents
/bubbles.workflow  stochastic-quality-sweep triggerAgents: regression,simplify,stabilize,security maxRounds: 8
```

> **💡 Tip:** Run `/bubbles.retro hotspots` before hardening to identify bug magnets and worsening hotspots. This tells you which files deserve the most attention during the hardening pass.

> **💡 Tip:** If you want retro to pick the hotspots and then immediately run the deterministic cleanup/hardening chain, use `retro-quality-sweep` instead of wiring together separate retro/simplify/harden steps yourself.

## Zero-Loose-Ends Release Path

If the requirement is not just "run the hardening agents" but "keep going until the whole feature is actually green," use:

```
/bubbles.workflow  full-delivery for <feature>
```

That parent workflow runs reusable child workflows for test verification, deterministic quality sweep, and final certification. New supported scenarios must update planning artifacts plus tests; true defects must be recorded as tracked bugs with regression tests and fixed inline before the run can finish.

## Pre-Ship Checklist

Before shipping a major feature, run all hardening agents in sequence:

```
/bubbles.workflow  harden-gaps-to-doc for <feature>
```

This runs the deterministic quality sweep child workflow: `harden → gaps → implement → test → regression → simplify → stabilize → security → chaos → validate → audit → docs`

The most thorough pre-release verification available. Like Lahey inspecting every trailer before park open.
