# Design: parseFloatEnv Inf/NaN Rejection

**Bug ID:** BUG-019-001

## Fix Design

Add a `math.IsNaN`/`math.IsInf` guard after the successful `strconv.ParseFloat` call in `parseFloatEnv`. This single-point fix protects all 12+ call sites in the connector wiring code.

### Before

```go
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
    return f
}
```

### After

```go
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
    return f
}
```

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main.go` | Add `"math"` import, add Inf/NaN guard in `parseFloatEnv` |
| `cmd/core/main_test.go` | Add 6 adversarial test cases for Inf/NaN/+Inf/-Inf/nan variants |
