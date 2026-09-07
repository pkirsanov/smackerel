---
name: bubbles-spec-template-bdd
description: Enforce spec.md adherence to .specify/templates/spec-template.md with Gherkin-style BDD scenarios and tech-agnostic requirements.
---

# Bubbles Spec Template BDD Compliance

## Purpose
Ensure `spec.md` follows the repository spec template and captures behavior using Gherkin-style BDD scenarios without implementation details.

## When to Use
- Creating `spec.md` from scratch
- Filling or validating `spec.md` content against the template
- Converting free-form requirements into template-compliant BDD scenarios

## Required Format
- Use `.specify/templates/spec-template.md` as the single source of structure and headings.
- Preserve section order and headings exactly.
- Replace all placeholders with concrete content.
- Remove unused template sections only when explicitly allowed by the template.
- The **Outcome Contract** section (Intent, Success Signal, Hard Constraints, Failure Condition) is MANDATORY and must be filled with concrete, testable content — not placeholders.

## BDD Rules (Gherkin Style)
- Write acceptance scenarios using explicit Given, When, Then phrasing.
- Each scenario describes system behavior, not implementation.
- Keep scenarios independent and testable.
- Avoid long chained steps.
- When the spec will enter Bubbles workflow execution, ensure each accepted scenario can later receive a stable `SCN-*` contract entry in `scenario-manifest.json`.

## Tech-Agnostic Rules
- Do not mention languages, frameworks, databases, services, or tools.
- Describe observable behavior, user outcomes, and data effects only.
- Avoid operational or deployment details in requirements.

## Quality Checklist
- All user scenarios include Given, When, Then steps.
- Edge cases are concrete and behavior-focused.
- Functional requirements are testable and implementation-free.
- Success criteria are measurable and technology-agnostic.
- Outcome Contract has concrete Intent, Success Signal, Hard Constraints, and Failure Condition.
- Scenario names are stable enough to support later `scenario-manifest.json` mapping and protected regression coverage.

## References
- `.specify/templates/spec-template.md`
- `.github/copilot-instructions.md`
- `.github/agents/bubbles_shared/feature-templates.md`
- `docs/guides/CONTROL_PLANE_SCHEMAS.md`