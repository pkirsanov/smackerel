# <img src="../icons/bubbles-glasses.svg" width="28"> Bubbles Cheat Sheet

<!-- GENERATED:FRAMEWORK_STATS_SUMMARY_START -->
> **34 Agents · 60 Gates · 29 Workflow Modes · 25 Phases**
<!-- GENERATED:FRAMEWORK_STATS_SUMMARY_END -->
>
> *"It Ain't Rocket Appliances, But It Works."*

---

## <img src="../icons/bubbles-glasses.svg" width="32"> Start Here — Universal Entry Point

| Icon | Agent | Alias | Role | Quote |
|:----:|-------|-------|------|-------|
| <img src="../icons/bubbles-glasses.svg" width="28"> | `bubbles.workflow` | Bubbles | **Universal entry point.** Just describe what you want. Accepts plain English, structured commands, or "continue" — resolves intent via `super`, picks work via `iterate`, and drives all phases to completion. | *"Decent. I can see how all this fits together. Just tell me what you need."* |
| <img src="../icons/lahey-badge.svg" width="28"> | `bubbles.super` | Mr. Lahey | Framework ops & advice. NLP resolver, command generator, health checks, framework validation, release hygiene, hooks, gates, upgrades, and repo-readiness guidance. Workflow delegates to him automatically for vague input. | *"I'm the trailer park supervisor. I'll tell you the next move."* |

## <img src="../icons/jacob-hardhat.svg" width="32"> Orchestrators

| Icon | Agent | Alias | Role | Quote |
|:----:|-------|-------|------|-------|
| <img src="../icons/jacob-hardhat.svg" width="28"> | `bubbles.iterate` | Jacob | Single-iteration work picker. Chooses the next executable slice and runs the right specialist chain. Also accepts plain English via `super` delegation. | *"I'll do whatever you need, Julian."* |
| <img src="../icons/cory-cap.svg" width="28"> | `bubbles.bug` | Cory | Bug orchestrator. Reproduces the issue, packets the work, dispatches the right owners, and keeps going until the bug is actually closed. | *"I didn't wanna find it, but... there it is."* |

## <img src="../icons/julian-glass.svg" width="32"> Owners & Executors

| Icon | Agent | Alias | Role | Quote |
|:----:|-------|-------|------|-------|
| <img src="../icons/ray-lawnchair.svg" width="28"> | `bubbles.analyst` | Ray | Defines business requirements and the observable truth the rest of the system must satisfy. | *"Sometimes she goes, sometimes she doesn't."* |
| <img src="../icons/lucy-mirror.svg" width="28"> | `bubbles.ux` | Lucy | Owns UX sections and interaction design when the feature needs them. | *"You can't just slap things together and call it done."* |
| <img src="../icons/sarah-clipboard.svg" width="28"> | `bubbles.design` | Sarah | Turns loose requirements into a technical shape that can actually survive implementation. | *"Let's get this organized before anybody breaks it."* |
| <img src="../icons/barb-keys.svg" width="28"> | `bubbles.plan` | Barb Lahey | Owns scopes, DoD, Test Plan structure, and scenario contracts. | *"Jim, you need a plan."* |
| <img src="../icons/julian-glass.svg" width="28"> | `bubbles.implement` | Julian | Delivers code. Every. Time. Zero drops, zero rollbacks. | *"I got work to do."* |
| <img src="../icons/trinity-notebook.svg" width="28"> | `bubbles.test` | Trinity | Grew up in chaos. Learned to verify everything independently. Trust nothing. | *"Dad, that's not how that works."* |
| <img src="../icons/jroc-mic.svg" width="28"> | `bubbles.docs` | J-Roc | Publishes the managed-doc truth and cleans out stale paperwork before closeout. | *"Know what I'm sayin'? Publish the truth."* |
| <img src="../icons/ricky-dynamite.svg" width="28"> | `bubbles.chaos` | Ricky | Breaks things in ways nobody could predict. Worst case Ontario, something catches fire. | *"It's not rocket appliances."* |
| <img src="../icons/donny-ducttape.svg" width="28"> | `bubbles.simplify` | Donny | Reduces needless complexity without weakening the behavior contract. | *"Just tape it up and move on."* |
| <img src="../icons/tommy-rack.svg" width="28"> | `bubbles.devops` | Tommy Bean | Owns CI/CD, build, deployment, monitoring, and observability execution once operational work is identified. | *"Get the rack humming and keep the park online."* |
| <img src="../icons/sebastian-guitar.svg" width="28"> | `bubbles.cinematic-designer` | Sebastian Bach | Premium UI implementation when the surface needs flagship treatment, not default sludge. | *"I was in Skid Row!"* |

## <img src="../icons/ted-badge.svg" width="32"> Diagnostic & Certification Routing

| Icon | Agent | Alias | Role | Quote |
|:----:|-------|-------|------|-------|
| <img src="../icons/randy-cheeseburger.svg" width="28"> | `bubbles.validate` | Randy | Owns certification state, checks gates, and can reopen work only through concrete packets and evidence. | *"Mr. Lahey, the tests aren't passing!"* |
| <img src="../icons/ted-badge.svg" width="28"> | `bubbles.audit` | Ted Johnson | Final compliance cop. Certifies, routes rework, and does not implement fixes. | *"This is an official audit now."* |
| <img src="../icons/private-dancer-lamp.svg" width="28"> | `bubbles.grill` | Leslie Dancer | Pressure-tests ideas and approval paths before the wrong work starts. | *"Let's get it under the light and see if it survives."* |
| <img src="../icons/george-green-badge.svg" width="28"> | `bubbles.clarify` | George Green | Calls out ambiguity and routes the correct owner instead of patching foreign artifacts himself. | *"What in the f— is going on here?"* |
| <img src="../icons/conky-puppet.svg" width="28"> | `bubbles.harden` | Conky | Finds the weak spots and packets the follow-up work. | *"Why don't you go pave your cave?"* |
| <img src="../icons/phil-collins-baam.svg" width="28"> | `bubbles.gaps` | Phil Collins | Finds missing behavior, evidence, and coverage, then routes the repair. | *"What are ya lookin' at my gut fer?"* |
| <img src="../icons/bill-wrench.svg" width="28"> | `bubbles.stabilize` | Shitty Bill | Surfaces reliability issues and routes the correct owner. Does not fix inline. | *"..."* |
| <img src="../icons/steve-french-paw.svg" width="28"> | `bubbles.regression` | Steve French | Guards the existing territory against regressions and cross-spec collisions. | *"Something's prowlin' around in the code, boys."* |
| <img src="../icons/cyrus-sunglasses.svg" width="28"> | `bubbles.security` | Cyrus | Finds threats, packets the fixes, and refuses greasy shortcuts. | *"F*** off, I got work to do."* |
| <img src="../icons/green-bastard-outline.svg" width="28"> | `bubbles.code-review` | Green Bastard | Engineering-only review surface. Detects problems and priorities without pretending to be the owner. | *"From parts unknown, I can smell what's broken in the code."* |
| <img src="../icons/orangie-fishbowl.svg" width="28"> | `bubbles.system-review` | Orangie | Holistic product/runtime/trust reviewer. Finds what needs attention, then routes it. | *"Orangie sees everything. He's not dead, he's just... reviewing."* |
| <img src="../icons/gary-laser-eyes.svg" width="28"> | `bubbles.spec-review` | Gary Laser Eyes | Audits artifact trust and freshness before maintenance work relies on stale truth. | *"Gary can see right through it, boys."* |

