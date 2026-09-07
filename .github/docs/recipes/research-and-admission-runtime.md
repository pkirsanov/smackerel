# Research And Admission Runtime

> Use the research runtime to build immutable evidence. Use admission to decide whether a measured action may consume the goal budget.

Bubbles 7.29.0 adds two related, provider-neutral runtime capabilities:

- the **research runtime** turns an explicit question into typed, content-addressed evidence records;
- the **measured admission runtime** controls reservations, debits, releases, retries, and verified session epochs before resource-consuming dispatch.

Both capabilities are opt-in and default off. They publish evidence and control decisions. They do not authorize consequential actions.

## Choose The Right Control

| Need | Use | Does not mean |
|---|---|---|
| Coordinate ownership of a shared Docker or Compose stack | `runtime` lease commands | Spend or token admission |
| Admit a measured model, subagent, web, browser, or tool dispatch | `admission` | Authorization to deploy, transact, message, or mutate infrastructure |
| Gather and publish evidence for a question | `research` | A workflow mode or product-delivery certification |
| Approve a consequential tool action | Tool-trust and action-risk controls | Budget availability |

The dependency direction is `ECF -> admission -> research`: the evidence-control foundation owns durable records, admission owns execution-control accounting, and research owns research semantics.

## Inspect Capability Before Running

In the Bubbles source repository:

```bash
bash bubbles/scripts/cli.sh research status
bash bubbles/scripts/cli.sh research capabilities
bash bubbles/scripts/cli.sh research schema
```

In an installed downstream repository, replace `bubbles/scripts/` with `.github/bubbles/scripts/`.

A missing adapter resolves to `none`. Invalid configured adapters fail loudly. The `none` usage adapter reports `unmeasured`; it never turns unknown usage into zero, free, estimated, or within budget.

## Research Lifecycle

The research command family exposes explicit lifecycle steps:

```bash
bash bubbles/scripts/cli.sh research validate-question --input /absolute/question.json
bash bubbles/scripts/cli.sh research plan --input /absolute/question.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research run --input /absolute/run.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research resume --input /absolute/resume.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research inspect --input /absolute/inspect.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research validate --input /absolute/validation.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research publish --input /absolute/publication.json --store-root /absolute/research-store
bash bubbles/scripts/cli.sh research cancel --input /absolute/cancellation.json --store-root /absolute/research-store
```

Use `bridge` only when a configured downstream bridge exists. `adapter-disabled` and `adapter-local-command` expose the shipped adapter boundary without pretending that a hosted provider is active.

Each stage receives exact inputs and emits typed immutable records. Deterministic stages and policy-selected model stages remain distinct. Publication produces evidence artifacts and a result envelope; it is not deployment, certification, or action authorization.

## Admission Lifecycle

Use the source CLI for the registered admission operations:

```bash
bash bubbles/scripts/cli.sh admission adapter
bash bubbles/scripts/cli.sh admission usage
bash bubbles/scripts/cli.sh admission evaluate --store-root /absolute/admission-store --input /absolute/admission.json
bash bubbles/scripts/cli.sh admission issue-permit --store-root /absolute/admission-store --input /absolute/permit.json
bash bubbles/scripts/cli.sh admission budget snapshot --store-root /absolute/admission-store --input /absolute/snapshot.json
bash bubbles/scripts/cli.sh admission epoch verify --store-root /absolute/admission-store --input /absolute/epoch.json
bash bubbles/scripts/cli.sh admission corpus evaluate --store-root /absolute/evaluation-store --input /absolute/corpus-evaluation.json
```

The direct facades are useful for focused integrations and diagnostics:

```bash
bash bubbles/scripts/dispatch-admission.sh evaluate --store-root /absolute/admission-store --input /absolute/admission.json
bash bubbles/scripts/goal-budget-ledger.sh snapshot --store-root /absolute/admission-store --input /absolute/snapshot.json
bash bubbles/scripts/session-epoch-authority.sh verify --store-root /absolute/admission-store --input /absolute/epoch.json
```

Admission binds reservations, debits, releases, holds, permits, retry identities, and epoch evidence to an exact goal budget. A permit controls one measured resource-consuming action. It does not grant broader workflow or consequential-action authority.

## Verified Session Epochs

A claimed fresh context is not enough. Epoch verification requires the runtime's recorded session evidence. Use the session-epoch facade to verify the epoch input against the store before treating rollover or retry accounting as a new epoch.

A missing or unverifiable epoch stays unverified. Do not convert it into a fresh-session claim.

## Frozen-Corpus Evaluation

The cost corpus evaluator replays fixed inputs against the repository-controlled accounting mechanisms:

```bash
bash bubbles/scripts/cost-corpus-evaluate.sh evaluate \
  --store-root /absolute/evaluation-store \
  --input /absolute/corpus-evaluation.json
```

This evaluates behavior on a frozen corpus. It does not prove causal cost savings, production rollout, or native host interception.

## Parked Activation Boundaries

The repository ships the contracts, local reference implementations, CLI routing, schemas, and focused selftests. These external activations remain parked until separately integrated and verified:

- hosted research providers;
- downstream research bridges;
- native host interception of every dispatch;
- live provider checkpoints and usage reconciliation;
- production rollout and causal savings measurement.

A reference broker demonstrates contract behavior. It is not evidence that the editor or host intercepts native dispatches.

## Related Recipes

- [Coordinate Runtime Leases](runtime-coordination.md) for shared runtime ownership.
- [Tool Trust And Untrusted Content](tool-trust-and-untrusted-content.md) for consequential-action authorization.
- [Framework Operations](framework-ops.md) for source validation and release hygiene.
