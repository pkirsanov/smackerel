// Package recipesearch is the BUG-061-003 capability-layer skill
// for the recipe_search scenario.
//
// The package owns:
//
//   - assembler.go — the contracts.SourceAssembler that translates
//     the LLM Final ({answer, cited_artifact_ids}) into Sources.
//     On a zero-hit Final (empty answer + empty cited_artifact_ids)
//     the assembler emits a deterministic StatusUnavailable response
//     override (CaptureRoute=false, actionable body) so the empty-
//     graph path does NOT fall through to the BandLow "saved as
//     idea" capture branch.
//
// The agent-side tool that actually executes the search lives in
// internal/agent/tools/recipesearch (per the Spec 037 substrate
// separation — internal/agent stays substrate-only). The scenario
// itself is defined in YAML at config/prompt_contracts/recipe-search-v1.yaml
// and loaded by the existing scenario loader; this package does not
// register a Scenario struct directly.
package recipesearch