### Ownership Quick Reference

| Artifact | Owner | Notes |
|----------|-------|-------|
| `spec.md` business requirements | `bubbles.analyst` | `bubbles.ux` may update UX sections only |
| `design.md` | `bubbles.design` | Technical design owner |
| `scopes.md` / planning structure | `bubbles.plan` | Gherkin, Test Plan, DoD, `uservalidation.md`, `scenario-manifest.json` |
| `state.json.certification.*` | `bubbles.validate` | Validate-owned authority only |
| Route-required findings | owning specialist | Diagnostic and certification agents packetize; they do not self-author foreign artifacts |

## <img src="../icons/camera-crew.svg" width="32"> Utilities

| Icon | Agent | Alias | Role | Quote |
|:----:|-------|-------|------|-------|
| <img src="../icons/camera-crew.svg" width="28"> | `bubbles.status` | Camera Crew | Documentary crew. Observes. Reports. Never interferes. Read-only. | *(just watches silently)* |
| <img src="../icons/camera-crew.svg" width="28"> | `bubbles.recap` | Talking Head | The interview segment. Gives the fast narrative version of this session: what happened, what is in progress, and which workflow should safely continue the work. | *"So basically what happened was..."* |
| <img src="../icons/lahey-bottle.svg" width="28"> | `bubbles.retro` | Jim Lahey (Bottle) | The liquor-fueled retrospective. Analyzes velocity, gate health, deep code hotspots (bug magnets, co-change coupling, bus factor, churn trends), and shipping patterns across sessions. | *"The liquor helps me see the patterns, Randy."* |
| <img src="../icons/trevor-handoff.svg" width="28"> | `bubbles.handoff` | Trevor | Runs the handoff package to the next shift. Carries things. | *"Here, take this. I gotta go."* |
| <img src="../icons/cory-trevor-smokes.svg" width="28"> | `bubbles.setup` | Cory & Trevor | The errand duo. Set up or refresh the framework layer. Do the prep. | *"Smokes, let's go."* |
| <img src="../icons/t-cap.svg" width="28"> | `bubbles.commands` | T | J-Roc's right hand. Makes the registry. Always there. | *"True."* |
| <img src="../icons/sam-binoculars.svg" width="28"> | `bubbles.create-skill` | Sam Losco | Packages weird but useful specializations into something you can actually use again later. | *"I used to be a vet, you know. I got specialties."* |

---

## <img src="../icons/ricky-dynamite.svg" width="32"> Command Aliases

| Alias | Maps To | Quote |
|-------|---------|-------|
| `sunnyvale pull-the-strings` | `bubbles.workflow` | *"Bubbles is pulling the strings, boys."* |
| `sunnyvale under-the-light` | `bubbles.grill` | *"Let's get it under the light and see if it survives."* |
| `sunnyvale private-dancer` | `bubbles.grill` | *"You want answers? Put it under the light."* |
| `sunnyvale worst-case-ontario` | `bubbles.chaos` | *"Worst case Ontario, something breaks"* |
| `sunnyvale by-the-book` | `bubbles.audit --strict` | *"This is by the book now."* |
| `sunnyvale get-two-birds-stoned` | `bubbles.implement` + `bubbles.test` | *"Get two birds stoned at once"* |
| `sunnyvale i-got-work-to-do` | `bubbles.implement` | *"I got work to do."* |
| `sunnyvale smokes-lets-go` | `bubbles.setup` | *"Smokes, let's go."* |
| `sunnyvale know-what-im-sayin` | `bubbles.docs` | *"Know what I'm sayin'? Publish the truth."* |
| `sunnyvale somethings-fucky` | `bubbles.validate` | *"Something's fucky"* |
| `sunnyvale mans-gotta-eat` | `bubbles.validate` | *"A man's gotta eat, Julian"* |
| `sunnyvale way-she-goes` | `bubbles.analyst` | *"Way she goes, boys."* |
| `sunnyvale peanut-butter-and-jam` | `bubbles.gaps` | *"BAAAAM! Peanut butter and JAAAAM!"* |
| `sunnyvale safety-always-off` | `bubbles.security` | *"Safety... always off"* |
| `sunnyvale somethings-prowlin` | `bubbles.regression` | *"Something's prowlin' around in the code, boys."* |
| `sunnyvale roll-camera` | `bubbles.status` | *(camera keeps rolling)* |
| `sunnyvale greasy` | `bubbles.harden` | *"That's greasy, boys."* |
| `sunnyvale pave-your-cave` | `bubbles.harden` | *"Why don't you go pave your cave?"* |
| `sunnyvale supply-and-command` | `bubbles.plan` | *"It's supply and command, Julian"* |
| `sunnyvale jim-needs-a-plan` | `bubbles.plan` | *"Jim, you need a plan."* |
| `sunnyvale water-under-the-fridge` | `bubbles.simplify` | *"Just tape it up and move on."* |
| `sunnyvale laser-eyes` | `bubbles.spec-review` | *"Gary can see right through it, boys. That spec expired three refactors ago."* |
| `sunnyvale have-a-good-one` | `bubbles.handoff` | *"Here, take this. I gotta go."* |
| `sunnyvale skid-row` | `bubbles.cinematic-designer` | *"I was in Skid Row!"* |
| `sunnyvale the-super` | `bubbles.super` | *"I'm the trailer park supervisor."* |
| `sunnyvale parts-unknown` | `bubbles.code-review` | *"From parts unknown!"* |
| `sunnyvale keep-the-park-online` | `bubbles.devops` | *"Get the rack humming and keep the park online."* |
| `sunnyvale whole-show` | `bubbles.system-review` | *"Orangie sees everything. He's not dead, he's just... reviewing."* |
| `sunnyvale not-how-that-works` | `bubbles.test` | *"Dad, that's not how that works."* |
| `sunnyvale lets-get-organized` | `bubbles.design` | *"Let's get this organized."* |
| `sunnyvale whats-going-on-here` | `bubbles.clarify` | *"What in the f— is going on here?"* |
| `sunnyvale nice-kitty` | `bubbles.bug` | *"That's a nice f\*\*\*ing kitty right there."* |
| `sunnyvale just-fixes` | `bubbles.stabilize` | *"..." (Bill spots the problem and points at it)* |
| `sunnyvale used-to-be-a-vet` | `bubbles.create-skill` | *"I used to be a vet, you know."* |
| `sunnyvale true` | `bubbles.commands` | *"True."* |
| `sunnyvale ill-do-whatever` | `bubbles.iterate` | *"I'll do whatever you need, Julian."* |
| `sunnyvale catch-me-up` | `bubbles.recap` | *"So basically what happened was..."* |
| `sunnyvale i-am-the-liquor` | `bubbles.retro` | *"The liquor helps me see the patterns, Randy."* |
| `sunnyvale see-the-patterns` | `bubbles.retro` | *"I AM the liquor."* |
| `sunnyvale wheres-the-bodies` | `bubbles.retro hotspots` | *"The liquor knows where the bodies are buried, Randy."* |
| `sunnyvale whos-driving` | `bubbles.retro busfactor` | *"Somebody's gotta know how to drive this thing."* |
| `sunnyvale tangled-up` | `bubbles.retro coupling` | *"It's all tangled up like Christmas lights, Randy."* |
| `sunnyvale liquor-then-tape` | `retro-to-simplify` | *"The liquor shows me the problems. Donny tapes them up."* |
| `sunnyvale liquor-then-harden` | `retro-to-harden` | *"The liquor shows me the weak spots. Harden up, boys."* |
| `sunnyvale liquor-then-sweep` | `retro-quality-sweep` | *"The liquor finds the mess. Then the whole crew sweeps it clean."* |
| `sunnyvale liquor-then-look` | `retro-to-review` | *"The liquor shows me where to look. Green Bastard tells me what's broken."* |
| `sunnyvale cant-just-slap` | `bubbles.ux` | *"You can't just slap things together."* |
| `sunnyvale same-lot-new-trailer` | `product-to-delivery` (with existing impl) | *"Same lot, boys. New trailer."* |

