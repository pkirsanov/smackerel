# Framework Improvements — Delivered Record

> Durable published record of framework improvements whose temporary
> `improvements/IMP-NNN-*.md` execution packets have been **delivered and
> removed**.
>
> Per [`Framework_Convergence_Health.md`](Framework_Convergence_Health.md):
> *"Execution packets created temporarily in the Bubbles source repo must be
> harvested into durable docs and framework assets, then removed."* The
> `improvements/` tree is a scratch surface (git-ignored in downstream repos);
> an IMP packet is deleted once its work ships. This file is where that work's
> substance is harvested so it survives the packet's deletion.
>
> **Process rule:** an improvement is appended here (or into another durable doc
> linked below) **before** its `improvements/IMP-NNN-*.md` packet is deleted.
> The `CHANGELOG.md` `[Unreleased]` entry records the *change*; this record
> preserves *what the improvement delivered, the framework assets it introduced,
> and where its behavior is durably documented*.
>
> **Audience:** maintainer. **Roll-up:** linked from
> [`governance-index.md`](governance-index.md).

---

## Delivered Improvements

### IMP-006 — `bubbles.journey` full-stack tutorial + internal-correctness verification

- **Problem:** guided-journey walkthroughs recorded only a user-facing verdict
  per step, so a green UI could pass over a sick trace or an un-persisted write
  (J1); the walk was not framed as a tutorial (J2); and the dev/validate-drive
  vs operate/prod-read-only plane boundary was ambiguous (J3).
- **Delivered:** additive, behavior-preserving hardening of the journey agent
  contract (SCOPE-1–6, applied to `agents/bubbles.journey.agent.md`,
  `prompts/bubbles.journey.prompt.md`, `docs/recipes/guided-journey.md`):
  four-layer per-step verification (UI + API + telemetry + data via the
  validate-plane `observability-endpoint-resolve.sh` resolver), NON-NEGOTIABLE
  plane governance (INV-12 — drive/mutate validate-plane only, prod read-only),
  tutorial posture + replayable `uservalidation.md` walkthrough (G057 human-
  acceptance boundary preserved), three added Skills-First pointers, and an
  expanded Output Contract carrying the four evidence lanes + a dual
  friction/internal verdict with a Hidden Defects (UI-passed, backend-failed →
  route to `bubbles.bug`) section.
- **Durable home:** [`docs/recipes/guided-journey.md`](recipes/guided-journey.md)
  + the journey agent contract + this entry.
- **Scope boundary:** SCOPE-7 (downstream propagation) is record-only /
  operator-gated (`install.sh --local-source` re-sync), not executed by the IMP.
  Workflow wiring (a `journey` phase / `journey-refinement` mode /
  `experientialFriction` scoring) is IMP-001's domain — IMP-006 is independently
  landable and does NOT require it (G125: no auto-mutation of `bubbles/*` config).
- **Status:** delivered (SCOPE-1–6 applied, SCOPE-7 record-only), green.

### IMP-017 — Template case-collision fix

- **Problem:** a case-only filename collision (`AGENTS.md` vs `templates/AGENTS.md.tmpl`)
  let the installer scaffold the wrong-case root `AGENTS.md` on case-insensitive
  filesystems.
- **Delivered:** removed the case-colliding `templates/AGENTS.md.tmpl` index
  entry; corrected `install.sh` to stop scaffolding the wrong-case root
  `AGENTS.md`; added `bubbles/scripts/case-collision-guard.sh` + a hermetic
  selftest wired into `framework-validate`.
- **Durable home:** the shipped guard + selftest (self-documenting), this entry,
  and the `CHANGELOG.md` `[Unreleased]` entry.
- **Status:** delivered, green under `framework-validate`. SCOPE-4 (authoring a
  *correct* starter-`AGENTS.md` guardrails scaffold) is explicitly a separate
  future proposal, out of scope for the fix.

### IMP-018 — Cross-platform (GNU/BSD) shell-portability guard

- **Problem:** framework and downstream shell must run identically on WSL/Linux
  (GNU coreutils) and macOS (BSD userland); GNU-only forms (`sed -i`, `date -d`,
  `paste` without operand, 3-arg `awk match`, …) silently break on BSD.
