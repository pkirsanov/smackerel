# Recipe: Ask the Super First

> *"I'm the trailer park supervisor. Start here and I'll tell you the next move."*

**Note:** Since v3.2, `/bubbles.workflow` is the **universal entry point** — it delegates to `super` automatically for vague input. You can now type `/bubbles.workflow improve the booking feature` directly and workflow will resolve the intent via super internally. Use `/bubbles.super` directly when you want framework ops (doctor, hooks, upgrade) or when you want command *recommendations* without execution.

Use `bubbles.super` as the help and assistant agent for Bubbles. This is the recipe for turning vague goals, messy problems, or framework confusion into the exact next prompt or command.

## When To Use It

Use this recipe when any of these are true:
- You do not know which agent should handle the task
- You do not know which workflow mode fits the situation
- You want the exact slash command, not a general explanation
- You need a multi-step sequence across several agents
- You need help solving a Bubbles-framework problem
- You want the framework translated into plain English

## Ask For One Command

```
/bubbles.super  I want to improve the booking feature
→ /bubbles.workflow  <booking-spec> mode: improve-existing

/bubbles.super  what's the right command to harden specs 11 through 37?
→ /bubbles.workflow  011-037 mode: harden-to-doc

/bubbles.super  before we improve booking, run one stale-spec pass so old duplicated scopes don't mislead the workflow
→ /bubbles.workflow  <booking-spec> mode: improve-existing specReview: once-before-implement

/bubbles.super  I need the no-loose-ends release workflow for booking
→ /bubbles.workflow  <booking-spec> mode: delivery-lockdown

/bubbles.super  I have a rough idea and want to think it through before we write code
→ /bubbles.workflow  mode: brainstorm for <idea>

/bubbles.super  I want the brownfield improvement path that researches current reality before it starts designing fixes
→ /bubbles.workflow  <feature-spec> mode: improve-existing

/bubbles.super  review this repo before we decide what to spec
→ /bubbles.system-review  scope: full-system output: summary-doc

/bubbles.super  give me the safest tdd-first workflow for this bug
→ /bubbles.workflow  <bug-or-feature> mode: bugfix-fastlane tdd: true
```

## Ask For A Prompt Sequence

```
/bubbles.super  I have a vague feature idea for notifications, what do I do?
→ 1. /bubbles.analyst  Build a notification system with email and push support
→ 2. /bubbles.ux  specs/NNN-notification-system
→ 3. /bubbles.workflow  specs/NNN-notification-system mode: product-to-delivery
```

## Ask For Workflow Advice

```
/bubbles.super  which mode should I use for improving an existing feature?
→ A short recommendation plus the exact command

/bubbles.super  what should I run before release if it has to keep going until everything is green?
→ A short recommendation plus `/bubbles.workflow  <feature> mode: delivery-lockdown`

/bubbles.super  what's the difference between harden-to-doc and gaps-to-doc?
→ A concise comparison and the recommended choice for your situation

/bubbles.super  should we grill this before we plan it?
→ A short recommendation plus the exact command, usually `/bubbles.grill ...` or `/bubbles.workflow ... grillMode: required-on-ambiguity`

/bubbles.super  should we ask the super first or call the agent directly?
→ A short policy note: use `super` for vague intent and translation; call the specialist directly when you already know the exact target
```

## Ask For Framework Help

```
/bubbles.super  why did my workflow stop after validate?
→ A diagnosis of the likely framework reason and the recovery command

/bubbles.super  fix all found from the last sweep
→ /bubbles.workflow  <same target> mode: stochastic-quality-sweep

/bubbles.super  my hooks don't seem installed right, fix that
→ The repair action plus the verification command

/bubbles.super  why are my parallel sessions colliding?
→ `bubbles runtime doctor` plus the safest recovery step

/bubbles.super  what just happened in the framework?
→ `bash <source-or-downstream-cli> framework-events --tail 20`

/bubbles.super  show me whether we're shipping progress or just cleaning up rework
→ `/bubbles.retro  week`

/bubbles.super  did our prompt surfaces get too bloated?
→ `bash <source-or-downstream-cli> lint-budget`

/bubbles.super  show me the active and recent workflow runs
→ `bash <source-or-downstream-cli> run-state --all`

/bubbles.super  reuse the validation stack if it is compatible
→ `bubbles runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml`

/bubbles.super  is this repo ready for Bubbles?
→ `bash <source-or-downstream-cli> repo-readiness .`

/bubbles.super  compare the Bubbles adoption profiles for this repo
→ `bash <source-or-downstream-cli> profile show`

/bubbles.super  switch this repo from foundation to delivery guidance
→ `bash <source-or-downstream-cli> profile set delivery`

/bubbles.super  show the stricter readiness posture without changing certification semantics
→ `bash <source-or-downstream-cli> repo-readiness . --profile assured`
```

## Ask It To Translate Problems Into Bubbles

```
/bubbles.super  turn this bug report into the right Bubbles prompts
/bubbles.super  help me use Bubbles to get this repo ready
/bubbles.super  give me the smallest next move to recover this feature
```

## What A Good Super Request Looks Like

Good requests include one of these:
- The goal: "I want to ship this feature"
- The problem: "my workflow stopped after validate"
- The uncertainty: "I don't know which mode fits"
- The outcome: "turn this into the right prompts"

You do not need perfect framework vocabulary. Just describe the situation and let the super resolve it.

Super should resolve the correct CLI path automatically:
- source framework repo: `bash bubbles/scripts/cli.sh ...`
- downstream installed repo: `bash .github/bubbles/scripts/cli.sh ...`