---

## <img src="../icons/julian-glass.svg" width="32"> Workflow Modes

| Mode | Alias | What It Does |
|------|-------|-------------|
| `value-first-e2e-batch` | boys-plan | Auto-discover highest-value work, full delivery pipeline |
| `full-delivery` | full-send | Standard complete delivery — the default |
| `full-delivery` (with strict tag) | clean-and-sober | Strict enforcement, no blocked continuation |
| `delivery-lockdown` | no-loose-ends | Keep looping through tests, quality sweep, validation, and bug closure until truly green |
| `devops-to-doc` | keep-the-park-online | Focused DevOps execution + operational verification + docs sync |
| `simplify-to-doc` | strip-it-down | Simplify an existing implementation, prove it still works, then sync docs |
| `spec-review-to-doc` | laser-eyes-sweep | Audit specs for freshness, classify trust levels, produce maintenance report |
| `chaos-hardening` | shit-storm | Iterative chaos + bugfix cycles until clean |
| `bugfix-fastlane` | smash-and-grab | Fast bug closure with regression, hardening, validation, and audit |
| `validate-only` | randy-put-a-shirt-on | Run validation gates only |
| `stochastic-quality-sweep` | bottle-kids | Randomized probing — you never know where they'll hit |
| `harden-gaps-to-doc` | conky-says | Thorough pre-release sweep |
| `product-to-delivery` | freedom-35 | Full pipeline: analyst → UX → design → implement → ship |
| `docs-only` | gnome-sayin | Documentation maintenance only |
| `full-delivery` (with bootstrap) | smokes-and-setup | Repair missing artifacts, then continue delivery |
| `iterate` | keep-going | Continue scope-by-scope implementation |
| `resume-only` | resume-the-tape | Resume from last session state |
| `spec-scope-hardening` (with analyze) | whats-the-big-idea | Business analysis + UX exploration only |
| `test-to-doc` | quick-dirty | Run tests, fix failures, update docs |
| `audit-only` | open-and-shut | Run audit phase only |
| `stabilize-to-doc` | bill-fixes-it | Stability fixes → test → docs |
| `improve-existing` | survival-of-the-fitness | Analyze, harden, improve → test → docs |
| `product-to-delivery` (with existing impl) | same-lot-new-trailer | Reconcile stale artifacts, redesign an existing feature, then deliver |
| `spec-scope-hardening` | harden-up | Tighten specs and scope definitions |
| `harden-to-doc` | shit-winds-coming | Harden → test → docs |
| `gaps-to-doc` | gut-feeling | Gap analysis → test → docs |
| `chaos-to-doc` | we-broke-it | Chaos → test → docs |
| `reconcile-to-doc` | i-toad-a-so | Reconcile conflicts → test → docs |
| `validate-to-doc` | just-watching | Validate + audit + docs |
| `spec-scope-hardening` (with analyze + socratic) | smokes-and-think | Explore ideas before building — produces design artifacts, no code |
| `retro-to-simplify` | liquor-then-tape | Data-driven simplification — retro finds hotspots, then simplify fixes them |
| `retro-to-harden` | liquor-then-harden | Data-driven hardening — retro finds bug magnets, then harden targets them |
| `retro-quality-sweep` | liquor-then-sweep | Retro finds hotspots, then the deterministic quality crew cleans them up |
| `retro-to-review` | liquor-then-look | Data-driven review — retro finds risks, then code-review diagnoses them |

**Optional execution tags:** `grillMode`, `tdd` (inner-loop red→green only), `backlogExport` (off|tasks|issues), `specReview` (off|once-before-implement), `socratic`, `socraticQuestions`, `gitIsolation`, `autoCommit` (off|scope|dod), `maxScopeMinutes`, `maxDodMinutes`, `microFixes`, `crossModelReview` (off|codex|terminal)

### How Users Actually Use The Newer Planning Improvements

| Goal | What To Run | What Shows Up |
|------|-------------|---------------|
| Explore an idea before code | `/bubbles.workflow  mode: brainstorm for <idea>` | Planning artifacts only, no code |
| Improve an existing feature | `/bubbles.workflow  improve <feature>` | Objective research pass, then Design Brief + Execution Outline |
| Fix a bug in existing code | `/bubbles.workflow  fix the <bug>` | Bugfix-fastlane with objective research and reproduce/fix/verify flow |
| Keep moving the current work forward | `/bubbles.workflow  continue` | Resume active workflow or fall back to `iterate` |
| Keep going until the feature is truly green | `/bubbles.workflow  <feature> mode: delivery-lockdown` | Repeated quality/certification rounds until done or concretely blocked |
| Inspect rework and bug-magnet patterns | `/bubbles.retro  week` | Slop Tax, retries, reversions, hotspots |
| Audit framework prompt size | `bash bubbles/scripts/cli.sh lint-budget` | Instruction budget report for framework maintainers |

**Baseline workflow law:** spec/design/plan coherence, explicit Gherkin scenarios, scenario-specific test planning, and scenario-driven E2E/integration proof are required before implementation starts.

**Control-plane law:** `state.json.execution.*` records runtime claims, `state.json.certification.*` is validate-owned authority, `policySnapshot` records effective defaults with provenance, changed behavior should flow through `scenario-manifest.json` with stable `SCN-*` contracts, diagnostics and certification route foreign-owned work instead of fixing inline, and every invocation ends with a concrete result envelope.

## <img src="../icons/bubbles-glasses.svg" width="32"> TPB Vocabulary