- **Delivered:** `bubbles/scripts/macos-portability-guard.sh` — a 13-class
  GNU/BSD pitfall lint — plus its hermetic selftest. Helper-aware, honors a
  `# portable-ok:<reason>` pragma, scans a caller-supplied surface (never the
  framework's own `bubbles/scripts/`), wired into `framework-validate` via its
  selftest.
- **Durable home:** skill [`bubbles-cross-platform-shell`](../skills/bubbles-cross-platform-shell/SKILL.md)
  (§ *Mechanical Enforcement*) + instruction
  [`wsl-macos-compatibility.instructions.md`](../instructions/wsl-macos-compatibility.instructions.md)
  + this entry.
- **Status:** delivered, green.

### IMP-019 — Direct authorized workflow runners

- **Problem:** workflow/goal/iterate/sprint runners needed direct, authorized
  execution routing without an extra dispatch hop.
- **Delivered:** the direct authorized workflow-runner control-plane / routing
  changes (shipped v7.19.0–v7.19.2).
- **Durable home:** the v7.19.x `CHANGELOG.md` entries, the shipped
  control-plane / routing changes in the workflow/goal/iterate/sprint agents,
  and this entry.
- **Status:** delivered.

### IMP-023 — Artifact-writer shared-state lease + guard (session-aware runtime coordination)

- **Problem:** two live writers to a parent-owned `state.json` /
  `scenario-manifest.json` / `spec.md` / `design.md` could silently collide;
  the framework must refuse the second writer, not "reconcile" by appending
  evidence (the Feature-010 anti-pattern).
- **Delivered:** a parent-owned shared-state writer lease + guard mechanizing the
  IMP-004 SCOPE-2 contract. `bubbles/scripts/runtime-leases.sh` gains
  `writer-acquire` + `writer-guard` (role-aware: a child scope is refused when it
  writes parent-owned shared state; a parent orchestrator may) and emits a
  machine-parseable `writer-lease-refusal result=blocked reason=… route=…
  remediation=…` structured envelope. Extended `runtime-lease-selftest.sh`. Agent
  wiring: `scope-workflow.md` (acquire-before-mutate for `parallelScopes=dag`) +
  `workflow-execution-loops.md` (the sequential-only shared-state rule is now
  mechanized).
- **Durable home:** [`docs/issues/session-aware-runtime-coordination.md`](issues/session-aware-runtime-coordination.md)
  + [`docs/recipes/runtime-coordination.md`](recipes/runtime-coordination.md)
  + this entry.
- **Status:** delivered (SCOPE-1–7), green.

### IMP-025 — Fail-loud multi-root agent/repository binding

- **Problem:** in a multi-root workspace the same agent name exists under every
  root, and nothing asserts that the selected agent's source repo matches the
  repo being edited; a foreign-root agent could silently mutate another repo.
- **Delivered:** fail-loud agent↔repo binding across six scopes.
  `bubbles/scripts/repo-binding-preflight.sh` + selftest (T1–T8b, incl. per-repo
  MCP-id slug-derivation compatibility); `install.sh` stamps a repo-relative
  `targetRepoSlug` marker into `.github/bubbles/.install-source.json`;
  continuation/handoff envelopes carry a `provenance` block (`repositoryRoot` /
  `agentSourceRoot` / `frameworkVersion`) with resume re-validation
  (`skills/bubbles-result-envelope`, handoff agent, `workflow-delegation-core.md`).
- **Durable home:** [`docs/guides/AI_ENVIRONMENT.md`](guides/AI_ENVIRONMENT.md)
  (*Multi-Root Workspaces*) + [`docs/MCP.md`](MCP.md) + this entry.
- **Design note:** SCOPE-2's cosmetic per-repo picker label was declined — it
  breaks the managed-file byte-identity integrity invariant (the
  `install-provenance` selftest's "installed bytes match canonical source" /
  BUG-009 check). Multi-root disambiguation is met mechanically (unique MCP id +
  preflight + marker), which is stronger than a cosmetic suffix.
- **Status:** delivered (SCOPE-1–6), green.
