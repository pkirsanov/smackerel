# Cross-Model Review: Unavailable Capability And Migration Note

## Current Status

Cross-model review is not an executable Bubbles capability. Bubbles currently
has no verified external provider/model adapter, and current VS Code subagents
inherit the active session's model and tools. Naming a provider or model in a
label, configuration value, or CLI-shaped string does not prove that a distinct
provider/model invocation occurred.

The former active recipe, registry setup, usage tracking, and command examples
were unsupported claims. They are intentionally not retained as operator
instructions.

## What `samples: N` Means

Adversarial `samples: N` requests N separate `bubbles.redteam` invocations from
an active top-level workflow. Those invocations are always labeled
`sampleSemantics: same-runtime-correlated`. They are repeated checks in the
active runtime, not cross-model review.

Each direct redteam invocation emits one sample. A top-level workflow must
dispatch all N invocations and aggregate their actual schema-version-1 records.
Disagreement preserves and escalates the full finding union; a majority cannot
silence a finding.

See [Adversarial Verification](adversarial-verification.md) for the supported
sample and aggregation contract.

## Requirement For Enabling This Capability

An external review adapter would need trusted host- or provider-derived
provenance for provider and model identity, proof that the identity differs from
the active runtime, hashed adapter/config identity, and retained invocation
evidence. Operator-entered labels and executable names are insufficient. No
adapter satisfying that contract is enabled in Bubbles.