| Term | Meaning |
|------|---------|
| `workflow-only continuation` | Recap, status, and handoff point you back to `/bubbles.workflow ...` by default instead of raw `/bubbles.implement` or `/bubbles.test` commands. |
| `continuation envelope` | Machine-readable packet from a read-only agent carrying the target, intent, preferred workflow mode, and reason for the next workflow step. |
| `scenario replay` | Validate reruns the linked live-system `SCN-*` user journeys from `scenario-manifest.json` before certification. |
| `human acceptance` | `uservalidation.md` is human-owned acceptance input. Automation findings do not toggle it. |
| `framework validation` | The framework's own self-check surface. Runs portable-surface, ownership, registry, and selftest checks before you trust a release or upgrade. |
| `release hygiene` | Source-repo ship check for Bubbles itself. Confirms framework validation passed, required release docs exist, and no stray temp or backup files are riding along. |
| `workflow run-state` | Durable per-run coordination state that makes resume, runtime reuse, and packet routing explicit instead of guesswork. |
| `typed framework event` | Structured framework log entry for gate outcomes, packet routing, lease changes, and policy provenance instead of narrative-only breadcrumbs. |
| `action risk class` | Safety label for an operation such as read-only, owned mutation, destructive mutation, external side effect, or runtime teardown. |
| `repo-readiness` | Advisory repo hygiene check for agent adoption. Useful before deep framework use, but separate from `bubbles.validate` certification. |
| `adoption profile` | Maturity-tier onboarding posture such as `foundation`, `delivery`, or `assured`. It changes guidance and early guardrail messaging, not certification rigor. |
| `release manifest` | Upstream trust bundle that states what version shipped, which profiles and interop sources are supported, what surfaces were validated, and which managed files belong to the release. |
| `install provenance` | The install record that explains where a downstream framework copy came from and whether local-source risk exists before upgrade or doctor guidance is trusted. |
| `trust preview` | Upgrade dry-run output that compares the current installed provenance and manifest to the target release before any framework-managed files are replaced. |
| `review-only interop intake` | Project-owned import path that snapshots and normalizes Claude Code, Roo Code, Cursor, or Cline assets without mutating framework-managed Bubbles files. |
| `supported interop apply` | Safe promotion path that writes only declared project-owned outputs from an interop import and falls back to proposals when a change would touch framework-owned or colliding surfaces. |
| `objective research pass` | Brownfield workflows split question generation from codebase research so the research context reports current truth instead of solution-shaped opinions. |
| `current truth` | The short section in `design.md` produced by objective research that summarizes how the code actually works today. |
| `design brief` | A short top-of-file checkpoint in `design.md` that lets humans steer the design without reviewing the whole document. |
| `execution outline` | A short top-of-file checkpoint in `scopes.md` that shows phase order, new signatures, and validation checkpoints before implementation starts. |
| `horizontal plan` | A layer-by-layer plan like DB → service → API → UI that Bubbles tries to rewrite because it hides breakage until too late. |
| `vertical slice` | An implementation sequence that gives you checkpoints through the stack instead of a giant horizontal batch. |
| `slop tax` | Rework signal measured by `bubbles.retro`: retries, reversions, reopened scopes, and fix-on-fix churn versus forward progress. |
| `instruction budget` | Prompt-size budget for agent surfaces. If the prompt gets too bloated, adherence gets worse; audit it with `bubbles lint-budget`. |
| `read the code` | Use the short planning artifacts to steer early, then read the implementation and evidence instead of trusting giant plans blindly. |

---

<!-- GENERATED:FRAMEWORK_STATS_CHEATSHEET_GATES_START -->
## <img src="../icons/lahey-badge.svg" width="32"> The 60 Gates
<!-- GENERATED:FRAMEWORK_STATS_CHEATSHEET_GATES_END -->

**Phase flow:**
`analyze` → `discover` → `select` → `spec-review` → `bootstrap` → `harden` → `gaps` → `stabilize` → `devops` → `implement` → `test` → `regression` → `simplify` → `stabilize` → `devops` → `security` → `docs` → `validate` → `audit` → `chaos` → `finalize`

| Gate | Name | What It Checks |
|------|------|---------------|
| G001 | Artifact gate | Required artifacts exist |
| G002 | Scope definition | Scope has scenarios, test plan, and DoD |
| G003 | Test integrity | Test classifications match execution reality |
| G004 | Test execution | Required tests executed and passing |
| G005 | Evidence gate | Raw execution evidence captured |
| G006 | Docs sync | Docs updated and coherent |
| G007 | Validation | Validation checks pass |
| G008 | Audit gate | Audit verdict acceptable |
| G009 | Chaos gate | Chaos rounds complete without failures |
| G010 | User validation | User validation checklist updated |
| G011 | Session gate | Session state updated for resume |
| G012 | Final promotion | All mode-required gates pass |
| G013 | Priority selection | Highest-value work selected with rationale |
| G014 | Bootstrap readiness | Design/spec/scopes ready before implementation |
| G015 | Scenario depth | Detailed Gherkin scenarios covering use cases |
| G016 | Gherkin traceability | Scenarios map to E2E tests |
| G016 | DoD E2E expansion | DoD includes E2E items |
| G018 | DoD completion | All DoD checkboxes checked |
| G019 | Sequential completion | Previous spec done before starting next |
| G020 | Anti-fabrication | Evidence is real, not fabricated |
| G021 | Detection heuristics | < 10 lines = presumed fabricated |
| G022 | Specialist execution | Required phases actually executed |
| G023 | State transition guard | Mechanical enforcement script passes |
| G024 | All scopes done | All scopes Done before spec done |
| G025 | Per-DoD evidence | Every `[x]` has inline evidence ≥ 10 lines |
| G026 | Stress for SLA | SLA scopes have stress tests |
| G027 | Phase-scope coherence | Completed phases match completed scopes |
| G028 | Implementation reality | No stubs/fakes/hardcoded data in source |
| G029 | Integration completeness | All artifacts wired into the system |
| G028 | No defaults/no fallbacks | Production code fails fast instead of masking missing inputs |
| G031 | Findings artifact update | Findings are recorded in artifacts before verdict |
| G032 | Business analysis | Actors, use cases, scenarios, wireframes are present when required |
| G033 | Design readiness | design.md + scopes.md exist before implement |
| G034 | Security scan | No vulnerabilities in changed code |
| G035 | Vertical slice | Frontend API calls match backend handlers |
| G036 | Red→green traceability | Changed behavior shows failing proof before passing proof |
| G037 | Scope size discipline | Scopes stay small, isolated, and single-outcome |
| G038 | Micro-fix containment | Failures are repaired in narrow loops before broad reruns |
| G038 | Self-healing containment | Fix loops never stack; maxDepth=1, maxRetries=3, narrowing context |
| G040 | Zero deferral language | Scope artifacts scanned for "deferred", "future scope", "out of scope", etc. — can't mark done with outstanding work |
| G041 | DoD format integrity | Prevents agents from bypassing guards by reformatting checkboxes (`- (deferred)`) or inventing scope statuses (`Deferred — Planned Improvement`) |
| G042 | Agent ownership | Foreign-owned artifacts must be routed to the owning specialist; no cross-authoring by diagnostic agents |
| G043 | Consumer trace | Renames/removals require zero stale references across all consumers |
| G044 | Regression baseline | Before/after test count comparison — previously-passing tests must still pass |
| G044 | Cross-spec regression | Done specs' tests rerun after changes — no cross-feature interference |
| G044 | Spec conflict detection | Route/table/API collisions scanned against all existing specs |
| G047 | IDOR/auth bypass | Authorization decisions must use authenticated context, not caller-controlled identity fields |
| G048 | Silent decode failure | Stored-data decode failures must be surfaced, never silently dropped |
| G021 | Evidence clone detection | DoD evidence blocks must be unique, not copy-pasted clones |
| G035 | Gateway route forwarding | Backend endpoints must have matching gateway or proxy forwarding rules |
| G051 | Test env dependency | Tests must not rely on hidden environment dependencies |
| G052 | Artifact freshness | Superseded content must be isolated from active truth; stale scope appendices cannot keep executable structure |
| G053 | Implementation delta evidence | Implementation-bearing workflows must prove runtime delivery with git-backed code-diff evidence |
| G042 | Capability delegation | Foreign-owned work must route through the registered specialist; agents must not perform cross-owner actions inline |
| G055 | Policy provenance | Active execution modes (grill, TDD, lockdown, etc.) must record value plus source in policySnapshot |
| G056 | Validate certification | Only bubbles.validate may certify completion state; other agents submit execution claims and transition requests |
| G057 | Scenario manifest | Every changed user-visible behavior must map to stable scenario IDs in scenario-manifest.json with live-system BDD coverage |
| G058 | Lockdown | Locked scenarios cannot change without grill approval plus validate-certified invalidation or replacement |
| G059 | Regression contract | Scenario-linked regression tests cannot drift, weaken, or be removed until the owning scenario contract is invalidated |
| G060 | Scenario TDD | When TDD is active, targeted failing proof must exist before the implementation is accepted as green |
| G061 | Rework packet | Route-required findings must produce structured transition or rework packets tied to scenarios, DoD items, and owning specialists |
| G042 | Owner-only remediation | Only owning planning or execution specialists may remediate owned surfaces; diagnostics and certification must route |
| G063 | Concrete result | Every agent or child-workflow invocation must end with `completed_owned`, `completed_diagnostic`, `route_required`, or `blocked` |
| G064 | Child workflow depth | Only orchestrators may invoke child workflows, and nesting depth must stay bounded |
| G040 | Pseudo-completion language | Scope and report artifacts must not contain unresolved pseudo-completion language when transitioning to done |
| G066 | Phase-claim provenance | Phase claims in completedPhaseClaims must have matching agent provenance in executionHistory; cross-phase impersonation is fabrication |
| G067 | Shared infrastructure blast radius | High-fan-out shared infrastructure changes require blast-radius planning, canary coverage, rollback, and explicit change boundaries |
| G068 | DoD-Gherkin content fidelity | DoD items must faithfully represent the behavioral claims of their source Gherkin scenarios; no silent rewrites to match delivery |
| G069 | Collateral change containment | Narrow repairs and risky refactors must declare a Change Boundary; opportunistic cleanups bundled into repair paths are blocking |
| G070 | Outcome contract | spec.md must have Outcome Contract (Intent, Success Signal, Hard Constraints, Failure Condition); validate verifies the outcome was actually achieved |

