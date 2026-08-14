# Spec: BUG-061-012 — Server-derived principal for agent tools

**Bug:** BUG-061-012
**Spec:** 061-conversational-assistant
**Severity:** S1

## Problem statement

The identity under which the knowledge corpus is read is chosen by the language model, not derived
by the server. See `bug.md` for the verified evidence.

## Requirements

**R1 — Identity is server-derived.**
**R1.1** No agent tool MAY accept a caller identity as a tool argument. `user_id` MUST be absent
from every agent tool input schema.
**R1.2** A tool that acts on behalf of a user MUST obtain that identity from the request context,
placed there by the surface that authenticated or resolved it.
**R1.3** A tool that finds no principal in context MUST fail closed with a distinct, greppable
error. It MUST NOT fall back to an argument, a default, an empty string, or a system identity.

**R2 — The retrieval boundary requires a grant.**
**R2.1** `retrieval_search` MUST require the `corpus:read` grant on the resolved principal before
it searches.
**R2.2** A principal without that grant MUST be refused with a distinct error, and the refusal MUST
be distinguishable in logs from "no principal present".

**R3 — Every surface supplies a principal or is refused.**
**R3.1** The HTTP surface MUST pass the authenticated session through unchanged. It already does;
this MUST NOT regress.
**R3.2** The Telegram surface MUST resolve `chatID` to a mapped user **before** invoking the agent
and MUST inject that principal. An unmapped chat MUST NOT reach a corpus tool.
**R3.3** System-triggered surfaces (scheduler, pipeline, judgment) MUST inject an explicit system
principal that does **not** carry `corpus:read`, so corpus tools fail closed for them by
construction rather than by omission.

**R4 — The fix is mechanically protected.**
**R4.1** A test MUST fail if `user_id` (or any equivalent caller-identity field) is reintroduced
into any agent tool input schema.
**R4.2** A test MUST fail if a corpus tool resolves an identity from its arguments.
**R4.3** The adversarial tests MUST be written so they fail against the PRE-fix code. A test suite
that passes on both the broken and fixed implementation does not protect this fix.

## Out of scope, and not made safer by this work

- Spec 108's corpus-grant gate on HTTP routes. That is a different boundary, mid-rollout behind a
  ratified OBSERVE window, and this work makes no claim about it.
- The `RequireScope` shared-token/bootstrap bypass (P1 hole 1). Same spec-108 rollout; untouched.
- Telegram inbound fail-closed on an empty allowlist (P1 hole 3). Related surface, separate defect.

## Acceptance

The corpus cannot be read under an identity the model chose, on any of the five surfaces that
invoke the agent, and a reintroduction of the model-supplied form fails a test rather than shipping.
