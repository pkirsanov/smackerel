# User Validation: BUG-075-001 Residual metric order independence

## Checklist

- [x] The residual privacy E2E creates its own real retired-command observation.
- [x] The live scrape requires a concrete residual sample with only privacy-safe labels.
- [x] The test passes alone and does not rely on another test's execution order.
