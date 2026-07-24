# User Validation: [BUG-002-006]

## Checklist

- [x] The final planning baseline preserves the reported wrong-SRI, zero-request Search failure and required request-plus-DOM outcome; this is packet fidelity, not runtime acceptance.
- [x] The plan source-locks HTMX and validates shared read/mutation canaries before Search browser proof.
- [x] The plan requires a complete semantic no-JavaScript form path before client enhancement.
- [x] The plan distinguishes validation, results, no-match, filtered-empty, unauthorized, timeout, network, server error, degraded, and retry states.
- [x] The plan requires real disposable-stack Playwright with no interception, auth injection, response stubbing, or bailout.
- [x] The plan covers keyboard, screen reader, 320px, 200% zoom, reduced motion, privacy, and exactly-once request behavior.

## Goal

- Goal: submit Search by keyboard or pointer and receive a truthful result or recoverable error.
- Success signal: one real request and a visible terminal state under strict CSP/SRI.

## Journey Steps

| Step | User Intent | Observed | Evidence | Friction |
|---|---|---|---|---|
| 1 | Open Search | Reported page render | Interpreted input in `report.md` | works |
| 2 | Submit a query | Reported zero requests after SRI block | Interpreted input in `report.md` | broken |
| 3 | Use repaired Search | Planned in Scope 04; not observed | No runtime evidence | not yet executed |

## Human Acceptance After Implementation

- Confirm Enter and pointer each issue one request and produce truthful visible states.
- Confirm Search remains complete with JavaScript disabled or HTMX unavailable.
- Confirm no query/result content leaks through announcements, storage, URLs, logs, metrics, or recovery context.
