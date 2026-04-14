# Report: BUG-019-001

## Execution Evidence

| Step | Result | Evidence |
|------|--------|----------|
| Chaos probe | Found | `parseFloatEnv` accepts Inf/NaN from `strconv.ParseFloat` — silently suppresses market alerts via `classifyTier()` |
| Fix implemented | Done | Added `math.IsNaN`/`math.IsInf` guard in `parseFloatEnv` (`cmd/core/main.go`) |
| Tests added | Done | 5 adversarial tests: `TestParseFloatEnv_Inf`, `TestParseFloatEnv_NegInf`, `TestParseFloatEnv_PosInf`, `TestParseFloatEnv_NaN`, `TestParseFloatEnv_NaN_Lowercase` |
| Unit tests pass | Done | `./smackerel.sh test unit` — all 33 Go packages pass, `cmd/core` rebuilt (0.040s) |
