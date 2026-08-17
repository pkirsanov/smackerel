# Bug Artifact Templates

Use these templates when creating bug artifacts.

## bug.md Template

```markdown
# Bug: [BUG-NNN] Short Description

## Summary
One-line summary of the bug.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [ ] Reported
- [ ] Confirmed (reproduced)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Step 1
2. Step 2
3. ...

## Expected Behavior
What should happen.

## Actual Behavior
What actually happens.

## Environment
- Service: [service-name]
- Version: [commit/version]
- Platform: [OS/browser/device]

## Error Output
```
[stack trace or error message]
```

## Root Cause (filled after analysis)
[Description of root cause]

## Related
- Feature: `specs/[NNN-feature-name]/`
- Related bugs: [links]
- Related PRs: [links]

## Deferred Reason (if mode: document)
[Why this bug is being deferred, priority, when to fix]
```

## design.md Template (Bug Fix)

```markdown
# Bug Fix Design: [BUG-NNN]

## Root Cause Analysis

### Investigation Summary
[What was investigated]

### Root Cause
[Precise technical root cause]

### Impact Analysis
- Affected components: [list]
- Affected data: [if any]
- Affected users: [scope]

## Fix Design

### Solution Approach
[Chosen solution and why]

### Alternative Approaches Considered
1. [Alternative 1] - Why rejected
2. [Alternative 2] - Why rejected

## Complexity Tracking
List any deviation from the simplest fix that resolves the root cause. If the fix introduces no such deviation, write a single line: `None — simplest viable fix used.` Otherwise add one row per deviation. This is a lightweight documentation discipline, not a blocking gate.

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| [Added complexity beyond the minimal fix] | [The minimal fix, e.g., a one-line guard] | [Concrete reason the minimal fix was insufficient] |
```


## Additional Bug Artifact Expectations

The bug-artifact question has ONE authority:
[`bubbles/registry/bug-packet.yaml`](../../bubbles/registry/bug-packet.yaml).

It records all three admissible forms — the full packet, the compact micro-fix
packet, and the deliberate single-file `BUGS.md` form for framework source bugs —
along with the state.json and regression expectations that apply to each. The
list used to be restated here, in `BUGS.md`, and in `micro-fix-packet.yaml`, and
the three copies had drifted into different answers.

Read the registry. Do not restate its lists in a spec, an agent, or a script: a
second copy is a second answer.
