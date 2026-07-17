# <img src="../../icons/green-bastard-mask.svg" width="28"> Adversarial Verification (Red-Team)

> *"Nothing's bulletproof, boys. Let me prove it."*

Use this when you want a finished result **attacked**, not just checklisted. Green Bastard (`bubbles.redteam`) tries to make the "done" claim **false** with evidence-first counterexamples, risk-gated correlated samples, or a bounded chaos-monkey probe against a live system.

`bubbles.redteam` pairs with `bubbles.grill`: grill pressure-tests *ideas before* you build; red-team attacks *results after* you build. It emits **findings** and **never certifies** — completion authority stays with `bubbles.validate`.

## Problem

Sectioning verifiers (audit, security, regression) each own a disjoint domain, and the mechanical gates catch fabrication. Red-team verification adds an explicit attempt to *falsify* a finished result. For high-risk or uncertain claims, a top-level workflow can request multiple checks from the active runtime and reconcile every returned finding.

## Solution — Off By Default, Opt In Seamlessly

The capability is **off by default**. Turn it on through one layered precedence chain (highest wins): per-run directive → `BUBBLES_ADVERSARIAL*` env → `.github/bubbles-project.yaml` `adversarial:` block → framework default `off`.

**Session / CI default (env):**

```bash
export BUBBLES_ADVERSARIAL=auto      # auto = run only on high-risk scopes
export BUBBLES_ADVERSARIAL_SAMPLES=3 # 3 correlated checks in the active runtime
```

**Repo default (config):**

```yaml
# .github/bubbles-project.yaml
adversarial:
  mode: auto        # off | auto | on
  samples: 1        # correlated samples; 1 is the normal default
  teeth: warn       # warn (grandfathered) | blocking
```

**Per-run, for one agent/workflow only (directive — highest precedence):**

```text
/bubbles.redteam  <describe what to attack>
/bubbles.workflow <feature> mode: redteam-to-doc adversarial: on samples: 3
```

Resolve the effective posture any time:

```bash
bash bubbles/scripts/adversarial-resolve.sh --repo-root .
# Output includes mode, samples, sampleSemantics, teeth, source,
# samplesSource, and deprecation.
```

The resolver always emits `sampleSemantics=same-runtime-correlated`. The old
`passes` input remains accepted only as deprecated compatibility syntax. Alias
use emits a deprecation message and `deprecation=passes-alias`; new environment,
config, directive, and output examples use `SAMPLES` or `samples`.

## Sample Execution And Aggregation

One `bubbles.redteam` invocation emits exactly one schema-version-1 sample
record. Asking that direct invocation for more than one sample does not create
the missing invocations. To run `samples: N`, use an active top-level workflow,
which must dispatch `bubbles.redteam` exactly N times with unique sample and
invocation IDs, retain each record's runtime/model/tool provenance and
verification state, and then call the deterministic aggregator.

Each record uses
`bubbles/eval/schemas/adversarial-sample.schema.json` and carries:

- `sampleSemantics: same-runtime-correlated`
- `status`: `completed`, `error`, or `unavailable`
- `verdict`: `clear`, `findings`, `error`, or `unavailable`
- invocation identity plus runtime, model, and tool provenance
- the complete finding list, or structured error details when not completed

The top-level aggregator fingerprints each finding from category, target,
evidence reference, and claim. It preserves every unique finding and emits one
of four outcomes:

| Outcome | Meaning |
| --- | --- |
| `agreement-clear` | Every completed sample is clear and has no findings. |
| `agreement-findings` | Every completed sample reports the same finding and blocking-finding fingerprint sets; route the complete set. |
| `disagreement` | Verdicts or finding sets differ; block normal continuation and escalate the union plus the per-sample matrix. |
| `aggregation-error` | A count, schema, provenance, or sample-status requirement failed; block the workflow. |

These samples inherit the active VS Code model and tools, so they are correlated
checks rather than evidence of model or tool independence. Bubbles currently has
no verified external provider/model adapter. A majority cannot silence a
finding: disagreement preserves and escalates the union.

## Three Modes Of Attack

| You want | Invoke |
| --- | --- |
| Attack a finished result (counterexamples) | `/bubbles.redteam attack the result` or `mode: redteam-to-doc` |
| N correlated checks on a high-risk or uncertain claim | `/bubbles.workflow <feature> mode: redteam-to-doc adversarial: on samples: N` |
| Bounded chaos-monkey probe of a LIVE system | `mode: production-adversarial-probe` (requires arming + target allowlist) |

## Release + Ops Integration

- **Release:** on a release-phase scenario (`bubbles.goal` / `bubbles.sprint` release mode), red-team runs a pre-ship adversarial sample on high-risk delivered features (riskClass-gated). Findings route through the normal fix-cycle.
- **Ops / incident:** available alongside `incident-fastlane` as the "attack the hypothesis" probe — does the fix actually hold under hostile input?
- **Production chaos-monkey:** `production-adversarial-probe` runs Green Bastard on a leash — **armed + allowlisted only**, read-only operate plane (it never mutates/silences prod telemetry), bounded + seeded, and **restore-or-fix** before the round completes (cleanup failure is a blocking stop).

## When It Helps Most

- A high-risk change (money, auth, data-mutation, irreversible ops) is about to be certified
- Risk or uncertainty justifies bounded additional checks in the active runtime
- You want to prove resilience against hostile input on a running system
- A "done" claim feels too clean

## Good Follow-Ups

- `/bubbles.bug <counterexample>` to document a successful attack
- `/bubbles.implement` then `/bubbles.test` to fix it and add durable regression coverage
- `/bubbles.validate <feature>` to re-certify after remediation
- `/bubbles.security <feature>` when an attack exposes an auth/IDOR/decode vulnerability class
