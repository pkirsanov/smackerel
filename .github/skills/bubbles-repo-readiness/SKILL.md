---
name: bubbles-repo-readiness
description: Verify-first repo-readiness checks for downstream repos. Use when auditing whether a repo is agent-ready, checking onboarding-doc drift, validating that documented commands map to real repo command surfaces, or improving repo hygiene without mixing it into certification.
---

# Bubbles Repo Readiness

## Goal
Make a repo easier for humans and agents to understand and operate without turning repo hygiene into a hidden promotion gate.

## Portability
This is a portable governance skill. Keep it project-agnostic and advisory by default.

## Non-Negotiables
- Do not treat repo-readiness as `bubbles.validate` certification.
- Do not require `AGENTS.md` as a universal repo index.
- Do not hardcode language- or ecosystem-specific rules as framework law.
- Do not prescribe project-specific commands directly; resolve command truth from the repo command registry and installed framework surfaces.

## When To Use
- Auditing whether a repo is agent-ready
- Checking onboarding or architecture docs for broken local references
- Verifying that documented build, test, lint, or run commands map to real repo command surfaces
- Improving project guidance before installing stricter project-local checks
- Comparing repo hygiene against a verify-first baseline

## Verify-First Workflow
1. Inspect the repo's high-signal onboarding and architecture docs.
2. Check local file references for dead links.
3. Compare documented commands against real repo command surfaces.
4. Check whether minimal onboarding and architecture guidance exists.
5. Classify findings as advisory repo-readiness gaps, not certification failures.

## What To Check
- Broken local references in onboarding docs
- Documented commands versus real command registry or repo entrypoints
- Minimal architecture or entrypoint documentation
- Presence of at least one repo-health or framework-health CI check
- Whether repo-specific agent guidance is compact and navigable

## What Not To Check
- Spec or scope completion status
- `state.json.certification.*`
- Project-specific build quality beyond what the repo itself already governs
- Language-specific tool choices as universal framework requirements

## Output Shape
- Summarize findings by severity and effort
- Separate "repo-readiness" from "certification" clearly
- Recommend the smallest upgrade path first
- Keep the result actionable and mechanical

## Quality Bar
A repo-readiness pass is useful when:
- the checks are portable
- the findings are advisory unless a repo explicitly opts into stricter enforcement
- repo hygiene is improved without weakening Bubbles ownership or certification rules

## References
- `docs/guides/CONTROL_PLANE_DESIGN.md`
- `docs/recipes/framework-ops.md`
- `skills/bubbles-skill-authoring/SKILL.md`