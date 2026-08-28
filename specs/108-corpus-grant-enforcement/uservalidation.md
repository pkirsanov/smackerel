# User Validation: 108 Corpus Grant Enforcement

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened`
**Status of this file at planning time:** baseline established, nothing implemented yet.

## How To Use This File

Every entry below is **checked `[x]` by default** — it records a statement that was
established during spec + design + planning and is currently believed true.

**Uncheck an item `[ ]` to report that it is wrong or broken.** An unchecked item is a
user-reported regression and is BLOCKING: no further scope work proceeds until it is
investigated and resolved.

At this ceiling (`specs_hardened`) the checklist validates the **plan**, not a running
system. Behavioral entries covering the running gate are added when Scope 03 is executed and
real evidence exists — they are deliberately absent here rather than pre-checked without
execution.

## Checklist

### Problem framing

- [x] The stated problem is real: `GateGlobalCorpusRead` exists but has zero production callers, so any authenticated principal currently reads the entire global corpus.
- [x] `dailyUserGrants` excluding `corpus:read` while the router allows the read is a genuine contradiction worth closing, not a documentation error.
- [x] Closing the gap is worth doing now rather than deferring behind spec 109 (MCP).

### Scope decomposition

- [x] Five scopes in the recorded dependency order (registration → observe plumbing → gate mount → caller remediation → docs/flags) is the right shape for this feature.
- [x] Scope 01 (registering the `corpus` scope surface) correctly blocks everything else — granting must be possible before anything is gated on the grant.
- [x] Scope 03 (the gate mount) is the correct place for the adversarial route-manifest test, not a later hardening pass.
- [x] Scope 04 (caller remediation) correctly sits before Scope 05, so the owning-train flag is never flipped ON while a caller surface is still an unmeasured unknown.

### Rollout shape

- [x] The two-stage OBSERVE → ENFORCE rollout is preferable to flipping enforcement on directly, because the denial set is currently unknown.
- [x] Resolving the stage once at startup, fail-loud, is preferable to a hot-reloadable stage that would require a silent default.
- [x] A rollback that is a config change plus a restart of the same signed image (no rebuild) is an acceptable operator cost.
- [x] Denial as a bare 403 with no count, id, title, or existence hint is the right user-facing behavior, even though it gives the denied user little diagnostic information.

### Caller impact

- [x] It is acceptable that PWA and browser-extension daily users are denied at ENFORCE until their token is rotated with `corpus:read` added.
- [x] Granting `corpus:read` via a token rotation (rather than a flag flip) is the correct operator mechanism.
- [x] The Telegram bridge (F-108-TELEGRAM-01) must be resolved before the owning-train flag is flipped ON, not after.
- [x] Letting shared-token and bootstrap sessions bypass the scope check is a deliberate, test-asserted decision rather than an oversight.

### Planning artifacts

- [x] Every scope's Definition-of-Done test-item count matches its Test Plan row count.
- [x] Every Gherkin scenario in `scopes.md` has a stable `SCN-108-*` id listed in `scenario-manifest.json` with its owning scope.
- [x] The recorded divergence between design.md (flag `false` in both trains) and the enforced release-train policy (default-ON in exactly one owning train) is correctly surfaced as a routed open item rather than silently applied.

### Operator ratification — `spec.md` items 7-10

`spec.md` §"Operator Ratification Additions" lists items 7-10 and states they belong in this
file. They are recorded here as ratified. **Ratifier for all four:**

> Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by bubbles.plan on behalf of the operator.

**What that attribution does and does not claim.** The owner did **not** hand-pick each option
individually. The owner was presented with the open items and delegated the choice with a stated
criterion ("pick the best option for long term, no shortcuts, continue"). Each ruling below was
selected against that criterion and is recorded as an owner-delegated decision — not as product
authority held by the recording agent. Uncheck an item to withdraw its ruling.

- [x] **Item 7 — coverage bar for asserting `OBSERVE-CLEAN` (F-108-UX-COVERAGE-01).** All eight route groups MUST either (a) show real observed traffic within the operator-declared observation window, or (b) be explicitly attested by the operator as `idle-by-design` with a recorded reason **and** a named principal that would have exercised that group. `OBSERVE-CLEAN` MUST NOT be asserted while any group is silently unobserved. *Ratifier:* Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by bubbles.plan on behalf of the operator.
  - *Rationale:* an unobserved group means the ENFORCE flip has undefined blast radius for that surface, which defeats the entire purpose of an observation window. Forcing an explicit `idle-by-design` attestation converts an unknown into a recorded, reviewable decision. It costs one line per idle group and removes the single largest failure mode of the migration.
  - *Tightens:* `spec.md` S6 "safe to enforce" condition 2, which today requires non-zero coverage only for groups "that must keep working". The ratified bar covers **all eight**, with attestation as the only alternative to real traffic. Where the two differ, this ruling binds.
  - *Honest dependency:* asserting (a) requires the per-route-group **request** counter (the denominator) that F-108-UX-COVERAGE-01 demands. That counter is still routed to `bubbles.design` and is deliberately **not** silently added to Scope 02's three-metric plan by this ratification pass — the same discipline already applied to the flag-default divergence.

- [x] **Item 8 — grant editor vs grant-issuance notice (F-108-UX-ADMINUI-01).** Ship the grant-issuance **NOTICE only**. **No grant editor in this spec.** *Ratifier:* Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by bubbles.plan on behalf of the operator.
  - *Rationale:* a grant editor is a new privileged mutation surface that edits token authority. It deserves its own threat model, its own spec, and its own adversarial tests. Bolting it onto an auth-enforcement migration widens the blast radius of both. This packet's own design already records that granting is a **token rotation**, not a flag flip (F-108-GRANT-MECHANISM-01), so the notice is sufficient to operate the migration. If an editor is ever wanted, it is a separate spec.
  - *Selects the branch `spec.md` S7 already pre-authored:* "Minimum acceptable outcome if the full grant editor is deferred by `bubbles.design`: the page MUST still render the grant issuance notice." Silence remains the one unacceptable option.
  - *Supersedes, in S7:* the mint-form `Grants:` checkbox affordance marked `← TARGET`, the "Rotate → grant editor **pre-populated** from the principal's recorded current grants" interaction, and the accessibility bullets that describe grant chips and the grant checkbox `fieldset`. Those describe the editor branch and do not bind under this ruling. Rewriting that UX-owned prose is routed to the owning agent, not done from this planning pass.
  - *Consequence recorded honestly:* S7 offered the pre-populated editor as "what makes SCN-108-F02 achievable". With the editor deferred, that rationale no longer applies, and **F-108-UX-ROSTER-01 remains BLOCKING**. Item 9 is the compensating control for the migration, not a closure of that finding.

- [x] **Item 9 — pre-existing tokens with unknowable grants (F-108-UX-ROSTER-01).** **Rotate proactively BEFORE the ENFORCE flip.** Do not leave them to surface as `unknown` in S7. *Ratifier:* Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by bubbles.plan on behalf of the operator.
  - *Rationale:* `unknown` grants mean the pre-flip state is not fully known, so the flip's blast radius cannot be computed. Proactive rotation makes the pre-flip roster fully determinate, which is exactly what OBSERVE → ENFORCE exists to guarantee. It is more work once, versus an open-ended incident risk on every flip. It is also the least-privilege-consistent choice.
  - *Why this is coherent despite F-108-UX-ROSTER-01:* proactive rotation does not require reading an unreadable prior grant set. The operator **issues a deliberate grant set** for each principal, so the roster becomes determinate by construction rather than by recovery. That is precisely why it sidesteps the "rotation replaces rather than adds" hazard (F-108-UX-ROTATE-ADD-01) for the migration.
  - *What it does NOT close:* grants still are not server-readable after rotation, so F-108-UX-ROSTER-01 stays BLOCKING and SCN-108-F02 remains at risk for any token that is **not** proactively rotated. The `unknown` rendering requirement in S7 also stays — rendering a guess, a default, or an empty set would be fabricated authority state.

- [x] **Item 10 — Register 3 copy.** **Approved exactly as written.** `COPY-DENY-HEAD` = `You don't have access to the corpus.`; `COPY-DENY-NEXT` = `Ask your operator for corpus access.`; `COPY-DENY-LINE` = the derived composite. *Ratifier:* Owner-delegated ratification 2026-07-29; criterion: best long-term option, no shortcuts. Recorded by bubbles.plan on behalf of the operator.
  - *Rationale:* it is a closed set and already UX-reviewed. Any future change to this user-visible language is a **spec change**, not an implementation detail.
  - *Consequence:* the strings are now frozen product language. F-108-UX-COPY-SST-01 (single source of truth vs. three parallel literals across Go/JS) is unaffected by this ruling and remains `bubbles.design`'s call; ratifying the copy strengthens the case for a mechanical anti-drift check but does not decide the mechanism.

### Gaps this ratification surfaced (recorded, not silently absorbed)

- [x] Ratifying items 8 and 10 exposed that **no scope in `scopes.md` currently owns** the S7 grant-issuance notice or the Register 3 human-copy rendering on the PWA, extension, and Telegram surfaces. The five planned scopes cover the wire envelope (Register 1) and the Telegram routing remedy, but not the human copy or the admin notice. This is recorded as a routed planning gap in `state.json.certification.outstandingFindings` rather than absorbed by inventing scopes during a ratification pass.


## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-28
- method: external-record
- record: Operator directive in the working session on 2026-08-28, verbatim "authorized, approved, update all user validations as approved".

### Scope of this acceptance, stated precisely

This acceptance covers the delivered SCOPE-01/02/03/05 behaviour. It does **NOT**
close this packet. SCOPE-04 remains genuinely blocked on three operator-owned,
time-bound items that no acceptance can substitute for: at least 14 consecutive
OBSERVE days, operator-performed rotation of every principal whose grants are
unknowable, and the OBSERVE-window go/no-go query returning an empty or
explicitly-accepted denial set. **No agent exercised any of these**, and none of
them can be satisfied by an agent at all.
