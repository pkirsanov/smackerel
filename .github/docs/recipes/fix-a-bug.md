# Recipe: Fix a Bug

> *"That's a nice f***ing kitty right there."* — Found the bug.

---

## The Situation

Something's broken. You need to fix it with proper reproduction, root cause analysis, and regression testing.

## Quick Fix (Fast Track)

```
/bubbles.bug  Users can't log in when password contains special characters
```

**What happens:**
1. Creates a bug folder (`specs/bugs/BUG-NNN-...`)
2. Reproduces the bug with evidence
3. Identifies root cause
4. Implements the fix
5. Adds an adversarial regression test that fails without the fix
6. Verifies the fix with evidence

**Done.** One command.

## Full Pipeline (For Complex Bugs)

```
/bubbles.workflow  bugfix-fastlane for BUG-015

/bubbles.workflow  bugfix-fastlane for BUG-015 tdd: true
```

**Phases:** bug-reproduce → implement-fix → test → validate

## What You Get

```
specs/bugs/BUG-015-login-special-chars/
├── bug.md        # Description, reproduction steps, severity
├── spec.md       # Expected behavior
├── design.md     # Root cause analysis and fix design
├── scopes.md     # Fix scope with DoD
├── report.md     # Before/after evidence
└── state.json    # Status tracking
```

## Rules

- The bug **must be reproduced** before the fix (before/after evidence)
- A regression test **must be added** that fails without the fix
- The regression test **must be adversarial**: it needs input that would fail if the bug came back, not fixtures that already satisfy the broken path
- Required bug-fix tests **must not** use bailout returns that silently pass when the failure occurs
- Fix is not done until evidence shows the test passing with the fix