---

## <img src="../icons/trinity-notebook.svg" width="32"> Shared Skills

> *"Dad, that's not how that works. You can't just say the tests pass."*

Skills are portable procedural checklists auto-installed to every repo. They activate on specific triggers and give agents a focused playbook instead of making them piece together rules from six governance files. Think of them as the park bylaws — posted on the community board, enforced by everyone.

### The Skill Roster

| Icon | Skill | Character | Quote |
|:----:|-------|-----------|-------|
| <img src="../icons/trinity-notebook.svg" width="28"> | `bubbles-test-integrity` | Trinity | *"Dad, that's not how that works. You have to actually run them."* |
| <img src="../icons/ray-lawnchair.svg" width="28"> | `bubbles-spec-template-bdd` | Ray | *"Way she goes? No. Way the SPEC goes."* |
| <img src="../icons/barb-keys.svg" width="28"> | `bubbles-docker-lifecycle-governance` | Barb Lahey | *"Jim, there are RULES about what stays and what gets cleaned."* |
| <img src="../icons/ted-badge.svg" width="28"> | `bubbles-docker-port-standards` | Ted Johnson | *"You can't just park wherever you want. There's a system."* |
| <img src="../icons/sam-binoculars.svg" width="28"> | `bubbles-skill-authoring` | Sam Losco | *"I used to be a vet, you know. I got specialties."* |
| <img src="../icons/lahey-badge.svg" width="28"> | `bubbles-repo-readiness` | Mr. Lahey | *"Before the liquor starts talking, make sure the trailer's still standing."* |

### What Each Skill Does

#### <img src="../icons/trinity-notebook.svg" width="24"> Trinity's Field Manual — `bubbles-test-integrity`

*The smartest person in the park grew up watching adults cut corners. She doesn't.*

| | |
|---|---|
| **What It Enforces** | Tests are real. No fakes. No shortcuts. No greasy workarounds. Every Gherkin scenario gets a test. Every assertion proves the behavior the spec describes. Bug-fix regressions must use adversarial cases, not tautologies. |
| **Activates When** | Writing tests, implementing scope test plans, reviewing coverage, marking test DoD items, verifying Gherkin scenario coverage |
| **6 Quality Gates** | Gherkin coverage · No internal mocks (live categories) · No silent-pass patterns · Real assertions · Test Plan↔DoD parity · Adversarial bug-fix regression coverage |
| **The Decision Tree** | Does it execute real code? Does it assert spec behavior? Does it hit the real stack? Can it fail if the feature is broken? If any answer is no → it ain't a real test. |

**Vocabulary:**
- *"Greasy test"* — a test that passes when it shouldn't (silent-pass, no assertions, mocked internals)
- *"Tautological regression"* — a bug-fix test whose fixtures already satisfy the broken path, so it passes whether the bug exists or not
- *"Proxy assertion"* — asserting status codes or "defined" checks instead of actual behavior (*"Returns 200 or 404"* is not a test)
- *"Trinity's checklist"* — the pre-test-writing checklist: read Gherkin, read spec, identify all paths, determine categories, verify test plan
- *"Red before green"* — changed behavior must show a failing test first, then a fix

#### <img src="../icons/ray-lawnchair.svg" width="24"> The Spec Book — `bubbles-spec-template-bdd`

*Ray sees the pattern from his lawn chair. The spec template is the pattern.*

| | |
|---|---|
| **What It Enforces** | `spec.md` follows the repo template. Gherkin-style Given/When/Then scenarios. Tech-agnostic requirements. No implementation details leaking in. |
| **Activates When** | Creating `spec.md` from scratch, filling or validating content, converting free-form requirements into BDD scenarios |
| **Rules** | Preserve section order · Replace all placeholders · Scenarios are independent and testable · No languages, frameworks, or databases mentioned |

**Vocabulary:**
- *"Way the spec goes"* — the spec defines truth; tests validate it; implementation follows it
- *"Observable behavior"* — what users see and what data changes, not how the code is structured
- *"Tech leak"* — mentioning Rust, PostgreSQL, React, etc. in a spec (*"That's a tech leak, Ray."*)
- *"Outcome contract"* — the mandatory Intent / Success Signal / Hard Constraints / Failure Condition block in spec.md that defines what "done" actually means for the user
#### <img src="../icons/barb-keys.svg" width="24"> The Lot Rules — `bubbles-docker-lifecycle-governance`

*Barb ran the business side of the park. She knows what stays, what goes, and what gets cleaned up.*

| | |
|---|---|
| **What It Enforces** | Docker resource classification (persistent / ephemeral / cache), build freshness, cleanup safety, test storage isolation, stack grouping via project names and profiles |
| **Activates When** | Changing Dockerfiles, compose files, adding cleanup commands, rebuild/deploy verification, deciding persistent vs disposable storage |
| **Resource Classes** | `persistent` (survives cleanup) · `ephemeral` (test/validation, disposable) · `cache` (safe to prune) · `tooling` (debug, recreatable) · `monitoring` (preserve unless marked disposable) |

**Vocabulary:**
- *"Persistent volume"* — sacred ground. Never cleaned by default. Like Ray's bottles — you don't throw those out.
- *"Ephemeral storage"* — test and validation data. Burns clean on restart. Like Ricky's plans — gone by morning.
- *"Build freshness"* — proving the image you're running was actually built from the code you just changed, not last Tuesday's leftovers
- *"Label-aware cleanup"* — prune by labels, not `docker system prune -af`. That's burning down the park to fix one trailer.

#### <img src="../icons/ted-badge.svg" width="24"> The Port Authority — `bubbles-docker-port-standards`

*Ted Johnson doesn't care about your feelings. Ports go where the system says they go.*

