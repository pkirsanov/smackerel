---
description: How to author per-train feature-flag YAML bundles consumed by the existing config-bundle generator (G081 Build-Once Deploy-Many). Use when introducing a new feature flag, when creating a new train, or when retiring a flag.
---

# Per-Train Config Bundle Authoring

## Why Per-Train Bundles?

Trunk holds code for ALL trains (mvp, v1, v2, experimental). The same git SHA can ship to multiple slots simultaneously. What differs per train is **which feature flags are default-ON**. Bundles encode that difference.

CI's existing config-bundle generator (G081 Build-Once Deploy-Many) reads the train's bundle file and emits `<env>-<sourceSha>.tar.gz` containing the resolved env file. The bundle is signed (cosign) and pushed to the registry as an immutable artifact.
## Bundle File Layout

`config/feature-flags.<train>.yaml`:

```yaml
# Per-train feature-flag bundle. Owned by bubbles.train.
# This file is consumed by the config-bundle generator (G081) at CI build time.
version: 1
train: mvp              # MUST match the file name suffix and a train id in release-trains.yaml

flags:
  # Each entry: <flag_name>: <bool>
  # Default-ON in this train, default-OFF in every other train (G111).
  new_payment_flow: true
  fast_checkout: true
  experimental_quant_model_v2: false   # belongs to a different train; MUST be false here
  multi_tenant_admin: false            # belongs to a different train; MUST be false here

# Optional: per-flag metadata (consumed by flag-audit + compliance-sweep)
metadata:
  new_payment_flow:
    owning_spec: specs/220-new-payment-flow/
    introduced_in_train: mvp
    introduced_at: "2026-05-15"
  fast_checkout:
    owning_spec: specs/225-fast-checkout/
    introduced_in_train: mvp
    introduced_at: "2026-05-22"
```

## Flag Naming Convention

| Pattern | Rule |
|---|---|
| Snake case | `new_payment_flow`, not `newPaymentFlow` |
| Verb-noun or feature-noun | `enable_fast_checkout`, `use_v2_pricing`, `experimental_quant_model_v2` |
| No version suffix unless intentionally versioned | `payment_flow` (not `payment_flow_v1`); use `experimental_payment_flow_v2` only when both must coexist |
| Bool semantics, not enum | If you need an enum, use config not a flag |
| Owning train name in flag name FORBIDDEN | `mvp_payment_flow` — wrong; flag should outlive train |

## Language-Specific Consumption

Once the bundle is rendered to an env file by CI, services read flags from env vars (NEVER hardcoded):

### Rust

```rust
// Read at startup, fail fast if missing (no defaults).
let new_payment_flow: bool = std::env::var("NEW_PAYMENT_FLOW")
    .expect("NEW_PAYMENT_FLOW env required")
    .parse()
    .expect("NEW_PAYMENT_FLOW must be bool");

if new_payment_flow {
    serve_new_payment_route(router);
}
```

### Go

```go
import "os"
import "strconv"

newPaymentFlow, err := strconv.ParseBool(os.Getenv("NEW_PAYMENT_FLOW"))
if err != nil {
    log.Fatalf("NEW_PAYMENT_FLOW required, got: %v", err)
}
if newPaymentFlow {
    registerNewPaymentRoutes(router)
}
```

### TypeScript (frontend or node)

```typescript
const newPaymentFlow = process.env.NEW_PAYMENT_FLOW;
if (newPaymentFlow !== "true" && newPaymentFlow !== "false") {
  throw new Error("NEW_PAYMENT_FLOW env required (true|false)");
}
if (newPaymentFlow === "true") {
  registerNewPaymentRoutes();
}
```

### Python

```python
import os
val = os.environ["NEW_PAYMENT_FLOW"]  # KeyError if missing
if val not in ("true", "false"):
    raise ValueError(f"NEW_PAYMENT_FLOW must be true|false, got {val}")
if val == "true":
    register_new_payment_routes(app)
```

## Adding a New Flag (Workflow)

1. Author spec under `specs/NNN-feature-name/` declaring `releaseTrain: <owning-train>` in `state.json`.
2. Add `flagsIntroduced: [<flag_name>]` to `state.json`.
3. Add `<flag_name>: true` to `config/feature-flags.<owning-train>.yaml`.
4. Add `<flag_name>: false` to EVERY OTHER `config/feature-flags.<other-train>.yaml`.
5. Add metadata entry under `metadata:` in the owning bundle.
6. Implement code reading the env var (no defaults, fail-fast).
7. `release-train-guard.sh` verifies discipline at pre-push.

## Retiring a Flag (Workflow)

Triggered by `release-train-flag-audit.sh` flagging the flag as overdue (its train graduated > 1 cycle):

1. `bubbles.upkeep` (during `flag-cleanup-audit`) packets to `bubbles.train`.
2. `bubbles.train` opens a cleanup spec under `specs/NNN-flag-cleanup-<name>/`.
3. `bubbles.implement` removes the flag conditional from code (keeps the on-path).
4. `bubbles.train` removes the flag from ALL bundles (`feature-flags.*.yaml`).
5. `bubbles.implement` removes the env var reference from service startup.
6. `release-train-guard.sh` confirms no spec still declares `flagsIntroduced: [<flag>]`.

## Anti-Patterns

| ❌ Wrong | ✅ Right |
|---|---|
| `new_payment_flow: true` defaulted in `feature-flags.v1.yaml` because "we want to test it there too" | Default-ON ONLY in owning train; if v1 also wants it, declare in v1's bundle but route through a *new* spec on v1 |
| Hardcoded `if (env === 'prod')` checks instead of flag reads | Flags via env var; envs differ only by which bundle they got |
| Removing a flag from one bundle without all others | All bundles must agree the flag no longer exists; otherwise G111 false-positives |
| Flag value as string `"yes"` / `"on"` | Strict `true`/`false` (parsed as bool) |

## See Also

- Skill: `bubbles-release-train-model` (trains + cuts + promotes)
- Skill: `bubbles-flag-lifecycle` (introduction, retirement triggers)
- Skill: `bubbles-config-sst` (config single source of truth)
- Gate: G081 (Build-Once Deploy-Many), G111 (flag default-off)
