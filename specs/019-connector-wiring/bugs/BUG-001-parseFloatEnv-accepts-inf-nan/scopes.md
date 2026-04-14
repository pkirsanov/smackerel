# Scopes: BUG-019-001 parseFloatEnv Inf/NaN Rejection

## Scope 1: Add Non-Finite Guard to parseFloatEnv

**Status:** Done

### DoD

- [x] `parseFloatEnv` rejects `Inf`, `-Inf`, `+Inf`, `NaN` (any case) and returns 0 with warning log
- [x] `math` package imported in `cmd/core/main.go`
- [x] Adversarial test: `"Inf"` → returns 0
- [x] Adversarial test: `"-Inf"` → returns 0
- [x] Adversarial test: `"+Inf"` → returns 0
- [x] Adversarial test: `"NaN"` → returns 0
- [x] Adversarial test: `"nan"` (lowercase) → returns 0
- [x] Existing `parseFloatEnv` tests still pass (no regression)
- [x] `./smackerel.sh test unit` passes