| | |
|---|---|
| **What It Enforces** | The 10k Rule (project port blocks), Dual-URL Standard (internal vs external), no `localhost`, no standard-port host mappings |
| **Activates When** | Generating or modifying docker-compose, service configs, port assignments |
| **Rules** | Never map `80`, `5432`, `6379` to host · Always `127.0.0.1` not `localhost` · Internal = `http://<service>:<port>` · External = `http://127.0.0.1:<allocated_port>` |

**Vocabulary:**
- *"The 10k Rule"* — each project gets a 10,000-port block. Stay in your lot.
- *"Dual-URL"* — every service has two addresses: one inside Docker, one outside. Like having a lot number AND a mailing address.
- *"Port squatting"* — mapping `5432:5432` on the host. *"You can't just squat on standard ports, Ricky."*
- *"Localhost is a lie"* — use `127.0.0.1`. Explicit. No DNS resolution games on WSL or Docker networks.

#### <img src="../icons/sam-binoculars.svg" width="24"> Sam's Specialties — `bubbles-skill-authoring`

*Sam used to be a vet. Now he packages weird but useful specializations into something repeatable.*

| | |
|---|---|
| **What It Enforces** | Skills are project-agnostic, short, action-oriented. No hardcoded hosts/ports/URLs. No forbidden defaults. Progressive disclosure (SKILL.md for workflow, references/ for deep docs). |
| **Activates When** | Adding procedural workflows, checklists, or reusable resources under `.github/skills/` |
| **Quality Bar** | No conflict with copilot-instructions · No forbidden defaults · Routes execution through repo-standard workflows · Improves repeatability |

**Vocabulary:**
- *"Specialties"* — Sam's word for skills. Packaged know-how that doesn't expire.
- *"Progressive disclosure"* — SKILL.md is the field card; references/ are the textbooks. Don't shove the textbook into the field card.
- *"Project-agnostic"* — no repo names, no port numbers, no CLI commands. Skills travel between parks.

#### <img src="../icons/lahey-badge.svg" width="24"> The Pre-Flight Walkaround — `bubbles-repo-readiness`

*Lahey checks whether the framework can operate cleanly before anyone starts declaring victory.*

| | |
|---|---|
| **What It Enforces** | Verify-first repo hygiene for agent adoption: docs point at real commands, framework-owned surfaces are understood, automation entrypoints exist, and repo-specific expectations are written down clearly enough for agents to operate safely. |
| **Activates When** | Auditing whether a repo is ready for Bubbles-style work, checking agent onboarding hygiene, reviewing framework adoption quality, or translating vague "is this repo agent-ready?" questions into a structured checklist. |
| **Boundary Rule** | Repo-readiness is advisory framework ops. It does **not** certify feature completion and it must never replace `bubbles.validate`. |

**Vocabulary:**
- *"Walk the lot first"* — check the repo surfaces before you start heavy workflow execution.
- *"Advisory, not certification"* — repo-readiness tells you whether the park is ready for agents, not whether the feature is done.
- *"Verify-first"* — read the real commands, hooks, docs, and managed surfaces before making framework promises.

### Sunnyvale Skill Aliases

| Alias | Skill | Quote |
|-------|-------|-------|
| `sunnyvale no-greasy-tests` | `bubbles-test-integrity` | *"That test is GREASY, boys."* |
| `sunnyvale trinity-says` | `bubbles-test-integrity` | *"Dad, that's not how that works."* |
| `sunnyvale way-the-spec-goes` | `bubbles-spec-template-bdd` | *"Way she goes? No. Way the SPEC goes."* |
| `sunnyvale lot-rules` | `bubbles-docker-lifecycle-governance` | *"There are RULES about what stays and what gets cleaned."* |
| `sunnyvale no-port-squatting` | `bubbles-docker-port-standards` | *"You can't just squat on standard ports, Ricky."* |
| `sunnyvale sams-specialties` | `bubbles-skill-authoring` | *"I used to be a vet, you know."* |
| `sunnyvale walk-the-lot` | `bubbles-repo-readiness` | *"Before we start, walk the lot and see what's actually standing."* |

---

## <img src="../icons/phil-collins-baam.svg" width="32"> Fun Mode Messages (`BUBBLES_FUN_MODE=true`)

| Event | Message |
|-------|---------|
| ✅ Gate passed | *"Decent!"* |
| ✅ Scope ready | *"Looks good, boys."* |
| ❌ Gate failure | *"Something's fucky."* |
| ❌ Fabrication detected | *"That's GREASY, boys. Real greasy."* |
| ❌ Missing evidence | *"Where's your evidence? Shit hawk circling."* |
| ✅ All gates pass | *"Way she goes, boys. Way she goes."* |
| ❌ Build failed | *"Holy f\*\*\*, boys."* |
| ✅ Spec completed | *"DEEEE-CENT!"* |
| ❌ Warnings found | *"The shit winds are coming, Randy."* |
| ✅ Chaos clean | *"Worst case Ontario... nothing broke."* |
| 🟢 Regression clean | *"Steve French is purrin'. No regressions, boys."* |
| 🔴 Regression found | *"Something's prowlin' around in the code, boys."* |
| 🔍 Deep hotspot analysis | *"The liquor knows where the bodies are buried, Randy."* |
| 🔍 Co-change coupling detected | *"It's all tangled up like Christmas lights, Randy."* |
| 🔍 Bus factor risk | *"Somebody's gotta know how to drive this thing."* |
| 🔴 Bug magnet file | *"That file's a bug magnet, Randy. Stay away from it."* |
| 🟢 Hotspot stabilizing | *"That hotspot's cooling down. The liquor did its job."* |
| 🔴 Hotspot worsening | *"That file's getting worse, Randy. The shit-fire is spreading."* || � Spec stale | *"Gary can see right through it, boys. That spec expired three refactors ago."* |
| �🔴 Spec conflict | *"Steve French found another cougar's territory. Two specs, same route."* |
| ❌ Security vuln | *"Safety... always ON."* |
| ✅ Docs updated | *"Know what I'm sayin'? It's published."* |
| ❌ Deferral detected | *"You can't just NOT do things, Corey!"* |
| ❌ Deferral blocks done | *"That's NOT gettin' two birds stoned — that's just sayin' you WILL."* |
| ❌ Manipulation detected | *"That's GREASY, boys. You can't just cross things out and say they're done!"* |
| ✅ Outcome contract satisfied | *"That's what we said we'd do, and that's what we did. DEEEE-CENT!"* |
| ❌ Outcome contract violated | *"Tests passed but the thing don't actually WORK, boys. That's not decent."* |
| ❌ Missing outcome contract | *"You can't ship it if you never said what it's supposed to DO, Ray."* |
| ❌ Format bypass | *"You can't just erase the checkboxes and call it a day, Ricky!"* |
| ❌ Invented status | *"'Deferred — Planned Improvement'?! That's not even a real thing, Julian!"* |
| ✅ Handoff complete | *"Have a good one, boys."* |
| ❌ Gap found | *"This is f\*\*\*ed. BAAAAM!"* |
| ✅ Bug located | *"That's a nice f\*\*\*ing kitty right there."* |
| ✅ Build succeeds | *"Knock knock." "A passing build."* |
| Milestone reached | *"Freedom 35, boys!"* |

---

## <img src="../icons/trinity-notebook.svg" width="32"> Quick Reference — What To Type When

### Starting a Job

