# Report: [BUG-080-001] Graph API Fails Soft Into Runtime Disablement

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts only were initialized on 2026-07-23. No source, secret, config generation, host, operator deploy repository, test, production, commit, push, or deployment mutation occurred.

## Completion Statement

Incomplete and non-terminal. Status remains `in_progress`; design/planning, reproduction, implementation, devops injection, testing, validation, and audit are unclaimed.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** Graph cursor-secret indirection resolves empty; core warns and leaves handlers nil; topics/people/places/time/edges 404 while static Wiki and strict deployment verification pass.
- **Evidence status:** no secret, config, startup, HTTP, browser, or deploy output was captured here.

## Decision Record

- Required capability configuration must fail loud before serving.
- Product acceptance requires authenticated reads, not static pages or health alone.
- Smackerel owns the generic contract; operator deploy-adapter consumption is devops-owned and untouched.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declarations

- Exact config/wiring branches and strict-acceptance omission are not locally confirmed.
- No secret value was read and no red/green regression exists.

## Scenario Contract Evidence

Initialized in [scenario-manifest.json](scenario-manifest.json); evidence references are empty.

## Validation Summary

No completion validation or certification was performed.

## Audit Verdict

Not audited. No terminal verdict is claimed.
