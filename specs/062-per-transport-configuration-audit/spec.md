# Feature: 062 Per-Transport Configuration Surface Audit

**Status:** not_started (ceiling = `done`)
**Workflow Mode:** `full-delivery`
**Theme:** 060-series — assistant + transport governance
**Owner Directive (2026-06-02):** Spec slot 062 was skipped during the
convergence session that produced specs 060/061/063+. The 2026-06-02
gap-audit (GAPS-2026-06-02-06) confirmed the skip was unintentional.
This spec occupies the slot with a concrete governance deliverable:
a per-transport configuration surface audit that closes the
NO-DEFAULTS / SST-zero-defaults gaps surfaced while landing the HTTP
transport (069), WhatsApp Business transport (072), and the legacy
Telegram surface.

**Depends On:**
- [spec 020 — Security Hardening](../020-security-hardening/spec.md) (SST + fail-loud doctrine)
- [spec 060 — Bearer Auth Scope Claim](../060-bearer-auth-scope-claim/spec.md) (shared bearer config surface)
- [spec 069 — Assistant HTTP Transport](../069-assistant-http-transport/spec.md) (HTTP adapter SST keys)
- [spec 072 — WhatsApp Business Transport](../072-whatsapp-business-transport/spec.md) (WhatsApp adapter SST keys)

---

## 1. Problem Statement

Three assistant-facing transports have now landed (HTTP 069, WhatsApp
072, legacy Telegram), each with its own slice of `config/smackerel.yaml`
keys, environment-variable bindings, and fail-loud startup checks.
There is currently no single artifact that:

1. Enumerates every per-transport runtime configuration value across
   the three transports.
2. Proves each value is wired through the SST pipeline
   (`config/smackerel.yaml` → `config/generated/<env>.env` → adapter
   process) without hidden defaults, helper fallbacks, or
   `${VAR:-default}` Compose forms.
3. Asserts via a committed registry-style test that adding a new
   transport, or adding a new config key to an existing transport,
   either appears in the registry or fails the build.

Without this audit, the next transport (or the next contributor) can
silently reintroduce a fallback default and the NO-DEFAULTS guarantee
becomes aspirational rather than enforced.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Operator** | Owns `config/smackerel.yaml` and per-env `.env` bundles. | Know, for each transport, exactly which keys are required, which are optional, and what fail-loud message they produce when missing. | Edits `config/smackerel.yaml`; runs `./smackerel.sh config generate`. |
| **Contributor (new transport author)** | Adds a fourth transport adapter (e.g. Discord, SMS). | Discover the registry, add their transport's config slice, and have the test suite reject any forgotten fail-loud check. | Edits `internal/assistant/*adapter/`, `internal/config/`, `config/smackerel.yaml`. |
| **Contributor (existing transport maintainer)** | Adds a key to HTTP/WhatsApp/Telegram. | Have the registry test fail until the new key is documented + fail-loud-checked. | Edits the relevant adapter package + the registry. |
| **Auditor / reviewer** | Reviews NO-DEFAULTS compliance during PR review. | Read one document that enumerates all per-transport config values and points to the fail-loud test that protects each one. | Read-only. |

---

## 3. Outcome Contract

**Intent:** Every per-transport runtime configuration value across HTTP
(069), WhatsApp (072), and legacy Telegram is enumerated in a single
committed registry, each value has a fail-loud startup check when the
value is required, and a registry-driven test prevents future transports
from regressing the NO-DEFAULTS guarantee.

**Success Signal:**
- `internal/assistant/transportconfig/registry.go` (or equivalent under
  the assistant package) enumerates every per-transport config key with
  required/optional classification, env-var binding, and the
  human-readable fail-loud message.
- A committed unit test (`registry_test.go`) walks the registry and
  asserts: (a) each required key has a fail-loud check in the owning
  adapter package, (b) no value is sourced via `os.Getenv(k, "default")`
  style helpers, (c) every key in `config/smackerel.yaml` for a
  transport namespace is in the registry, and vice versa.
- `docs/Operations.md` (or a new `docs/Transport_Configuration.md`)
  renders the registry as an operator-facing table.

**Hard Constraints:**
- Zero new hidden defaults introduced. The audit MAY discover existing
  defaults; each one must be either removed (preferred) or explicitly
  ratified in the registry with a `defaultedFor: <reason>` annotation
  that the test allow-lists.
- The registry is the SST for "what per-transport config exists" — it
  MUST be derived-from or kept in lockstep with `config/smackerel.yaml`,
  never a parallel source of truth.

**Failure Condition:** The audit ships, but a contributor can still add
a new transport with a `${WHATSAPP_TOKEN:-}` style fallback and the
test suite passes.

---

## 4. Business Scenarios

### BS-062-01 — Operator missing a required transport key gets a fail-loud message
Given the operator removes `assistant.http.bearer.shared_user_id` from
`config/smackerel.yaml`,
When the operator runs `./smackerel.sh up`,
Then the `smackerel-core` container exits non-zero with a message that
names the missing key and the owning transport ("HTTP assistant
transport requires `assistant.http.bearer.shared_user_id`").

### BS-062-02 — Contributor adds a new transport key without registry entry
Given a contributor adds a new key under `assistant.whatsapp.*` in
`config/smackerel.yaml`,
When they run `./smackerel.sh test unit`,
Then the registry test fails with a message identifying the
undocumented key and pointing to `internal/assistant/transportconfig/registry.go`.

### BS-062-03 — Auditor verifies NO-DEFAULTS compliance in one place
Given the auditor opens the rendered transport-configuration document,
When they look up any per-transport key,
Then they see its required/optional status, env-var binding, fail-loud
message, and a link to the test that protects it.

### BS-062-04 — New transport adapter author discovers the contract
Given a contributor begins adding a Discord transport,
When they read `docs/Connector_Development.md` (or the new transport
doc),
Then they find the registry entry point, an example fail-loud check, and
the registry test to extend.

---

## 5. Non-Functional Requirements

- **Reliability:** Fail-loud checks MUST execute before any transport
  begins accepting traffic. A missing key MUST NOT degrade silently to
  a half-running transport.
- **Maintainability:** Adding a new key MUST require exactly two edits
  (the YAML key + the registry entry) — anything more indicates the
  registry is the wrong shape.
- **Auditability:** Every registry entry MUST cite the spec/scope that
  introduced the key.

---

## 6. Product Principle Alignment

- **Principle 4 — Source-Qualified Processing:** Each transport
  preserves a distinct source identity; per-transport config governance
  is a direct expression of that principle at the configuration layer.
- **Smackerel NO-DEFAULTS SST:** This spec is the enforcement layer for
  the NO-DEFAULTS doctrine across the assistant transport surface.

---

## 7. Out of Scope

- New transport adapters (Discord, SMS, etc.). This spec audits the
  three existing transports.
- Restructuring `config/smackerel.yaml` schema beyond what the registry
  needs.
- ML-sidecar or connector configuration surfaces (those are owned by
  their respective specs).
