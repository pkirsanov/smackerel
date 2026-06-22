# <img src="../../icons/green-bastard-mask.svg" width="28"> Adversarial Verification (Red-Team)

> *"Nothing's bulletproof, boys. Let me prove it."*

Use this when you want a finished result **attacked**, not just checklisted — Green Bastard (`bubbles.redteam`) tries to make the "done" claim **false** with evidence-first counterexamples, risk-gated multi-validator (voting) scrutiny, or a bounded chaos-monkey probe against a live system.

`bubbles.redteam` pairs with `bubbles.grill`: grill pressure-tests *ideas before* you build; red-team attacks *results after* you build. It emits **findings** and **never certifies** — completion authority stays with `bubbles.validate`.

## Problem

Sectioning verifiers (audit, security, regression) each own a disjoint domain, and the mechanical gates catch fabrication — but nothing is chartered to actively *falsify* a finished result, and no two independent validators attack the **same** high-risk claim and reconcile.

## Solution — Off By Default, Opt In Seamlessly

The capability is **off by default**. Turn it on through one layered precedence chain (highest wins): per-run directive → `BUBBLES_ADVERSARIAL*` env → `.github/bubbles-project.yaml` `adversarial:` block → framework default `off`.

**Session / CI default (env):**

```bash
export BUBBLES_ADVERSARIAL=auto      # auto = run only on high-risk scopes
export BUBBLES_ADVERSARIAL_PASSES=3  # 3 independent validators (voting)
```

**Repo default (config):**

```yaml
# .github/bubbles-project.yaml
adversarial:
  mode: auto        # off | auto | on
  passes: 1         # validators (1 = single pass; >=2 = voting ensemble)
  teeth: warn       # warn (grandfathered) | blocking
```

**Per-run, for one agent/workflow only (directive — highest precedence):**

```text
/bubbles.redteam  <describe what to attack>
/bubbles.workflow <feature> mode: redteam-to-doc adversarial: on passes: 3
```

Resolve the effective posture any time:

```bash
bash bubbles/scripts/adversarial-resolve.sh --repo-root .
# → mode= / passes= / teeth= / source=
```

## Three Modes Of Attack

| You want | Invoke |
|----------|--------|
| Attack a finished result (counterexamples) | `/bubbles.redteam attack the result` or `mode: redteam-to-doc` |
| N independent validators vote on a high-risk change | `/bubbles.redteam get 3 validators on this` (`passes: 3`) |
| Bounded chaos-monkey probe of a LIVE system | `mode: production-adversarial-probe` (requires arming + target allowlist) |

## Release + Ops Integration

- **Release:** on a release-phase scenario (`bubbles.goal` / `bubbles.sprint` release mode), red-team runs a pre-ship adversarial pass on high-risk delivered features (riskClass-gated). Findings route through the normal fix-cycle.
- **Ops / incident:** available alongside `incident-fastlane` as the "attack the hypothesis" probe — does the fix actually hold under hostile input?
- **Production chaos-monkey:** `production-adversarial-probe` runs Green Bastard on a leash — **armed + allowlisted only**, read-only operate plane (it never mutates/silences prod telemetry), bounded + seeded, and **restore-or-fix** before the round completes (cleanup failure is a blocking stop).

## When It Helps Most

- A high-risk change (money, auth, data-mutation, irreversible ops) is about to be certified
- You don't fully trust a single audit pass and want independent voting
- You want to prove resilience against hostile input on a running system
- A "done" claim feels too clean

## Good Follow-Ups

- `/bubbles.bug <counterexample>` to document a successful attack
- `/bubbles.implement` then `/bubbles.test` to fix it and add durable regression coverage
- `/bubbles.validate <feature>` to re-certify after remediation
- `/bubbles.security <feature>` when an attack exposes an auth/IDOR/decode vulnerability class
