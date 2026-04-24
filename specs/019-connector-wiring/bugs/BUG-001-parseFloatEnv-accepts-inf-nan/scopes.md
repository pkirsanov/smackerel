# Scopes: BUG-019-001 parseFloatEnv Inf/NaN Rejection

## Scope 1: Add Non-Finite Guard to parseFloatEnv

**Status:** Done

### Definition of Done

- [x] `parseFloatEnv` rejects `Inf`, `-Inf`, `+Inf`, `NaN` (any case) and returns 0 with warning log
  ```
  cmd/core/helpers.go:75:	if math.IsNaN(f) || math.IsInf(f, 0) {
  cmd/core/helpers.go:76:		slog.Warn("non-finite float value in env var — using 0", "key", key, "value", s)
  cmd/core/helpers.go:77:		return 0
  ```
- [x] `math` package imported (originally specified `cmd/core/main.go`; function was extracted to `cmd/core/helpers.go` during later refactor — guard remains correct)
  ```
  $ grep -n "\"math\"" cmd/core/helpers.go
  (math import present in file header alongside strconv, log/slog, os)
  ```
- [x] Adversarial test: `"Inf"` → returns 0
  ```
  cmd/core/main_test.go:294:func TestParseFloatEnv_Inf(t *testing.T) {
  ```
- [x] Adversarial test: `"-Inf"` → returns 0
  ```
  cmd/core/main_test.go: TestParseFloatEnv_NegInf (under CHAOS-019-001 banner at line 292)
  ```
- [x] Adversarial test: `"+Inf"` → returns 0
  ```
  cmd/core/main_test.go: TestParseFloatEnv_PosInf (under CHAOS-019-001 banner at line 292)
  ```
- [x] Adversarial test: `"NaN"` → returns 0
  ```
  cmd/core/main_test.go: TestParseFloatEnv_NaN (under CHAOS-019-001 banner at line 292)
  ```
- [x] Adversarial test: `"nan"` (lowercase) → returns 0
  ```
  cmd/core/main_test.go: TestParseFloatEnv_NaN_Lowercase (under CHAOS-019-001 banner at line 292)
  ```
- [x] Existing `parseFloatEnv` tests still pass (no regression)
  ```
  cmd/core/main_test.go: TestParseFloatEnv_ValidFloat, _Integer, _EmptyString, _UnsetVar, _InvalidFloat, _NegativeFloat, _Zero, _ScientificNotation (lines 227-289)
  ```
- [x] `./smackerel.sh test unit` passes
  ```
  $ ./smackerel.sh test unit
  ok  	github.com/smackerel/smackerel/cmd/core	(cached)
  ```
