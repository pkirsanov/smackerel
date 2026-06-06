# BUG-042-001 — Certified-Done Spec With Not-Started Scopes (Scope-Status Reconciliation)

**Spec:** 042-tailnet-edge-bind-pattern
**Severity:** Governance (artifact-only; zero runtime change)
**Status:** open → resolved
**Discovered:** 2026-06-06
**Discovered by:** OPS-002 G088 review (certified-`done` spec whose `scopes.md` carried `Not started` scopes)
**Closure Mode:** bugfix-fastlane (artifact reconciliation only; mirrors BUG-029-007 precedent)

---

## Summary

`specs/042-tailnet-edge-bind-pattern` carries `state.json status: done` and
`certification.status: done`, but its `scopes.md` declared **both** active scopes
as `Not started` with **26 unchecked DoD items** (Scope 1 "Fail-loud compose
contract and mechanical guard": 14 items; Scope 2 "Operator docs and agent
guardrails": 12 items). The certified `done` status and the scope statuses were
**inconsistent**, and `artifact-lint.sh specs/042-tailnet-edge-bind-pattern`
FAILED with `state.json status 'done' is invalid: DoD contains unchecked items`.

The root cause is a post-promotion reconciliation commit `15e1c453` (2026-05-25)
that flipped the `HOST_BIND_ADDRESS` contract to fail-loud
(`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}`), rewrote
scenarios `SCN-042-001..004`, added NFR-5/NFR-6, and **reset both scopes to
`Not started`** — but the re-verification was never completed and the DoD was
never re-ticked. The implementation, however, **HAS shipped and is enforced**: a
fresh probe of the live spec-042 surface returns zero functional regression.

This BUG packet closes the inconsistency by **re-verifying every one of the 26
DoD items against the shipped+tested code/docs with real command evidence** and
re-ticking only those genuinely satisfied, restoring both scopes to `Done`, and
recertifying the parent. No DoD item was force-ticked.

### Reality-check probe (all green except one disclosed non-042 caveat)

- `deploy/compose.deploy.yml` L128/L190 — `smackerel-core`/`smackerel-ml` use the
  fail-loud `${HOST_BIND_ADDRESS:?...}` prefix; `postgres` (L33) and `nats` (L74)
  have **no** `ports:` block. ✅
- `go test -count=1 -v ./internal/deploy/ -run 'Compose'` — `ok` 0.040s;
  `TestComposeContract_LiveFile` + adversarial LiteralBind / DefaultFallbackBind /
  InfraHasPorts / NetworkModeHostBypass / Multi+MLMultiPortsBypass /
  OllamaLiteralBind / PrometheusLiteralBindAndFallbackForms all PASS. ✅
- `docker compose -f deploy/compose.deploy.yml config` without `HOST_BIND_ADDRESS`
  → exit 1 with `HOST_BIND_ADDRESS must be set by deploy adapter`. ✅
- `HOST_BIND_ADDRESS=127.0.0.1 docker compose ... config` → exit 0; core binds
  `127.0.0.1:41001`, ml binds `127.0.0.1:41002`; no infra ports. ✅
- `config/smackerel.yaml` L27-40 — fail-loud comment ("auditable configured value
  here, not a Compose fallback"); the `:-` form named only to forbid it. ✅
- `.github/copilot-instructions.md` L221-242,L482 + `docs/Operations.md`
  DevOps-Access section — Tailnet-Edge Bind Pattern, fail-loud form, Pattern P1
  `docker exec` over Tailscale SSH, Pattern P5 host Caddy, generic placeholders. ✅
- `.github/instructions/smackerel-no-defaults.instructions.md` +
  `.github/skills/smackerel-no-defaults/SKILL.md` exist; forbidden-fallback scan
  of `deploy/compose.deploy.yml` is clean. ✅
- `./smackerel.sh check` EXIT=0; `./smackerel.sh config generate` EXIT=0 with
  `HOST_BIND_ADDRESS=127.0.0.1` at `config/generated/dev.env:75` + `test.env:75`. ✅
- **Non-042 caveat (disclosed, not force-ticked):** `./smackerel.sh test unit --go`
  full-suite exit is currently 1 from `internal/assistant` (tool-registry/scenario
  loader, committed state owned by the assistant specs) and `tests/unit/clients`
  (cross-language canary requires `node`+`dart`, not installed on this host) —
  both OUTSIDE Scope 1's change boundary; the spec-042 package itself passes
  (`ok internal/deploy 23.803s`). ⚠️

---

## Root Cause

Spec 042 was certified `done` on 2026-05-09 (full bugfix-fastlane chain). On
2026-05-25, reconciliation commit `15e1c453` reversed the original
`${HOST_BIND_ADDRESS:-127.0.0.1}` loopback-default compose form to the fail-loud
`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` form
(per Gate G028 NO-DEFAULTS / fail-loud SST, codified in
`.github/instructions/smackerel-no-defaults.instructions.md`), rewrote the active
`scopes.md` planning truth (renamed both scopes, rewrote SCN-042-001..004, added
NFR-5/NFR-6), and **reset Scope 1 + Scope 2 to `Not started`** with the intent
that the fail-loud contract be re-verified against the new form. That
re-verification was never recorded: the 26 DoD items stayed `[ ]` and the scope
statuses stayed `Not started`, while `state.json` retained the certified `done`
status. The result is a `done`-vs-`Not started` inconsistency that
`artifact-lint.sh` mechanically rejects and that OPS-002's G088 review flagged.

The shipped code already satisfies the fail-loud contract (the contract test
`internal/deploy/compose_contract_test.go` was upgraded in lockstep and is GREEN
with comprehensive adversarial coverage), so this is **pure artifact governance
drift** — the planning truth lags the shipped+tested reality.

---

## Scope

**In-scope (artifact mutations only):**

- `specs/042-tailnet-edge-bind-pattern/scopes.md` — re-tick all 26 DoD items with
  inline evidence; restore Scope 1 + Scope 2 to `Done`; update the Active Scope
  Inventory table + reconciliation note.
- `specs/042-tailnet-edge-bind-pattern/state.json` — add top-level `certifiedAt`
  + `certifiedBy`; update `lastUpdatedAt` + `certification.certifiedAt`; append a
  `bubbles.spec-review` CURRENT executionHistory entry; align `scopeProgress`
  names.
- `specs/042-tailnet-edge-bind-pattern/report.md` — append the Reconciliation
  Recertification section (Validation Evidence + Audit Evidence); wrap historical
  round evidence in sanctioned `bubbles:evidence-legitimacy-skip` markers; reword
  one narrative-trigger table row.
- `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-001-scope-status-reconciliation/**`
  — this packet (8 artifacts).

**Out-of-scope (NOT touched):**

- `specs/042-tailnet-edge-bind-pattern/spec.md` — NOT touched (would re-trigger
  G088 with a fresh post-cert planning edit).
- `specs/042-tailnet-edge-bind-pattern/design.md` — NOT touched.
- `specs/042-tailnet-edge-bind-pattern/scenario-manifest.json` — NOT touched.
- `specs/042-tailnet-edge-bind-pattern/uservalidation.md` — NOT touched.
- Any `.go`, `.py`, `.yaml`, `.sql`, `.sh`, `.ts`, `.tsx` source under `cmd/`,
  `internal/`, `ml/`, `tests/`, `scripts/`, `web/`, `config/`,
  `.github/workflows/`, `deploy/`, `smackerel.sh` — the fail-loud contract is
  already shipped; zero source change.
- `.github/bubbles/**` and the external v6/v7-upgrade files — framework-managed,
  immutable per repo policy.
- `internal/assistant/**`, `tests/unit/clients/**` — the disclosed non-042 unit
  suite reds belong to other specs; not this packet's surface.
- Any other spec folder.

---

## Acceptance

- `grep -cE '^- \[ \] ' specs/042-tailnet-edge-bind-pattern/scopes.md` returns
  **0** (no unchecked DoD items).
- Both Scope 1 and Scope 2 carry `**Status:** Done`; the Active Scope Inventory
  table shows both `Done`.
- `specs/042-tailnet-edge-bind-pattern/state.json` declares top-level
  `certifiedAt: 2026-06-06T17:30:00Z` and a `bubbles.spec-review` CURRENT
  executionHistory entry.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern`
  returns **PASSED**.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-001-scope-status-reconciliation`
  returns **PASSED**.
- `bash .github/bubbles/scripts/post-cert-spec-edit-guard.sh specs/042-tailnet-edge-bind-pattern`:
  committed-history check is clean (`git log --since=certifiedAt` over the
  planning files returns nothing); the only pending entry is the uncommitted
  `scopes.md` edit, which clears once the parent batch-commits before
  `certifiedAt`.
- No DoD item was force-ticked: the single whole-repo-gate item that is red
  (`./smackerel.sh test unit --go` full suite) is disclosed as a non-042 caveat
  rather than ticked as `EXIT=0`.
