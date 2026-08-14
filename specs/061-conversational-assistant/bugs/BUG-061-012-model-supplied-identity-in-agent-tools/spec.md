# Spec: BUG-061-012 — Server-derived principal for agent tools

**Bug:** BUG-061-012
**Spec:** 061-conversational-assistant
**Severity:** S1

## Problem statement

The agent's retrieval tool reads the single operator-owned global corpus with no grant check, and
demands a caller identity it never uses. See `bug.md` for the verified evidence, including the
correction to this packet's first framing.

## Requirements

**R1 — No tool argument may name the caller.**
**R1.1** `user_id` MUST be absent from every agent tool input schema. No agent tool MAY accept a
caller identity as an argument.
**R1.2** The removal MUST be complete rather than cosmetic: the field, its emptiness check, and its
error path all go. A retained-but-unused field is what produced the misleading appearance of access
control that this bug reports.

**R2 — The retrieval boundary requires a grant (the substantive fix).**
**R2.1** `retrieval_search` MUST resolve the caller's session from the request context and MUST
require `auth.GrantGlobalCorpusRead` before it searches.
**R2.2** A context carrying no session MUST fail closed with `retrieval_search_no_principal`. It
MUST NOT fall back to an argument, a default, an empty identity, or a system identity.
**R2.3** A session lacking the grant MUST be refused with `retrieval_search_grant_required`, an
error distinguishable in logs from the no-principal case.
**R2.4** The grant decision MUST reuse `auth.GateGlobalCorpusRead` rather than re-deriving grant
logic, so the agent boundary and the HTTP boundary cannot drift apart.

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
