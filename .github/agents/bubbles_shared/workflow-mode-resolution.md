## Workflow Mode Resolution

Use this module as the canonical mode-selection reference for `bubbles.workflow`.

### Mode Selection Decision Tree

Use this table to select the correct mode based on the execution goal.

| Your Goal | Mode | Ceiling | Phases |
|-----------|------|---------|--------|
| Improve spec/scope quality only (no code changes) | `spec-scope-hardening` | `specs_hardened` | select -> bootstrap -> harden -> docs -> validate -> audit -> finalize |
| Find and fix code issues against existing specs | `harden-to-doc` | `done` | select -> bootstrap -> validate -> harden -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> chaos -> validate -> audit -> docs -> finalize |
| Fix performance, infra, config, reliability, security issues | `stabilize-to-doc` | `done` | select -> bootstrap -> validate -> stabilize -> devops -> implement -> test -> regression -> simplify -> security -> chaos -> validate -> audit -> docs -> finalize |
| Close design-vs-code gaps and fix | `gaps-to-doc` | `done` | select -> bootstrap -> validate -> gaps -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> chaos -> validate -> audit -> docs -> finalize |
| Full quality sweep (harden + gaps + fix + test) | `harden-gaps-to-doc` | `done` | select -> bootstrap -> validate -> harden -> gaps -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> chaos -> validate -> audit -> docs -> finalize |
| Full end-to-end delivery from scratch | `full-delivery` | `done` | select -> bootstrap -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> docs -> validate -> audit -> chaos -> finalize |
| Maximum-assurance delivery until everything is truly green | `delivery-lockdown` | `done` | [repeat until certified done: optional analyze/ux/design/plan prelude -> bootstrap -> implement -> test -> regression -> simplify -> gaps -> harden -> stabilize -> security -> validate -> audit -> chaos -> docs] -> finalize |
| Find highest-value work and deliver it | `value-first-e2e-batch` | `done` | discover -> select -> bootstrap -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> docs -> validate -> audit -> chaos -> finalize |
| Fix a specific bug | `bugfix-fastlane` | `done` | select -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> validate -> audit -> finalize |
| Run chaos probes and fix what breaks | `chaos-hardening` | `done` | select -> bootstrap -> chaos -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> validate -> audit -> docs -> finalize |
| Run security review and fix what it finds | `security-to-doc` | `done` | select -> bootstrap -> security -> implement -> test -> regression -> simplify -> stabilize -> devops -> chaos -> validate -> audit -> docs -> finalize |
| Run regression scan and fix what it finds | `regression-to-doc` | `done` | select -> bootstrap -> regression -> implement -> test -> simplify -> stabilize -> devops -> security -> chaos -> validate -> audit -> docs -> finalize |
| Run tests, then quality chain | `test-to-doc` | `done` | select -> bootstrap -> test -> validate -> audit -> docs -> finalize |
| Run chaos, then quality chain | `chaos-to-doc` | `done` | select -> chaos -> validate -> audit -> docs -> finalize |
| Validate claims, reconcile stale state, then deliver | `reconcile-to-doc` | `done` | [one-shot spec-review default] -> select -> bootstrap -> validate -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> validate -> audit -> chaos -> docs -> finalize |
| Update docs only (no code changes) | `docs-only` | `docs_updated` | select -> docs -> validate -> audit -> finalize |
| Validate only | `validate-only` | `validated` | select -> validate -> finalize |
| Audit only | `audit-only` | `validated` | select -> audit -> finalize |
| Final validation + audit + docs | `validate-to-doc` | `validated` | select -> validate -> audit -> docs -> finalize |
| Resume from saved state | `resume-only` | `in_progress` | select -> finalize |
| Discover requirements, design UX, then deliver | `product-to-delivery` | `done` | analyze -> select -> bootstrap -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> docs -> validate -> audit -> chaos -> finalize |
| Analyze existing feature, reconcile stale claims, then improve competitively | `improve-existing` | `done` | analyze -> [one-shot spec-review default] -> select -> validate -> harden -> gaps -> implement -> test -> regression -> simplify -> stabilize -> devops -> security -> validate -> audit -> chaos -> docs -> finalize |
| Simplify an existing implementation, prove behavior still works, then sync docs | `simplify-to-doc` | `done` | select -> simplify -> test -> validate -> audit -> docs -> finalize |
| Retro-target the hotspot mess, simplify first, then run the full quality crew | `retro-quality-sweep` | `done` | select -> retro -> simplify -> harden -> gaps -> implement -> test -> regression -> stabilize -> devops -> security -> validate -> audit -> docs -> finalize |
| Randomized adversarial quality probing across specs | `stochastic-quality-sweep` | `done` | [N rounds: random spec + random trigger -> dispatch trigger-owned child workflow] -> sweep summary |
| Priority-driven iterative work execution (N iterations or time-bounded) | `iterate` | `done` | [N iterations: pick highest-priority work -> auto-select mode -> execute full delivery cycle] -> finalize |

### Invocation Syntax

