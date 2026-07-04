# <img src="../../icons/orangie-fishbowl.svg" width="28"> Readiness Review

> *"Orangie sees everything. He's not dead, he's just... reviewing."*

Use this when the work looks finished and you want one honest ship/no-ship read before release.

## Problem

Individual gates are green, but nobody has synthesized the full picture — spec, code, system, security, regression, and adversarial (redteam) lenses — into a single release-readiness call.

## Solution

Run the synthesizer mode against the finished feature:

```text
/bubbles.workflow  specs/NNN-feature-name mode: readiness-review
```

`readiness-review` is homed on `bubbles.system-review` (Orangie). It reads the existing evidence across the spec/code/system/security/regression/redteam lenses and persists ONE advisory verdict to `certification.readinessVerdict`:

- `ship` — no blocking concerns found across the lenses
- `ship-with-notes` — shippable, but with recorded caveats to track
- `not-ready` — at least one lens surfaced a blocking concern

## What It Persists

- `certification.readinessVerdict` — advisory only. It is **advisory-to-release**, NOT a completion authority: it NEVER performs a `done` transition (G056 unchanged). Certification stays with `bubbles.validate`.

## When It Helps Most

- Right before cutting a release or a release-train promotion
- After a hardening/regression/redteam pass, to synthesize the result
- When you want a single, honest readiness call instead of scattered gate output

## Good Follow-Ups

- `/bubbles.validate <feature>` when you need the actual certification authority
- `/bubbles.workflow <feature> mode: post-impl-hardening` when the verdict is `not-ready`
- `/bubbles.harden <feature>` or `/bubbles.security <feature>` to close a lens that blocked `ship`
