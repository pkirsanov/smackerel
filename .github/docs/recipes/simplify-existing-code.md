# <img src="../../icons/donny-ducttape.svg" width="28"> Simplify Existing Code

> *"Cut the nonsense. Keep what actually works."*

Use this when a feature already works, but the code is too noisy, too clever, or too hard to maintain.

## Problem

You do not need a new feature. You need less complexity.

## Solution

Run the dedicated simplification workflow:

```text
/bubbles.workflow  <feature> mode: simplify-to-doc
```

If you want the simplification to stay test-first:

```text
/bubbles.workflow  <feature> mode: simplify-to-doc tdd: true
```

If you want assumptions challenged before the cleanup begins:

```text
/bubbles.workflow  <feature> mode: simplify-to-doc grillMode: required-on-ambiguity
```

> **💡 Tip:** Run `/bubbles.retro hotspots` first to identify the highest-churn, highest-bug-fix-ratio files. This tells you where simplification will have the biggest impact — start with the bug magnets.

## What This Mode Does

- Runs `bubbles.simplify` to reduce complexity
- Proves behavior still works with tests
- Re-validates and audits the result
- Syncs docs and evidence before finish

## Use It For

- Collapsing needless abstractions
- Removing dead branches and duplicated logic
- Flattening over-engineered control flow
- Trimming fragile adapter layers after a feature has settled