`bubbles.workflow` remains the execution orchestrator. Use:

```text
/bubbles.workflow <spec-targets> mode: <mode-name>
```

Minimal syntax anchors retained in the workflow shell:

```text
/bubbles.workflow <spec-targets> mode: <mode-name>
/bubbles.workflow <spec-targets> mode: delivery-lockdown
/bubbles.workflow <spec-targets> mode: stochastic-quality-sweep maxRounds: 10
/bubbles.workflow <spec-targets> mode: iterate iterations: 3
```

### Invocation Examples

```text
/bubbles.workflow 011-037 mode: harden-to-doc
/bubbles.workflow 027 mode: gaps-to-doc
/bubbles.workflow 011,012,019 mode: harden-gaps-to-doc
/bubbles.workflow 027 mode: reconcile-to-doc
/bubbles.workflow 011-037 mode: improve-existing
/bubbles.workflow 011-037 mode: harden-to-doc batch: false
/bubbles.workflow 042 mode: full-delivery
/bubbles.workflow 011-037 mode: full-delivery strict: true
/bubbles.workflow 042 mode: delivery-lockdown improvementPrelude: analyze-ux-design-plan improvementPreludeRounds: 2
/bubbles.workflow 042 mode: improve-existing specReview: once-before-implement
/bubbles.workflow mode: value-first-e2e-batch
/bubbles.workflow 011-037 mode: spec-scope-hardening
/bubbles.workflow specs/027-feature/bugs/BUG-001 mode: bugfix-fastlane
/bubbles.workflow specs/050-new-feature mode: product-to-delivery
/bubbles.workflow specs/050-new-feature mode: product-to-delivery socratic: true socraticQuestions: 4
/bubbles.workflow specs/050-new-feature mode: product-to-delivery grillMode: required-on-ambiguity tdd: true
/bubbles.workflow specs/050-new-feature mode: spec-scope-hardening analyze: true
/bubbles.workflow specs/019-visual-page-builder mode: improve-existing
/bubbles.workflow specs/019-visual-page-builder mode: product-to-delivery
/bubbles.workflow specs/019-visual-page-builder mode: simplify-to-doc
/bubbles.workflow specs/019-visual-page-builder mode: retro-quality-sweep
/bubbles.workflow mode: stochastic-quality-sweep
/bubbles.workflow 042 mode: full-delivery gitIsolation: true autoCommit: scope
/bubbles.workflow 011-037 mode: stochastic-quality-sweep
/bubbles.workflow mode: stochastic-quality-sweep minutes: 60 triggerAgents: chaos,validate
/bubbles.workflow 011,027,037 mode: stochastic-quality-sweep maxRounds: 5 triggerAgents: harden,gaps,simplify
/bubbles.workflow mode: iterate
/bubbles.workflow mode: iterate iterations: 5
/bubbles.workflow mode: iterate minutes: 120
/bubbles.workflow mode: iterate type: improve
/bubbles.workflow mode: iterate type: chaos
```

### Status Ceiling Warning

When a selected mode has `statusCeiling` below `done`, `bubbles.workflow` must warn the user if the request language implies full completion. Delivery-capable modes should be recommended instead of silently proceeding with a lower ceiling.

### Reciprocal Status Ceiling Warning (Planning-Only Intent)

When the user's request implies planning-only intent (contains "plan", "planning", "design", "scope", "analyze", "create specs", "create bugs" WITHOUT "implement", "build", "fix", "deliver"), `bubbles.workflow` MUST:

1. If the selected mode has `statusCeiling: done` → warn and suggest planning-capped alternative
2. Example warning: "Your request 'plan scope 027' implies planning-only work. Selected mode 'full-delivery' includes implementation. Switching to spec-scope-hardening mode."
3. If user explicitly provides `mode: full-delivery` alongside planning language → respect the explicit mode but log the intent mismatch
4. If no explicit `mode:` is provided → auto-select the planning-capped mode

**Planning-intent keywords:** plan, planning, design, scope, analyze, create specs, create bugs, convert findings to specs, planning cycle, planning workflow
**Delivery-intent keywords:** implement, build, fix, deliver, ship, deploy, full delivery, bugfix

### Status Ceiling Warning Details

When resolving mode in Phase 0, `bubbles.workflow` MUST check whether the user's requested outcome conflicts with the selected mode's `statusCeiling`.

- If the user's prompt contains words like `complete`, `implement`, `fix`, `test`, or `done` and the selected mode has `statusCeiling` below `done`, warn before starting and suggest a delivery-capable mode instead of silently proceeding.
- Modes that cannot reach `done`: `spec-scope-hardening` (`specs_hardened`), `docs-only` (`docs_updated`), `validate-only` (`validated`), `audit-only` (`validated`), `validate-to-doc` (`validated`), `resume-only` (`in_progress`).
- Modes that can reach `done`: all delivery modes, including `full-delivery`, `delivery-lockdown`, `bugfix-fastlane`, `product-to-delivery`, `stochastic-quality-sweep`, and `iterate`.