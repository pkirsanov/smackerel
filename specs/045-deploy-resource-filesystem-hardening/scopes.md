# Scopes: Deploy Resource and Filesystem Hardening

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Resource and ML model envelope contract

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-045-A01 Operator sees bounded service resources
  Given Smackerel is prepared for deployment
  When the deploy contract or generated runtime config is inspected
  Then core and ML services have explicit CPU and memory envelopes
  And those values originate from the SST configuration pipeline

Scenario: SCN-045-A02 ML model selection fits the memory envelope
  Given the operator configures a model and deployment resource class
  When config validation runs
  Then the selected model is accepted only if the documented memory envelope can support it
  And incompatible model choices fail loudly before runtime start
```

### Implementation Plan

1. Add SST-backed resource envelope keys for core, ML, and supporting services.
2. Generate those values into the product-owned deployment contract or compose surface.
3. Add model memory profile validation for configured ML model names.
4. Add contract tests for missing limits and incompatible model choices.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-045-001 | unit/config | config generator tests | SCN-045-A01 | Missing CPU or memory envelope fails config validation. |
| T-045-002 | contract | deploy/compose contract test | SCN-045-A01 | Generated runtime surface contains required service limits. |
| T-045-003 | unit/config | config validation tests | SCN-045-A02 | Oversized ML model fails validation before runtime start. |

### Definition of Done

- [ ] T-045-001 passes with missing resource envelope values rejected.
- [ ] T-045-002 passes and proves generated runtime/deploy surfaces contain service resource limits.
- [ ] T-045-003 passes and proves incompatible ML model choices fail loudly.

## Scope 2: Read-only root filesystem contract

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-045-A03 Container roots are read-only except explicit mounts
  Given the runtime stack is generated
  When the service containers start
  Then writable root filesystem access is denied where read-only root is declared
  And required writable directories are backed by explicit tmpfs or named volumes
```

### Implementation Plan

1. Inventory writable paths for core, ML, PostgreSQL, NATS, Ollama, and temporary processing.
2. Mark eligible service roots read-only in product-owned runtime surfaces.
3. Add explicit tmpfs or named volume mounts for required writable paths.
4. Add integration or smoke checks that attempt writes to root and writable paths.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-045-004 | integration | runtime stack test | SCN-045-A03 | Root writes are denied for read-only-root services. |
| T-045-005 | integration | runtime stack test | SCN-045-A03 | Declared writable paths accept required writes. |
| T-045-006 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-045-004 passes and proves read-only root enforcement.
- [ ] T-045-005 passes and proves explicit writable mounts satisfy runtime needs.
- [ ] T-045-006 passes and this planning packet remains lint-clean.
