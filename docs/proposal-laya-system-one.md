# Proposal: System One Decision Layer with Laya 421M

## Context
smackerel requires fast schema-bound decisions for intent compile, scenario routing, and guardrails. Current plan considered Jev / Clef alternatives.

## Decision
Adopt Laya 421M as the System One decision provider for smackerel.

## Rationale
- Laya is 421M non-autoregressive ModernBERT backbone, Apache 2.0
- CPU runnable, <1 GB footprint, no GPU required
- Ollama compatible, no new runner
- Single-pass classification/routing with guaranteed schema output
- Keeps model count minimal vs Clef/Clef-Flash

## Integration
- Provider: laya:421m
- Use for: intent compile, scenario routing, guardrail classification
- Keep muse-glimmer 30b 8bit for System Two reasoning
- Provider seam: knb ProviderFactory systemone-fast → laya

## Success criteria
- p95 latency <100 ms per decision
- 100% schema valid output
- Calibration Brier <0.2 on intent test set

## Risks
- Smaller model may underperform on rare scenarios. Mitigate with fallback to muse-glimmer for low confidence.
