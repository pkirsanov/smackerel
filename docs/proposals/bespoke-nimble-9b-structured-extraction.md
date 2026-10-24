# Proposal: Bespoke-Nimble-9B for Structured Extraction and Fast Agent Tier

## Objective
Introduce Bespoke-Nimble-9B as the fast structured-output tier for Smackerel passive intelligence. Use for annotation classification, entity/relation extraction, and agent fast path. Retain muse-glimmer:30b for deep synthesis and multimodal.

## Rationale
Smackerel is local-first, Ollama-backed, LiteLLM routed. Current fast tier points to muse-glimmer:30b. Config shows fast_model currently equals generalist. Test lane uses qwen2.5:0.5b-instruct for speed.

Bespoke-Nimble-9B is tuned for schema adherence and low-latency classification. It fits Smackerel's search-by-meaning and graph linking where output must be valid JSON.

## No-brainer scenarios
1. Prompt contract annotation-classify-v1 with model_preference fast: structured classification output
2. Graph linker relation extraction: JSON {source, target, relation, confidence}
3. Passive intelligence digest assembly pre-filtering
4. Agent tool loop fast routing decisions

## Why better
- Higher schema validity than 0.5B, lower memory/latency than 30B
- Fits Ollama 8G envelope and local-first privacy posture
- Improves agent turn latency for model_preference fast paths

## Out of scope
Multimodal vision, OCR, deep synthesis. Keep deepseek-ocr:3b and muse-glimmer:30b for those.

## Implementation
- Add bespoke-nimble-9b to config/smackerel.yaml ollama_fast_model
- Update prompt contracts to prefer fast model for classification tasks
- Benchmark schema validity vs qwen2.5:0.5b-instruct

## Success criteria
- Schema validity >98%
- P95 inference <800ms on target hardware
- No regression in graph linking recall
