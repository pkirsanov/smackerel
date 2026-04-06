# Recipe: Run a Retrospective

## When to Use

- End of a work session to capture velocity and patterns
- End of week to track shipping trends
- After a major feature delivery to analyze the lifecycle
- When you feel like things are slowing down and want data, not vibes
- When you want to find which files are bug magnets or knowledge silos
- When you suspect hidden architectural dependencies (files that always change together)
- Before starting a refactoring effort — find where the real problems are

## Commands

```
# Quick session retro (recent git activity)
/bubbles.retro

# Weekly retro
/bubbles.retro week

# Monthly retro
/bubbles.retro month

# Retro for a specific spec
/bubbles.retro spec 042

# Full retro across all specs
/bubbles.retro all

# Deep code hotspot analysis only (bug magnets, coupling, bus factor)
/bubbles.retro hotspots

# Time-bounded hotspot analysis
/bubbles.retro hotspots week
/bubbles.retro hotspots month

# Co-change coupling only (hidden architectural dependencies)
/bubbles.retro coupling

# Bus factor / author concentration only
/bubbles.retro busfactor
```

## What You Get

### Standard Retro (default, week, month, spec, all)
- **Velocity metrics**: scopes completed, DoD items validated, lines changed, commits
- **Gate health**: which gates fail most, most-retried phases
- **Hotspot analysis**: most-modified files, bug magnets, co-change coupling, bus factor, churn trends
- **Trend comparison**: velocity and failure rate vs. prior retros
- **Recommended actions**: targeted follow-up commands based on findings
- **Concrete observations**: 2-3 actionable insights specific to your repo

### Hotspot-Only (`hotspots`)
- **File churn**: top 10 most-modified files
- **Bug magnets**: files with highest bug-fix commit ratio (🔴 > 50%)
- **Co-change coupling**: file pairs that always change together, especially cross-directory
- **Bus factor**: single-author files (knowledge silos)
- **Churn trends**: stabilizing, worsening, new, and resolved hotspot comparison vs. prior retro
- **Recommended actions**: which files to simplify, review, or harden

### Coupling-Only (`coupling`)
- Top 10 file pairs that co-change most frequently
- Coupling percentage per pair
- Cross-directory coupling highlighted (hidden architectural dependencies)

### Bus Factor-Only (`busfactor`)
- Per-file author count for high-churn files
- Single-author files flagged as knowledge silos

## Acting On Retro Findings

The retro output includes a **Recommended Actions** section with copy-pasteable commands:

| Finding | Recommended Follow-Up |
|---------|----------------------|
| Bug magnet file | `/bubbles.simplify` or `/bubbles.code-review` on that file |
| Hidden coupling | `/bubbles.code-review` for architectural review of the coupled modules |
| Single-author file | Informational — consider pairing or code review rotation |
| Worsening hotspot | `/bubbles.harden` or `/bubbles.gaps` targeting those files |
| Stabilizing hotspot | Positive signal — refactoring is working |

## Output Location

Retros are saved to `.specify/memory/retros/YYYY-MM-DD.md`. Each retro references the prior one for trend comparison.

## Tips

- Enable metrics (`bubbles.sh metrics enable`) for richer gate health data
- Run retros consistently (weekly) to build trend baselines for hotspot tracking
- Use spec-scoped retros to understand why certain features took longer
- Use `hotspots` before a refactoring sprint to prioritize what to fix first
- Run `coupling` when you notice "I always have to change X when I change Y" — it confirms the pattern
- Retros are read-only — they never modify code, tests, or state
- The "Recommended Actions" section gives you the next command to run — just copy and paste
