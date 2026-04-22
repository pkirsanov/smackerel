# Design: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes

## Fix Design

Convert fire-and-forget NATS publishes to request/reply patterns with timeouts and local fallback.

### Approach

Use the NATS request/reply pattern (publish with inbox reply subject, await response with timeout):

1. **Monthly report** (`monthly.go`): Publish report data to `smk.monthly.generate` with a reply inbox. Wait up to 30s for LLM-generated text. If response arrives, use it for `report.ReportText`. Fall back to `assembleMonthlyReportText` on timeout.

2. **Content fuel** (`monthly.go::GenerateContentFuel`): Publish topic data to `smk.content.analyze` with reply inbox. Wait up to 15s per topic. Use LLM angles when available, fall back to template angles.

3. **Learning classify** (`learning.go`): After loading resources, publish each resource to `smk.learning.classify` with reply inbox. Wait up to 10s. Use LLM difficulty when returned, fall back to `classifyDifficultyHeuristic`.

4. **Quick reference** (`lookups.go`): When creating quick references, publish source artifacts to `smk.quickref.generate`. Wait up to 15s. Use LLM-compiled content when returned, fall back to basic compilation.

5. **Seasonal** (`monthly.go::DetectSeasonalPatterns`): Publish pattern data to `smk.seasonal.analyze`. Wait up to 15s. Use LLM commentary when returned, fall back to template observations.

### Pattern
```go
// Example: request/reply with timeout
ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
defer cancel()
reply, err := e.NATS.RequestWithContext(ctx, subject, payload)
if err != nil {
    slog.Warn("NATS request failed, using local fallback", "subject", subject, "error", err)
    // use local fallback
} else {
    // parse reply and use LLM-enhanced result
}
```

### Files Changed
- `internal/intelligence/monthly.go` — convert 2 fire-and-forget to request/reply
- `internal/intelligence/learning.go` — add NATS publish + reply for difficulty classification
- `internal/intelligence/lookups.go` — add NATS publish + reply for quick reference generation
- `internal/nats/client.go` — ensure `RequestWithContext` helper exists