| Situation | Command |
|-----------|---------|
| **Don't know what to do? Just describe it.** | **`/bubbles.workflow  <describe what you want in plain English>`** |
| Continue from last session | `/bubbles.workflow  continue` |
| New feature from scratch | `/bubbles.workflow  <describe feature> mode: product-to-delivery` |
| Full delivery pipeline | `/bubbles.workflow  full-delivery for <feature>` |
| Improve legacy feature with one stale-spec pass first | `/bubbles.workflow  improve-existing for <feature> specReview: once-before-implement` |
| Fix a bug | `/bubbles.workflow  fix the <describe bug>` |
| Plan and scope a feature | `/bubbles.plan  <feature>` |
| Need framework help or advice? | `/bubbles.super  help me <describe goal>` |
| Refresh framework setup | `/bubbles.setup  mode: refresh` |

### Natural Language — Just Say What You Want

All agents accept natural language. `/bubbles.workflow` is the **universal entry point** — it resolves intent via `super`, picks work via `iterate`, and drives phases to completion. Just describe what you want:

| You Type | Workflow Understands |
|----------|-------------------|
| `/bubbles.workflow  improve the booking feature to be competitive` | mode: improve-existing, spec: booking |
| `/bubbles.workflow  continue` | Resume active workflow if continuation context exists; otherwise pick next work via iterate |
| `/bubbles.workflow  fix all found` | Resume the active workflow's remaining routed work instead of dropping into raw specialists |
| `/bubbles.workflow  fix the calendar bug in page builder` | mode: bugfix-fastlane, spec: page-builder |
| `/bubbles.workflow  do 10 rounds of stabilize on booking` | mode: stochastic-quality-sweep, triggerAgents: stabilize, maxRounds: 10 |
| `/bubbles.workflow  spend 2 hours working on whatever needs attention` | mode: iterate, minutes: 120 |
| `/bubbles.workflow  doctor` | Framework health check (delegates to super) |
| `/bubbles.code-review  do an engineering sweep on the gateway` | profile: engineering-sweep, scope: service:gateway |
| `/bubbles.system-review  review the booking feature as a user` | mode: full, scope: feature:booking |
| `/bubbles.workflow  spend 2 hours working on whatever needs attention` | mode: iterate, minutes: 120 |
| `/bubbles.iterate  fix tests for the page builder` | type: tests, feature: page-builder |
| `/bubbles.workflow  do the next thing from recap` | mode: delivery-lockdown, target resolved from continuation envelope |
| `/bubbles.test  why are integration tests failing?` | action: triage, types: integration |
| `/bubbles.analyst  how does our booking compare to competitors?` | mode: improve, competitive research on |
| `/bubbles.security  scan for hardcoded secrets` | focus: secrets |
| `/bubbles.spec-review  are the booking specs still valid?` | scope: booking, depth: thorough |
| `/bubbles.chaos  break the search feature` | scope: search |
| `/bubbles.super  what's the best way to fix a bug?` | Platform Assistant: recommend bugfix sequence |

### Using The Super as Your Assistant

The super resolves intent and generates commands. Workflow delegates to it automatically for vague input, but you can also call `super` directly for advice or framework ops:

| You Ask | The Super Responds With |
|---------|-------------------|
| `/bubbles.super  I have a new feature idea for search` | Recommended sequence: analyst → ux → workflow product-to-delivery |
| `/bubbles.super  I want to make the booking feature better` | `/bubbles.workflow  <booking-spec> mode: improve-existing` |
| `/bubbles.super  review this repo before we decide what to spec` | `/bubbles.system-review  scope: full-system output: summary-doc` |
| `/bubbles.super  which mode should I use?` | Decision tree based on your situation |
| `/bubbles.super  help me write a command for chaos testing` | `/bubbles.workflow mode: stochastic-quality-sweep maxRounds: 5` |
| `/bubbles.super  before we improve booking, do one stale-spec check and then continue` | `/bubbles.workflow  <booking-spec> mode: improve-existing specReview: once-before-implement` |
| `/bubbles.super  fix all found from the last sweep` | `/bubbles.workflow  <same-target> mode: stochastic-quality-sweep` |
| `/bubbles.super  give me the no-loose-ends release workflow for booking` | `/bubbles.workflow  <booking-spec> mode: delivery-lockdown` |
| `/bubbles.super  what should I do before shipping?` | `/bubbles.workflow  <feature> mode: delivery-lockdown` |
| `/bubbles.super  should I start here or call the agent directly?` | Policy answer: use `super` for vague intent; go direct when the target is already known |
| `/bubbles.super  why did my workflow stop after validate?` | Short diagnosis + the next command to recover or continue |
| `/bubbles.super  run framework validation before I upgrade` | `bash bubbles/scripts/cli.sh framework-validate` |
| `/bubbles.super  check whether bubbles itself is ready to ship` | `bash bubbles/scripts/cli.sh release-check` |
| `/bubbles.super  show me the framework event stream` | `bash bubbles/scripts/cli.sh framework-events --tail 20` |
| `/bubbles.super  show current workflow run-state` | `bash bubbles/scripts/cli.sh run-state --all` |
| `/bubbles.super  is this repo agent-ready?` | `bash bubbles/scripts/cli.sh repo-readiness .` with the explicit note that this is advisory and not completion certification |
| `/bubbles.super  compare the adoption profiles for this repo` | `bash bubbles/scripts/cli.sh profile show` plus the right `repo-readiness` profile example for `foundation`, `delivery`, or `assured` |
| `/bubbles.super  what exactly will this upgrade replace?` | `bash bubbles/scripts/cli.sh upgrade --dry-run` with trust-preview explanation from release manifest and install provenance |
| `/bubbles.super  import my Claude Code or Cursor setup without touching framework files` | `bash bubbles/scripts/cli.sh interop import --review-only` and the project-owned intake path |
| `/bubbles.super  apply only the safe project-owned interop outputs` | `bash bubbles/scripts/cli.sh interop apply --safe` with proposal fallback for collisions |
| `/bubbles.super  why are my parallel sessions colliding?` | `bubbles runtime doctor` plus the right recovery step |
| `/bubbles.super  reuse the validation stack if it is compatible` | `bubbles runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml` |
| `/bubbles.super  turn this problem into the right Bubbles prompts` | A command sequence with brief reasons for each step |

### During Implementation

| Situation | Command |
|-----------|---------|
| Continue an active feature safely | `/bubbles.workflow  <feature> mode: delivery-lockdown` |
| Continue routed work from a stochastic sweep | `/bubbles.workflow  fix all found` |
| Implement a known scope surgically | `/bubbles.implement  execute scope 1 of <feature>` |
| Continue next scope | `/bubbles.iterate  continue <feature>` |
| Simplify complex code | `/bubbles.simplify` |
| Design the architecture | `/bubbles.design  create design for <feature>` |

### Testing & Validation

| Situation | Command |
|-----------|---------|
| Run the workflow test pass | `/bubbles.workflow  <feature> mode: test-to-doc` |
| Validate gates and publish the finishing packet | `/bubbles.workflow  <feature> mode: validate-to-doc` |
| Full audit | `/bubbles.audit` |
| Chaos testing | `/bubbles.chaos` |

### When Things Go Wrong

| Situation | Command |
|-----------|---------|
| Something seems off | `/bubbles.workflow  <feature> mode: validate-to-doc` |
| Find what's missing | `/bubbles.gaps` |
| Check if specs are still valid | `/bubbles.spec-review  all` |
| Harden weak spots | `/bubbles.harden` |
| Security scan | `/bubbles.security` |
| Check for regressions | `/bubbles.regression` |
| Quality sweep | `/bubbles.workflow  harden-gaps-to-doc` |
| Release lockdown | `/bubbles.workflow  delivery-lockdown` |

