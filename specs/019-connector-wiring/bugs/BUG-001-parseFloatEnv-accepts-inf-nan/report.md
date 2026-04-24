# Report: BUG-019-001

### Summary

Chaos probe identified that `parseFloatEnv` (originally in `cmd/core/main.go`, now in `cmd/core/helpers.go` after later refactor) silently accepted IEEE 754 special values (Inf, -Inf, +Inf, NaN, nan) from `strconv.ParseFloat`. These values would propagate into market alert thresholds via `classifyTier()` and silently suppress alerts. Fix added an explicit `math.IsNaN || math.IsInf` guard that returns 0 and emits a warning log when a non-finite value is encountered.

### Completion Statement

All Scope 1 DoD items verified. Non-finite guard present in `cmd/core/helpers.go:75-78`. Five adversarial regression tests (`TestParseFloatEnv_Inf`, `_NegInf`, `_PosInf`, `_NaN`, `_NaN_Lowercase`) added at `cmd/core/main_test.go:292+`. Existing parseFloatEnv tests preserved (lines 227-289). Repo CLI test and check commands pass.

### Test Evidence

```
$ ./smackerel.sh test unit
ok  	github.com/smackerel/smackerel/cmd/core	(cached)
ok  	github.com/smackerel/smackerel/internal/connector/markets	(cached)
... (all Go packages pass)
```

```
$ grep -n "TestParseFloatEnv" cmd/core/main_test.go
227:func TestParseFloatEnv_ValidFloat(t *testing.T) {
235:func TestParseFloatEnv_Integer(t *testing.T) {
243:func TestParseFloatEnv_EmptyString(t *testing.T) {
251:func TestParseFloatEnv_UnsetVar(t *testing.T) {
260:func TestParseFloatEnv_InvalidFloat(t *testing.T) {
268:func TestParseFloatEnv_NegativeFloat(t *testing.T) {
276:func TestParseFloatEnv_Zero(t *testing.T) {
284:func TestParseFloatEnv_ScientificNotation(t *testing.T) {
292:// --- CHAOS-019-001: parseFloatEnv must reject IEEE 754 special values ---
294:func TestParseFloatEnv_Inf(t *testing.T) {
```

### Validation Evidence

```
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
```

```
$ grep -n "math" cmd/core/helpers.go
6:	"math"
75:	if math.IsNaN(f) || math.IsInf(f, 0) {
$ head -10 cmd/core/helpers.go
package main

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"strconv"
)
```

### Audit Evidence

```
$ sed -n '63,80p' cmd/core/helpers.go
// parseFloatEnv reads an environment variable and parses it as float64.
// Returns 0 on empty string. Logs a warning and returns 0 on parse error.
func parseFloatEnv(key string) float64 {
	s := os.Getenv(key)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		slog.Warn("failed to parse float from env var — using 0", "key", key, "error", err)
		return 0
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		slog.Warn("non-finite float value in env var — using 0", "key", key, "value", s)
		return 0
	}
```

Adversarial coverage exhaustively probes all five IEEE 754 special-value spellings (Inf, -Inf, +Inf, NaN, nan). Existing 8 parseFloatEnv tests preserved — no regression. Function signature unchanged; downstream `classifyTier()` callers receive 0 instead of NaN/Inf-tainted thresholds.
