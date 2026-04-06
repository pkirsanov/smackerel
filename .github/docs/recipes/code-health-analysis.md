# Recipe: Analyze Code Health (Hotspots, Coupling, Bus Factor)

## When to Use

- Before a refactoring sprint — find where the real problems are
- When a file keeps breaking and you want data to confirm the pattern
- When you suspect hidden architectural dependencies between modules
- When worried about knowledge silos (one person owns critical code)
- After a feature delivery to check if hotspots are stabilizing or worsening
- During a planning session to decide where to invest engineering effort

## Quick Answers

### "Which files keep breaking?"

```
/bubbles.retro hotspots
```

Shows files ranked by **bug-fix ratio** — the percentage of commits that are bug fixes vs feature work. Files above 50% are "bug magnets."

### "Are there hidden dependencies?"

```
/bubbles.retro coupling
```

Shows file pairs that always change together, especially cross-directory pairs. High coupling between unrelated directories = hidden architectural dependency. Consider extracting a shared module.

### "What's our bus factor?"

```
/bubbles.retro busfactor
```

Shows files where only one person has made changes. If that person is unavailable, nobody else has context. Consider pairing or code review rotation.

### "Is the codebase getting better or worse?"

```
/bubbles.retro month
```

Full retro with **hotspot trend comparison** against prior retros. Shows stabilizing, worsening, new, and resolved hotspots.

## Acting On Findings

After running a hotspot analysis, the output includes a **Recommended Actions** section:

| Verdict | Icon | What To Do Next |
|---------|------|-----------------|
| Bug magnet | 🔴 | `/bubbles.simplify` on the file, or `/bubbles.code-review` for deeper analysis |
| Hidden coupling | 🔴 | `/bubbles.code-review scope: module:<dir>` to review architecture |
| Single-author file | 🟡 | Pairing or code review rotation (not a Bubbles action — team concern) |
| Worsening hotspot | 🟡 | `/bubbles.harden` or `/bubbles.gaps` targeting those files |
| Stabilizing hotspot | 🟢 | Good news — the refactoring is working |

## Full Workflow: Data-Driven Refactoring

```
# 1. Find the problems
/bubbles.retro hotspots

# 2. Review the worst offenders
/bubbles.code-review  scope: file:backend/api/router.rs

# 3. Simplify
/bubbles.simplify

# 4. Verify improvement in next retro
/bubbles.retro hotspots
```

## Related Agents

| Agent | What It Does | When To Use |
|-------|-------------|-------------|
| `bubbles.retro` | Hotspot analysis from git history | First — find the problems |
| `bubbles.code-review` | Engineering review of specific code | Second — understand the problems |
| `bubbles.simplify` | Reduce complexity | Third — fix the problems |
| `bubbles.system-review` | Holistic product/architecture review | When the problem is system-level, not file-level |

## Tips

- Run `hotspots` weekly to track trends
- Compare coupling data with your intuition — if you always change A and B together, coupling analysis confirms it
- Bus factor analysis is about the team, not the framework — Bubbles reports the data, you take the action
- Combine with `/bubbles.retro week` for velocity + hotspots in one report