### Success & Wrap-Up

| Situation | Command |
|-----------|---------|
| Check progress | `/bubbles.status` |
| Check progress (narrative) | `/bubbles.status --explain` |
| Quick session recap | `/bubbles.recap` |
| Run a retrospective | `/bubbles.retro week` |
| Find code hotspots (bug magnets, coupling) | `/bubbles.retro hotspots` |
| Check bus factor risk | `/bubbles.retro busfactor` |
| Find hidden dependencies | `/bubbles.retro coupling` |
| Update documentation | `/bubbles.docs` |
| End of session | `/bubbles.handoff` |
| Resume tomorrow | `/bubbles.workflow  resume` |

### Framework Operations — `bubbles.super` or CLI

| Situation | Agent | CLI |
|-----------|-------|-----|
| Check project health | `/bubbles.super doctor` | `bubbles doctor` |
| Auto-fix health issues | `/bubbles.super doctor --heal` | `bubbles doctor --heal` |
| Check portable surface drift | `/bubbles.super agnosticity` | `bubbles agnosticity` |
| Install framework-repo git hooks | `/bubbles.super install hooks` | `bubbles hooks install --all` |
| Show available hooks | `/bubbles.super list hook catalog` | `bubbles hooks catalog` |
| Add custom hook | `/bubbles.super add pre-push hook for license` | `bubbles hooks add pre-push script.sh --name my-hook` |
| Add custom gate | `/bubbles.super add license gate` | `bubbles project gates add name --script path` |
| Show scope dependencies | `/bubbles.super show dag for 042` | `bubbles dag 042` |
| Enable metrics | `/bubbles.super enable metrics` | `bubbles metrics enable` |
| Show runtime lease owners | `/bubbles.super show active runtime leases` | `bubbles runtime leases` |
| Summarize runtime usage | `/bubbles.super show runtime summary` | `bubbles runtime summary` |
| Diagnose runtime conflicts | `/bubbles.super show runtime lease conflicts` | `bubbles runtime doctor` |
| Reclaim stale runtime leases | `/bubbles.super reclaim stale runtime leases` | `bubbles runtime reclaim-stale` |
| View lessons learned | `/bubbles.super show lessons` | `bubbles lessons` |
| Compact old lessons | `/bubbles.super compact lessons` | `bubbles lessons compact` |
| View skill proposals | `/bubbles.super show skill proposals` | `bubbles skill-proposals` |
| View developer profile | `/bubbles.super show profile` | `bubbles profile` |
| Clear stale preferences | `/bubbles.super clear stale profile` | `bubbles profile --clear-stale` |
| Upgrade Bubbles | `/bubbles.super upgrade` | `bubbles upgrade` |
| Upgrade (dry run) | `/bubbles.super upgrade --dry-run` | `bubbles upgrade --dry-run` |
| **Help me choose an agent** | **`/bubbles.super help me <goal>`** | — |
| **Generate a command** | **`/bubbles.super what command for <task>`** | — |
| **Recommend workflow** | **`/bubbles.super which mode for <situation>`** | — |
| **Multi-step plan** | **`/bubbles.super plan steps for <goal>`** | — |

---

## <img src="../icons/ricky-dynamite.svg" width="32"> Rickyisms — The Official Glossary

| Rickyism | What He Meant | Bubbles Context |
|----------|--------------|-----------------|
| "Worst case Ontario" | Worst case scenario | Chaos testing fallback |
| "Get two birds stoned at once" | Kill two birds with one stone | Implement + test combo |
| "It's not rocket appliances" | It's not rocket science | Overcomplicating things |
| "Supply and command" | Supply and demand | Planning & resources |
| "Water under the fridge" | Water under the bridge | Simplification done, move on |
| "I toad a so" | I told you so | When Conky (harden) was right |
| "Make like a tree and f*** off" | Make like a tree and leave | Cleaning up dead code |
| "What comes around is all around" | What goes around comes around | Circular dependency |
| "Denial and error" | Trial and error | Ignoring failing tests |
| "Passed with flying carpets" | Passed with flying colors | All gates passed |
| "Survival of the fitness, boys" | Survival of the fittest | Stochastic sweep results |
| "Gorilla see, gorilla do" | Monkey see, monkey do | Copy-paste code detected |
| "Steve French is just a big stoned kitty" | The regression guardian is doing its job | Cross-spec check running |
| "It's a doggy-dog world" | Dog-eat-dog world | Competitive analysis |
| "I'll do it tomorrah" | I'll do it tomorrow | Deferring work (G040 violation) |
| "Let me think about it over a couple smokes" | Let me think about it | Brainstorm mode — explore before building |
| "Get two birds stoned at once" | Kill two birds with one stone | Parallel scope execution via worktrees |
| "The park knows what you like" | Personalized from observation | Developer profile auto-resolving taste decisions |
| "Same greasy mistake three times" | Repeated pattern detected | Skill evolution — lessons promoting to skill proposal |
| "Count the empties, Randy" | Count what's measurable | Activity tracking — only measurable metrics, no guesses |
| "Lease the lot" | Claim runtime ownership before you start or reuse a shared stack | `bubbles runtime acquire` — make Docker/Compose ownership explicit |
| "Same stack, same lease" | Reuse the running stack only when the fingerprint matches | `shared-compatible` runtime reuse |
| "Stale trailer tag" | The owning session disappeared and the lease aged out | `bubbles runtime doctor` / `bubbles runtime reclaim-stale` |
| "Don't burn down the wrong trailer" | Cleanup must only touch owned or stale stacks | Lease-aware teardown and runtime conflict recovery |
| "Where the bodies are buried" | Deep code hotspot analysis — bug magnets, coupling, bus factor | `/bubbles.retro hotspots` — the liquor sees which files keep breaking |
| "All tangled up like Christmas lights" | Co-change coupling — files that always change together | `/bubbles.retro coupling` — hidden architectural dependencies |
| "Somebody's gotta drive" | Bus factor analysis — single-author files are knowledge silos | `/bubbles.retro busfactor` — who knows what, and what happens if they leave |
| "Liquor then tape" | Data-driven simplification — retro finds hotspots, Donny simplifies them | `retro-to-simplify` workflow mode |
| "Liquor then harden" | Data-driven hardening — retro finds weak spots, then harden them up | `retro-to-harden` workflow mode |
| "Liquor then sweep" | Retro-guided quality sweep — retro picks the hotspot mess, then the full cleanup crew sweeps it | `retro-quality-sweep` workflow mode |
| "Liquor then look" | Data-driven review — retro targets the riskiest files for code review | `retro-to-review` workflow mode |
| \"That spec's got freezer burn\" | Expired/stale content | Spec freshness audit finding |
| \"Just tell Bubbles\" | Start with `/bubbles.workflow` and describe what you want in plain English | Universal entry point — workflow resolves intent, picks work, drives phases |
| \"Bubbles figures it out\" | Workflow delegates to super for NLP resolution and iterate for work-picking | Intent delegation — no need to know which agent or mode to use |
---

<p align="center">
  <img src="../icons/bubbles-glasses.svg" width="40"><br>
  <em>"Have a good one, boys."</em><br>
  Sunnyvale Trailer Park Software Division<br>
  0 Shit Hawks
</p>
