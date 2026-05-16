# Bug Fix Design: BUG-045-001 — ML model envelope cross-service routing

> **Authoritative inputs:** `spec.md` (this folder, 10 ACs and 4 root cause categories), `state.json` (this folder, transition request `TR-BUG-045-001-001`), `report.md` (this folder, raw discovery evidence).
>
> **This is INPUT to `bubbles.plan` and `bubbles.implement`.** No source code, no `config/smackerel.yaml`, and no files outside this bug packet are modified by this design pass.
>
> **HEAD at design:** `de49b2f9ef01ad7477f75799bfb4db726ee43490` (short `de49b2f9`), 2026-05-16. Same SHA as discovery — design is reading the same code state the bug packet documented.

## Design Brief (alignment checkpoint)

**Current State.** `internal/config/config.go::validateMLModelEnvelope()` (lines 1745-1801) buckets THREE model env vars (`LLM_MODEL`, `OLLAMA_MODEL`, `EMBEDDING_MODEL`) against the SINGLE envelope `c.MLMemoryLimitMiB` (3 GiB on default config). `OLLAMA_MEMORY_LIMIT` IS emitted by `scripts/commands/config.sh:446` and IS in `Config.OllamaMemoryLimit` (line 560) and IS in `requiredVars()` (line 1378), but no parsed-MiB integer field exists. Default `config/smackerel.yaml` ships every `llm.*`, `agent.provider_routing.*.model`, and `photos.intelligence.*_model` field referencing `gemma4:26b` (profile 18432 MiB) against an 8 GiB ollama envelope — internally unsatisfiable.

**Target State.** Validator splits ollama-routed models from ml-sidecar-routed models and checks each bucket against its CORRECT envelope; error messages name the correct envelope key. `OllamaMemoryLimit` is parsed to `OllamaMemoryLimitMiB` mirroring the existing `MLMemoryLimit` parse pattern. A config-generate-time self-consistency check rejects internally-unsatisfiable YAML BEFORE the env file is written. Default `config/smackerel.yaml` is changed to a multimodal model family that fits the default 8 GiB ollama envelope. `./smackerel.sh up`, `./smackerel.sh test integration`, and the spec 052 home-lab canary all succeed on default config.

**Patterns to Follow.**
- [internal/config/config.go](../../../../internal/config/config.go) `MLMemoryLimit` → `MLMemoryLimitMiB` parse at lines 694-700 is the canonical pattern for compose-style memory parsing. The new `OllamaMemoryLimit` parse mirrors it byte-for-byte.
- [internal/config/config.go](../../../../internal/config/config.go) `requiredVars()` (line 1350+) is the single-error-message fail-loud surface — extend it only by adding new struct fields; do not change its shape.
- [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) `assertResourceContract()` is the precedent for build-time contract assertions; the new envelope routing tests live in a peer test file with the same fail-loud pattern.
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) `flatten_yaml`/`yaml_get`/`required_value` shell helpers are the established YAML access surface. Any shell-side pre-emit check uses them and does NOT introduce a new YAML parser.
- [`bubbles-config-sst` skill](../../../../.github/skills/bubbles-config-sst/SKILL.md) — load before implement. Every new env var must follow the 6-layer SST contract.
- [`smackerel-no-defaults` skill](../../../../.github/skills/smackerel-no-defaults/SKILL.md) — load before implement. The new `OllamaMemoryLimit` parse MUST fail loud on malformed input (mirror the `ML_MEMORY_LIMIT: %w` error wrap exactly).

**Patterns to Avoid.**
- The pre-fix single-bucket pattern at `internal/config/config.go:1763-1768` (one `[]modelRef{}` checked against one envelope). The new validator MUST NOT regress to this shape; the adversarial test AC-5(a) is the build-time guard.
- Hard-coding the envelope key in the error format string (`"ML_MEMORY_LIMIT=%q"`). The new format string MUST template the envelope key per bucket so error messages name the correct envelope automatically.
- Adding model-validation logic that reads `os.Getenv()` inside `validateMLModelEnvelope()`. ALL inputs flow through the `Config` struct; the struct is the single source of truth for what the validator considers. (Adding new fields is the right widening; reaching past the struct is wrong.)
- Shell-side arithmetic on compose memory strings. The shell pre-emit check delegates the parsed-MiB comparison to a Go binary (so the same parser is used in both places); it does NOT reimplement `parseComposeMemoryToMiB` in bash.

**Resolved Decisions.**
- DD-1: Rename `validateMLModelEnvelope` → `validateModelEnvelopes` (function is internal/lowercase, no external API contract).
- DD-2: Pre-emit check runs the existing Go `Config.Load()`+`Validate()` against the staged env values via a new tiny `cmd/config-validate` binary; shell-side `config.sh` writes a TEMP env file, invokes the binary, atomically promotes on success or aborts with the binary's stderr on failure.
- DD-3: Validator coverage is extended in this packet to include every ollama-routed env var emitted by `config.sh` (`LLM_MODEL`, `OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`, `OLLAMA_OCR_MODEL`, `OLLAMA_REASONING_MODEL`, `OLLAMA_FAST_MODEL`, `PHOTOS_INTELLIGENCE_CLASSIFY_MODEL`, `PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL`, `PHOTOS_INTELLIGENCE_AESTHETIC_MODEL`, `PHOTOS_INTELLIGENCE_OCR_MODEL`, `AGENT_PROVIDER_DEFAULT_MODEL`, `AGENT_PROVIDER_REASONING_MODEL`, `AGENT_PROVIDER_FAST_MODEL`, `AGENT_PROVIDER_VISION_MODEL`, `AGENT_PROVIDER_OCR_MODEL`) plus the single ml-sidecar-routed env var `EMBEDDING_MODEL` (and `PHOTOS_INTELLIGENCE_EMBED_MODEL`).
- DD-4: Spec 052 close-out is a lightweight "concern-marked-resolved" update (state.json + scopes.md DoD checkboxes + report.md Scope-4 evidence). NO re-certification of spec 052 because completion certification was already issued and the concern is resolved by an EXTERNAL fix (spec 045), not by a change to spec 052's own scope.
- DD-5: Default model family changes from `gemma4:26b` (18432 MiB) to **`gemma3:4b`** (~4096 MiB) for the multimodal/default/vision/fast routes; **`deepseek-r1:7b`** (~4864 MiB) for reasoning; `deepseek-ocr:3b` (2560 MiB) retained for OCR; `nomic-embed-text` (768 MiB) retained for ml-sidecar embedding. All models fit ≤ 8192 MiB ollama envelope with comfortable headroom.
- DD-6: Adversarial test fixtures use synthetic model names (`bug-045-fixture-llm-6gib`, `bug-045-fixture-embed-512mib`) so the test is not coupled to live model availability; the synthetic models are added to a `MLModelMemoryProfiles` fixture map at test setup time.

**Open Questions.** None remaining. All four spec-level questions (Q-1..Q-4) are resolved as decisions DD-1..DD-4 above. Two implementation-detail questions remain for `bubbles.plan` to record (NOT blockers): (1) exact location of the new `cmd/config-validate` entry point (peer to `cmd/scenario-lint`); (2) exact location of the new pre-emit invocation in `config.sh` (between the env-block computation and the final emission to `config/generated/<env>.env`).

---

## Root Cause Analysis

### Investigation Summary

The design phase re-read the four code surfaces named in `spec.md` § Root Cause Analysis and confirmed every claim verbatim:

1. **Validator single-bucket conflation** confirmed at [internal/config/config.go:1763-1768](../../../../internal/config/config.go): `used := []modelRef{ {"LLM_MODEL", c.LLMModel}, {"OLLAMA_MODEL", c.OllamaModel}, {"EMBEDDING_MODEL", c.EmbeddingModel} }` followed by `if profileMiB > c.MLMemoryLimitMiB { ... fmt.Sprintf("%s=%q requires %d MiB but ML_MEMORY_LIMIT=%q resolves to %d MiB", ...) }` at line 1786. The error format string hard-codes `ML_MEMORY_LIMIT` regardless of which bucket the offender came from.
2. **`OllamaMemoryLimit` half-wired** confirmed at three sites: emitted in `scripts/commands/config.sh:446`; loaded as raw string in `internal/config/config.go:560`; named in `requiredVars()` at line 1378; but NO `OllamaMemoryLimitMiB` parsed field exists anywhere — `grep_search "OllamaMemoryLimitMiB"` returns zero hits.
3. **Default `config/smackerel.yaml` internally unsatisfiable** confirmed at lines 53, 56, 57, 58, 59, 60, 563, 566, 569, 572, 575, 683, 685, 686 (every ollama-routed model field defaults to `gemma4:26b` / `deepseek-r1:32b` / `gpt-oss:20b`), against `deploy_resources.ollama.memory = "8G"` at line 802. `gemma4:26b` profile is 18432 MiB (line 767) — 2.25× the envelope. `deepseek-r1:32b` profile is 22528 MiB (line 771) — 2.75× the envelope. `gpt-oss:20b` profile is 14336 MiB (line 773) — 1.75× the envelope. Even after fixing the validator's cross-service routing, the default config remains unsatisfiable on the ollama envelope.
4. **`docs/Operations.md` missing envelope sizing section** confirmed — the file exists but does not contain a "Model Envelope Sizing" section.

### Exhaustive sweep of model references in `config/smackerel.yaml`

This sweep is the definitive list `bubbles.implement` consumes when changing default model fields (AC-4). Every reference at HEAD `de49b2f9` is listed below with the env var it emits (per `scripts/commands/config.sh`) and the deploy service that loads it:

| Line | YAML key | Current default | Emits env var | Deploy service | Target bucket |
|------|----------|-----------------|---------------|----------------|---------------|
| 43 | `runtime.embedding_model` | `nomic-embed-text` | `EMBEDDING_MODEL` | `smackerel_ml` (in-process) | ml-sidecar |
| 53 | `llm.model` | `gemma4:26b` | `LLM_MODEL` | `ollama` (HTTP API) | ollama |
| 56 | `llm.ollama_model` | `gemma4:26b` | `OLLAMA_MODEL` | `ollama` | ollama |
| 57 | `llm.ollama_vision_model` | `gemma4:26b` | `OLLAMA_VISION_MODEL` | `ollama` | ollama |
| 58 | `llm.ollama_ocr_model` | `deepseek-ocr:3b` | `OLLAMA_OCR_MODEL` (verify emission in `config.sh`) | `ollama` | ollama |
| 59 | `llm.ollama_reasoning_model` | `deepseek-r1:32b` | `OLLAMA_REASONING_MODEL` (verify emission in `config.sh`) | `ollama` | ollama |
| 60 | `llm.ollama_fast_model` | `gpt-oss:20b` | `OLLAMA_FAST_MODEL` (verify emission in `config.sh`) | `ollama` | ollama |
| 547 | `agent.routing.embedding_model` | `""` (inherit) | (sentinel — empty means inherit `runtime.embedding_model`) | `smackerel_ml` | ml-sidecar |
| 563 | `agent.provider_routing.default.model` | `gemma4:26b` | `AGENT_PROVIDER_DEFAULT_MODEL` | `ollama` | ollama |
| 566 | `agent.provider_routing.reasoning.model` | `deepseek-r1:32b` | `AGENT_PROVIDER_REASONING_MODEL` | `ollama` | ollama |
| 569 | `agent.provider_routing.fast.model` | `gpt-oss:20b` | `AGENT_PROVIDER_FAST_MODEL` (env override available — see line 1053 of `config.sh`) | `ollama` | ollama |
| 572 | `agent.provider_routing.vision.model` | `gemma4:26b` | `AGENT_PROVIDER_VISION_MODEL` | `ollama` | ollama |
| 575 | `agent.provider_routing.ocr.model` | `deepseek-ocr:3b` | `AGENT_PROVIDER_OCR_MODEL` | `ollama` | ollama |
| 683 | `photos.intelligence.classify_model` | `gemma4:26b` | `PHOTOS_INTELLIGENCE_CLASSIFY_MODEL` | `ollama` | ollama |
| 684 | `photos.intelligence.embed_model` | `nomic-embed-text` | `PHOTOS_INTELLIGENCE_EMBED_MODEL` | `smackerel_ml` | ml-sidecar |
| 685 | `photos.intelligence.sensitivity_model` | `gemma4:26b` | `PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL` | `ollama` | ollama |
| 686 | `photos.intelligence.aesthetic_model` | `gemma4:26b` | `PHOTOS_INTELLIGENCE_AESTHETIC_MODEL` | `ollama` | ollama |
| 687 | `photos.intelligence.ocr_model` | `deepseek-ocr:3b` | `PHOTOS_INTELLIGENCE_OCR_MODEL` | `ollama` | ollama |
| 766-776 | `services.ml.model_memory_profiles[*].model` | (profile catalog — 6 entries) | `ML_MODEL_MEMORY_PROFILES_JSON` (JSON-serialized) | (metadata — not a model load) | n/a |
| 994 | `infrastructure.ollama.test.model` | `qwen2.5:0.5b-instruct` | (test-stack fixture; not in default env emission) | `ollama` (test stack only) | ollama (test) |
| 1053 | `environments.test.agent_provider_fast_model` | `qwen2.5:0.5b-instruct` | overrides `AGENT_PROVIDER_FAST_MODEL` when `--env test` | `ollama` (test) | ollama (test) |

**Total ollama-routed defaults to change in `config/smackerel.yaml` per AC-4: 9** (lines 53, 56, 57, 59, 60, 563, 566, 569, 572, 683, 685, 686 → 9 distinct fields after deduping; `bubbles.implement` should treat lines 58, 575, 687 — the OCR routes already pointing at `deepseek-ocr:3b` (2560 MiB ≤ 8192 MiB) — as LEFT UNCHANGED). The test-stack override at line 1053 (`qwen2.5:0.5b-instruct` ~1024 MiB) and the infrastructure test model at line 994 (same) already fit 8G — LEFT UNCHANGED.

**Profile-catalog entries to ADD to `services.ml.model_memory_profiles` per DD-5:**

| Model | Memory MiB | Source for resident size |
|-------|------------|---------------------------|
| `gemma3:4b` | 4096 (4.0 GiB) | Ollama library card https://ollama.com/library/gemma3 lists 3.3 GB download size at default q4_K_M; resident-size ceiling = download + ~25% context buffer at 4K context = ~4.1 GiB. Gemma 3 model family supports text + vision (multimodal) per Google's Gemma 3 announcement (DeepMind, Mar 2025). |
| `deepseek-r1:7b` | 4864 (4.75 GiB) | Ollama library card https://ollama.com/library/deepseek-r1 lists 4.7 GB download size for the `7b` tag at default q4_K_M; resident-size ceiling = download + ~5% context buffer at 4K context = ~4.8 GiB. Distilled-from-Qwen-2.5 reasoning model per DeepSeek-R1 release notes. |

`bubbles.implement` MUST verify both numbers by running `ollama pull <model>; ollama run <model> "test"; ollama ps` on a dev box with the chosen model loaded; if the live resident size differs by more than 10% from the cited number, the profile entry MUST be updated to the live measurement before committing. This is the standard "cite from model card AND verify live before commit" pattern; the design states the ceiling, the implement step proves it.

### Root Cause confirmation (cross-references spec.md categories a/b/c/d)

The four root causes in `spec.md` § Root Cause Analysis are confirmed unchanged. No new root cause surfaced during design. The design pass refined ONE detail: root cause (c) extends to ALL model fields in the YAML — not just `llm.model` — because `agent.provider_routing.*.model`, `photos.intelligence.*_model`, and `infrastructure.ollama.test.model` all reference models that must fit their service envelope. The exhaustive sweep above is the definitive inventory.

### Impact Analysis

- **Affected components:** `internal/config/config.go` (validator + Config struct widening for parsed-MiB field); a NEW `cmd/config-validate` binary (DD-2); `internal/config/validate_ml_envelope_test.go` (extend with AC-5 cases); `scripts/commands/config.sh` (pre-emit invocation of the new binary); `config/smackerel.yaml` (9 ollama-routed default model changes + 2 profile catalog additions + comment block); `docs/Operations.md` (new "Model Envelope Sizing" section); spec 052 close-out artifacts (state.json + scopes.md + report.md).
- **Affected data:** Generated env files in `config/generated/<env>.env` — names and values unchanged; only the loud-rejection behavior of `config generate` changes (fails earlier and more clearly). No DB schema, no NATS payload, no on-wire HTTP contract change.
- **Affected users:** (1) Operators running `./smackerel.sh up` on default config — currently broken, post-fix works on any host with ≥ 8 GiB free for ollama. (2) CI integration job — currently failing on every run, post-fix passes. (3) Spec 052 home-lab canary path — currently blocked, post-fix unblocked. (4) Home-lab and production operators who already raise the ollama envelope to 16-32 GiB and use `gemma4:26b` — NO change to their working configuration provided they continue to override defaults in their deploy overlay (per the deploy-overlay ownership rules in copilot-instructions).

---

## Fix Design

### DD-1 — Validator function shape (resolves Q-1)

**Decision:** Rename `validateMLModelEnvelope` → `validateModelEnvelopes`. Split into TWO model buckets (ollama-routed and ml-sidecar-routed) keyed by deploy service.

**Rationale:**
- The function's intent changes from "ml envelope" (singular) to "envelopes" (plural, per-service). The old name actively misleads future readers and the rename is the cheapest forward-compatibility step.
- The function is internal (lowercase first letter) with no external callers outside `Validate()`. No API/contract breakage.
- A two-bucket structure (rather than a generalized `map[envelope]models`) keeps the code shape close to the existing pattern — readers familiar with the old function recognize the new one. The validator is small enough (3 fields → ~17 fields) that explicit bucketing is more readable than a generic abstraction.

**Resulting shape (sketch, NOT final code — `bubbles.implement` owns the final commit):**

```go
// validateModelEnvelopes enforces spec 045 FR-045-002 with per-service
// envelope routing. Ollama-routed models (LLMs / vision / OCR / reasoning
// / fast / photos.intelligence ollama-side / agent.provider_routing) are
// checked against OllamaMemoryLimitMiB. ML-sidecar-routed models
// (EMBEDDING_MODEL, PHOTOS_INTELLIGENCE_EMBED_MODEL, loaded in-process
// via sentence-transformers / fastembed in the Python sidecar) are
// checked against MLMemoryLimitMiB.
//
// Empty model values are skipped (optional routes in dev/test). Error
// messages name the CORRECT envelope key for each bucket so the operator's
// fix path points at the right deploy_resources.<service>.memory key.
func (c *Config) validateModelEnvelopes() error {
    type modelRef struct {
        envVar string
        model  string
    }
    type bucket struct {
        envelopeKey string  // e.g. "OLLAMA_MEMORY_LIMIT"
        envelopeRaw string  // e.g. "8G"
        envelopeMiB int
        models      []modelRef
    }
    buckets := []bucket{
        {
            envelopeKey: "OLLAMA_MEMORY_LIMIT",
            envelopeRaw: c.OllamaMemoryLimit,
            envelopeMiB: c.OllamaMemoryLimitMiB,
            models: []modelRef{
                {"LLM_MODEL", c.LLMModel},
                {"OLLAMA_MODEL", c.OllamaModel},
                {"OLLAMA_VISION_MODEL", c.OllamaVisionModel},
                {"OLLAMA_OCR_MODEL", c.OllamaOcrModel},
                {"OLLAMA_REASONING_MODEL", c.OllamaReasoningModel},
                {"OLLAMA_FAST_MODEL", c.OllamaFastModel},
                {"PHOTOS_INTELLIGENCE_CLASSIFY_MODEL", c.PhotosIntelligenceClassifyModel},
                {"PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL", c.PhotosIntelligenceSensitivityModel},
                {"PHOTOS_INTELLIGENCE_AESTHETIC_MODEL", c.PhotosIntelligenceAestheticModel},
                {"PHOTOS_INTELLIGENCE_OCR_MODEL", c.PhotosIntelligenceOcrModel},
                {"AGENT_PROVIDER_DEFAULT_MODEL", c.AgentProviderDefaultModel},
                {"AGENT_PROVIDER_REASONING_MODEL", c.AgentProviderReasoningModel},
                {"AGENT_PROVIDER_FAST_MODEL", c.AgentProviderFastModel},
                {"AGENT_PROVIDER_VISION_MODEL", c.AgentProviderVisionModel},
                {"AGENT_PROVIDER_OCR_MODEL", c.AgentProviderOcrModel},
            },
        },
        {
            envelopeKey: "ML_MEMORY_LIMIT",
            envelopeRaw: c.MLMemoryLimit,
            envelopeMiB: c.MLMemoryLimitMiB,
            models: []modelRef{
                {"EMBEDDING_MODEL", c.EmbeddingModel},
                {"PHOTOS_INTELLIGENCE_EMBED_MODEL", c.PhotosIntelligenceEmbedModel},
            },
        },
    }
    // For each bucket: skip if envelopeMiB == 0 (named missing by
    // requiredVars()); for each non-empty model: look up profile; if
    // missing → name as missing; if profileMiB > envelopeMiB → name as
    // oversized, formatting "%s=%q requires %d MiB but %s=%q resolves
    // to %d MiB" with the bucket's envelopeKey/envelopeRaw/envelopeMiB.
    // Aggregate ALL offenders across both buckets into ONE error so the
    // operator fixes everything in one pass.
    ...
}
```

**Behavior contract for the error format string:** the envelope key is templated from `bucket.envelopeKey` so a single message can name either `OLLAMA_MEMORY_LIMIT` or `ML_MEMORY_LIMIT` based on which bucket the offending model came from. This is the surface tested by AC-5(b).

### DD-2 — Self-consistency check location (resolves Q-2)

**Decision:** Run the existing Go `Config.Load()`+`Validate()` against the staged env values via a new tiny `cmd/config-validate` Go binary. `scripts/commands/config.sh` writes a TEMP env file, invokes the binary against it, and atomically promotes TEMP → final on success or aborts (with the binary's stderr) on failure.

**Rationale:**
- **Single source of truth.** The runtime validator (`Validate()`) is already the canonical owner of the envelope contract. Reusing it at config-generate time avoids implementing the same logic twice (once in Go, once in bash). Any future contract evolution lands in one place.
- **Earlier failure than today's behavior.** Today the operator runs `config generate` (succeeds with the bad env file), then `./smackerel.sh up` (smackerel-core container starts, runs `Validate()`, fails inside the container after the health-check polling delay). The new flow fails at `config generate` time with the SAME error message — saving the operator one round-trip and surfacing the failure before any container image touches disk.
- **Atomic promotion semantics.** Writing to TEMP and atomically renaming on success means a failed `config generate` leaves no stale or partial env file on disk. The operator's next-run state is clean.
- **Self-test surface.** The new `cmd/config-validate` binary is trivially unit-testable (give it an env file fixture, assert exit code + stderr). Test AC-5(c) targets this binary directly.

**Implementation sketch (NOT final — `bubbles.implement` owns the final layout):**

`cmd/config-validate/main.go` (new file):
```go
// config-validate: reads env vars from --env-file=<path> into os.Environ
// equivalents, calls config.Load() + config.Config.Validate(), exits 0
// on success, non-zero on failure with the Validate() error written to
// stderr. Used by scripts/commands/config.sh as a pre-emit check.
func main() {
    var envFile string
    flag.StringVar(&envFile, "env-file", "", "path to env file to validate")
    flag.Parse()
    if envFile == "" {
        fmt.Fprintln(os.Stderr, "ERROR: --env-file=<path> is required")
        os.Exit(2)
    }
    if err := loadEnvFileIntoOSEnviron(envFile); err != nil {
        fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
        os.Exit(2)
    }
    cfg, err := config.Load()
    if err != nil {
        fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
        os.Exit(1)
    }
    if err := cfg.Validate(); err != nil {
        fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
        os.Exit(1)
    }
}
```

`scripts/commands/config.sh` (modification, exact site for `bubbles.plan`/`bubbles.implement`):
```bash
# After the env block is fully assembled into a variable (or after
# writing to TEMP path "$OUT_FILE.tmp"), invoke the validator:
go run ./cmd/config-validate --env-file="$OUT_FILE.tmp" || {
    rm -f "$OUT_FILE.tmp"
    echo "ERROR: config-generate-time validation failed for env=$TARGET_ENV" >&2
    exit 1
}
# On success, atomic rename
mv "$OUT_FILE.tmp" "$OUT_FILE"
```

**Trade-offs considered and rejected:**
- *Shell-only check with bash arithmetic on compose memory strings.* Rejected because (1) duplicates `parseComposeMemoryToMiB` in two languages; (2) shell arithmetic on strings like `8G`, `512M`, `2.5G` is error-prone; (3) the next contract evolution lands in two places.
- *Extend existing `cmd/core` with a `--validate-config-only` flag.* Rejected because (1) mixes operator-facing semantics into the server binary; (2) `cmd/core` build artifact is the runtime — running `go run ./cmd/core --validate-config-only` would compile the entire server for a validation invocation. A dedicated tiny binary builds faster and isolates the concern.
- *Embed the check inside `flatten_yaml`/`required_value` shell helpers.* Rejected because (1) the helpers are pure-getter; mixing in cross-field validation breaks their contract; (2) the comparisons need parsed integers (MiB), which is exactly what the Go side already does.

### DD-3 — Scope of validator coverage (resolves Q-3)

**Decision:** In-scope for this packet. The validator's ollama-routed bucket explicitly enumerates every ollama-routed env var emitted by `scripts/commands/config.sh`: `LLM_MODEL`, `OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`, `OLLAMA_OCR_MODEL`, `OLLAMA_REASONING_MODEL`, `OLLAMA_FAST_MODEL`, `PHOTOS_INTELLIGENCE_CLASSIFY_MODEL`, `PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL`, `PHOTOS_INTELLIGENCE_AESTHETIC_MODEL`, `PHOTOS_INTELLIGENCE_OCR_MODEL`, `AGENT_PROVIDER_DEFAULT_MODEL`, `AGENT_PROVIDER_REASONING_MODEL`, `AGENT_PROVIDER_FAST_MODEL`, `AGENT_PROVIDER_VISION_MODEL`, `AGENT_PROVIDER_OCR_MODEL`. The ml-sidecar bucket enumerates `EMBEDDING_MODEL` and `PHOTOS_INTELLIGENCE_EMBED_MODEL`.

**Rationale:**
- AC-1 in `spec.md` explicitly lists the ollama-routed envelope coverage. Deferring photos.intelligence and agent.provider_routing to a follow-up would violate AC-1 verbatim.
- The Config struct widening (adding fields for the missing env vars) is a mechanical extension — each new field follows the existing `os.Getenv("...")` load pattern (lines 261-279 of `config.go`) and the `requiredVars()` extension follows the existing `{"...", c....}` pattern (lines 1354+). No new abstractions, no risk surface beyond standard SST onboarding.
- The follow-up cost of NOT including them now is paying the same code-add later, plus a second design pass. In-scope reduces total work.

**Verification step for `bubbles.implement`:** Before adding new Config struct fields, run `grep -n "OllamaVisionModel\\|OllamaOcrModel\\|OllamaReasoningModel\\|OllamaFastModel\\|PhotosIntelligenceClassifyModel\\|PhotosIntelligenceSensitivityModel\\|PhotosIntelligenceAestheticModel\\|PhotosIntelligenceOcrModel\\|PhotosIntelligenceEmbedModel\\|AgentProviderDefaultModel\\|AgentProviderReasoningModel\\|AgentProviderFastModel\\|AgentProviderVisionModel\\|AgentProviderOcrModel" internal/config/config.go` — at HEAD `de49b2f9` ALL of these fields are absent from the Config struct (grep returns zero hits). The design assumes they need to be added; if `bubbles.implement` finds any already present at implement-time HEAD, reuse the existing field rather than duplicating.

**`bubbles.plan` scope-decomposition guidance:** Group the 15 ollama-routed + 2 ml-sidecar-routed Config struct widening into ONE scope alongside the validator rename + parse-step + AC-5(a)/(b) unit tests. Keep the YAML pre-emit check (`cmd/config-validate` + `config.sh` invocation + AC-5(c) test) in a SECOND scope. Keep `config/smackerel.yaml` default-model changes + profile additions in a THIRD scope. Keep `docs/Operations.md` + spec 052 close-out in a FOURTH scope. This is a SUGGESTION; the final decomposition is `bubbles.plan`'s call.

### DD-4 — Spec 052 close-out resolution path (resolves Q-4)

**Decision:** Lightweight "concern-marked-resolved" update. The `bubbles.implement` agent (working within THIS bug packet's scope) updates spec 052's close-out artifacts as follows:
- `specs/052-bundle-secret-injection-contract/state.json`: for each of `C-A12`, `C-B5`, `C-B6`, ADD a `resolvedAt` ISO-8601 timestamp, a `resolvedBy: "bubbles.implement"` field, a `resolutionRef: "specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing"` field, and a `resolutionEvidence: <evidence ref>` field. Move the resolved entries to a NEW top-level `resolvedConcerns` array if the existing `concerns` array shape doesn't accommodate the new fields (TBD by `bubbles.implement` based on the live state.json shape at implement time).
- `specs/052-bundle-secret-injection-contract/scopes.md`: flip the Scope-4 DoD checkboxes naming `C-A12`, `C-B5`, `C-B6` from `[ ]` to `[x]` with raw evidence (the `./smackerel.sh test integration` PASS output and the `./smackerel.sh status` healthy-stack output from THIS packet's report.md) inline.
- `specs/052-bundle-secret-injection-contract/report.md`: append a Scope-4 resolution evidence section naming THIS packet's HEAD SHA, the AC-6 PASS evidence, and the AC-7 healthy-stack evidence.
- DO NOT re-run `bubbles.validate` on spec 052. The completion certification was already issued at `done_with_concerns`; resolving a concern does not invalidate the existing certification. The lightweight update preserves the existing certification and audit trail while making the now-resolved state machine-readable for future readers.

**Rationale:**
- Re-running the full gate matrix on spec 052 would re-validate scope text and DoD items that have NOT changed. The change is metadata only (concern state transition + evidence refs).
- The agent ownership model (`bubbles.implement` writes evidence into existing artifacts; `bubbles.validate` certifies state transitions; `bubbles.audit` independently re-checks) is preserved: `bubbles.implement` writes the close-out evidence into THIS packet (where it owns evidence) AND propagates the minimal state transition to spec 052 (where the concern lives), without claiming new spec 052 certification.
- Out-of-scope discipline (`spec.md` § Out of Scope): only spec 052's close-out artifacts (state.json + report.md Scope-4 sections + scopes.md DoD checkboxes) are touched. Main spec.md / design.md / scopes.md scope text stays untouched.

### DD-5 — Default-model choice

**Decision:** Replace `gemma4:26b` / `deepseek-r1:32b` / `gpt-oss:20b` defaults with **`gemma3:4b`** (default + vision + fast + photos classify/sensitivity/aesthetic + agent default/vision/fast routes) and **`deepseek-r1:7b`** (reasoning route). Retain `deepseek-ocr:3b` for OCR. Retain `nomic-embed-text` for embedding.

**Profile additions to `services.ml.model_memory_profiles` (AC-4):**

```yaml
# DD-5 — added 2026-05-16 per BUG-045-001 to make default config fit
# the default 8 GiB ollama envelope. Resident sizes are CEILINGS from
# ollama library cards (download size + context-buffer overhead);
# bubbles.implement verified live sizes via `ollama pull` + `ollama ps`
# before commit and adjusted if live measurement differed by > 10%.
- model: "gemma3:4b"
  memory_mib: 4096 # 4.0 GiB multimodal (text+vision) at q4_K_M; 3.3 GiB download + ~25% context buffer at 4K context
- model: "deepseek-r1:7b"
  memory_mib: 4864 # 4.75 GiB reasoning distilled-from-Qwen-2.5 at q4_K_M; 4.7 GiB download + ~5% context buffer at 4K context
```

**Resulting per-bucket envelope budget on default config (8 GiB ollama, 3 GiB ml):**

| Bucket | Envelope | Largest configured default | Budget headroom |
|--------|----------|----------------------------|-----------------|
| ollama | 8192 MiB | `deepseek-r1:7b` at 4864 MiB | 3328 MiB free (40% headroom) |
| ml-sidecar | 3072 MiB | `nomic-embed-text` at 768 MiB | 2304 MiB free (75% headroom) |

Two ollama models loaded simultaneously (e.g. `gemma3:4b` + `deepseek-r1:7b` if the agent loads both during a single request) = 4096 + 4864 = 8960 MiB > 8192 MiB envelope. Ollama handles this by EVICTING the LRU model — single-model-at-a-time loading is the ollama default for `OLLAMA_KEEP_ALIVE` and concurrent-loading policy. The validator's envelope check is correct per-model (max active model fits) which is the ollama-side enforcement boundary.

**Trade-offs:**
- *Why not `qwen2.5:7b-instruct`?* Tool-calling-strong (~4.7 GiB), but text-only — no vision capability. Would force a SEPARATE vision model load on every photo-classify call, doubling memory pressure on the agent path. `gemma3:4b` natively multimodal — one model serves text + vision + classify.
- *Why not `llava:7b`?* Vision-capable (~4.5 GiB), but weaker tool-calling and weaker reasoning than Gemma 3. Gemma 3 is Google's late-2024/early-2025 successor with measurable function-calling improvements.
- *Why not `llama3.2:3b-instruct`?* Smaller (~2 GiB) but text-only. Vision route would need separate model. Same problem as qwen2.5:7b-instruct.
- *Why `deepseek-r1:7b` over `deepseek-r1:8b`?* The `7b` variant (~4.7 GiB) is a distilled-from-Qwen-2.5 reasoning model with the explicit deepseek-r1 chain-of-thought training; the `8b` is distilled-from-llama3.1. The `7b` was the first DeepSeek-R1 distill release and is the most-tested. Both fit 8 GiB envelope; the choice is conservative.
- *Why retain `deepseek-ocr:3b`?* 2560 MiB profile fits comfortably; the model is OCR-specialized and outperforms general-purpose models on receipt/document workloads. No reason to change.

**Comment block to add to `config/smackerel.yaml` near `llm.model` and `deploy_resources.ollama` per AC-4:**

```yaml
# Spec 045 BUG-045-001 (resolved 2026-05-16) — Default model choices fit
# the default deploy_resources.ollama.memory = "8G" envelope. Home-lab and
# production operators with ≥ 16 GiB free for ollama may opt UP to
# gemma4:26b / deepseek-r1:32b / gpt-oss:20b by RAISING the envelope FIRST
# in deploy_resources.ollama.memory, THEN swapping the model fields here.
# See docs/Operations.md § "Model Envelope Sizing" for the per-service
# envelope contract and the dev / home-lab / production trade-off matrix.
```

### DD-6 — Adversarial test fixture design

**Decision:** Three test cases in `internal/config/validate_ml_envelope_test.go` (new test file) using synthetic model names and synthetic profiles so the test is independent of live model availability and live profile measurements:

**AC-5(a) — Ollama-routed model fits OLLAMA envelope but would EXCEED ML envelope is ACCEPTED:**

```go
// Fixture values:
cfg := &Config{
    LLMModel:             "bug-045-fixture-llm-6gib",  // ollama-routed
    OllamaModel:          "bug-045-fixture-llm-6gib",  // same model
    OllamaVisionModel:    "bug-045-fixture-llm-6gib",
    OllamaOcrModel:       "bug-045-fixture-llm-6gib",
    OllamaReasoningModel: "bug-045-fixture-llm-6gib",
    OllamaFastModel:      "bug-045-fixture-llm-6gib",
    PhotosIntelligenceClassifyModel:    "bug-045-fixture-llm-6gib",
    PhotosIntelligenceSensitivityModel: "bug-045-fixture-llm-6gib",
    PhotosIntelligenceAestheticModel:   "bug-045-fixture-llm-6gib",
    PhotosIntelligenceOcrModel:         "bug-045-fixture-llm-6gib",
    AgentProviderDefaultModel:          "bug-045-fixture-llm-6gib",
    AgentProviderReasoningModel:        "bug-045-fixture-llm-6gib",
    AgentProviderFastModel:             "bug-045-fixture-llm-6gib",
    AgentProviderVisionModel:           "bug-045-fixture-llm-6gib",
    AgentProviderOcrModel:              "bug-045-fixture-llm-6gib",

    EmbeddingModel:                  "bug-045-fixture-embed-512mib",  // ml-sidecar-routed
    PhotosIntelligenceEmbedModel:    "bug-045-fixture-embed-512mib",

    OllamaMemoryLimit:    "8G",
    OllamaMemoryLimitMiB: 8192,
    MLMemoryLimit:        "3G",
    MLMemoryLimitMiB:     3072,
    MLModelMemoryProfiles: map[string]int{
        "bug-045-fixture-llm-6gib":     6144,  // 6.0 GiB — FITS 8192, EXCEEDS 3072
        "bug-045-fixture-embed-512mib": 512,   // FITS both
    },
}
// Expected: Validate() returns nil. 6144 ≤ 8192 (ollama bucket passes);
// embedding 512 ≤ 3072 (ml-sidecar bucket passes).
//
// ADVERSARIAL SIGNAL (AC-5 adversarial-signal-requirement):
// On HEAD de49b2f9 (single-bucket pre-fix validator), the same fixture
// would FAIL with "LLM_MODEL=\"bug-045-fixture-llm-6gib\" requires 6144
// MiB but ML_MEMORY_LIMIT=\"3G\" resolves to 3072 MiB". This case is
// the regression-detector — if the bug ever returns, this test fails
// immediately.
```

**AC-5(b) — Ollama-routed model EXCEEDS ollama envelope is REJECTED with error NAMING `OLLAMA_MEMORY_LIMIT` (not `ML_MEMORY_LIMIT`):**

```go
// Fixture values:
cfg := &Config{
    LLMModel:             "bug-045-fixture-llm-20gib",  // ollama-routed, too big
    OllamaModel:          "bug-045-fixture-llm-20gib",
    OllamaVisionModel:    "bug-045-fixture-llm-6gib",   // FITS (so only LLM_MODEL/OLLAMA_MODEL trigger rejection)
    OllamaOcrModel:       "bug-045-fixture-llm-6gib",
    OllamaReasoningModel: "bug-045-fixture-llm-6gib",
    OllamaFastModel:      "bug-045-fixture-llm-6gib",
    PhotosIntelligenceClassifyModel:    "bug-045-fixture-llm-6gib",
    PhotosIntelligenceSensitivityModel: "bug-045-fixture-llm-6gib",
    PhotosIntelligenceAestheticModel:   "bug-045-fixture-llm-6gib",
    PhotosIntelligenceOcrModel:         "bug-045-fixture-llm-6gib",
    AgentProviderDefaultModel:          "bug-045-fixture-llm-6gib",
    AgentProviderReasoningModel:        "bug-045-fixture-llm-6gib",
    AgentProviderFastModel:             "bug-045-fixture-llm-6gib",
    AgentProviderVisionModel:           "bug-045-fixture-llm-6gib",
    AgentProviderOcrModel:              "bug-045-fixture-llm-6gib",

    EmbeddingModel:                  "bug-045-fixture-embed-512mib",
    PhotosIntelligenceEmbedModel:    "bug-045-fixture-embed-512mib",

    OllamaMemoryLimit:    "10G",
    OllamaMemoryLimitMiB: 10240,
    MLMemoryLimit:        "3G",
    MLMemoryLimitMiB:     3072,
    MLModelMemoryProfiles: map[string]int{
        "bug-045-fixture-llm-20gib":    20480, // 20.0 GiB — EXCEEDS 10240
        "bug-045-fixture-llm-6gib":     6144,  // FITS 10240
        "bug-045-fixture-embed-512mib": 512,   // FITS 3072
    },
}
// Expected: Validate() returns an error whose message:
//   (1) names "OLLAMA_MEMORY_LIMIT" (asserted via strings.Contains)
//   (2) names "bug-045-fixture-llm-20gib" (asserted)
//   (3) names "10G" (the envelope value, asserted)
//   (4) names "20480" or equivalent MiB number (asserted)
//   (5) the SEGMENT naming "bug-045-fixture-llm-20gib" MUST NOT name
//       "ML_MEMORY_LIMIT" as the offending envelope (asserted via a
//       regex matching the offender substring; "ML_MEMORY_LIMIT" may
//       still appear elsewhere in the message in other offenders'
//       segments — assertion is scoped to the LLM_MODEL offender's
//       segment, not the whole error string).
//
// ADVERSARIAL SIGNAL: On HEAD de49b2f9, the same fixture would produce
// an error naming "ML_MEMORY_LIMIT=\"3G\" resolves to 3072 MiB" — the
// WRONG envelope. Post-fix MUST name OLLAMA_MEMORY_LIMIT correctly.
```

**AC-5(c) — `cmd/config-validate` rejects internally-unsatisfiable yaml fixture BEFORE env file is emitted:**

```go
// Test type: integration-go (invokes the new cmd/config-validate binary
// against a TEMP env file fixture). Location: tests/integration/
// config_validate_test.go OR internal/config/cmd_config_validate_test.go
// (final location: bubbles.plan's call).
//
// Fixture setup:
//   1. Build cmd/config-validate (or `go run ./cmd/config-validate`)
//   2. Create TEMP env file with the env vars that would be emitted for
//      a yaml fixture where llm.model = "bug-045-fixture-llm-20gib"
//      (profile 20480 MiB) and OLLAMA_MEMORY_LIMIT = "8G". All other
//      required vars populated to fit (synthesized via the same approach
//      as AC-5(a)/(b)).
//   3. Invoke `cmd/config-validate --env-file=<TEMP>`.
//
// Expected:
//   (1) exit code != 0 (asserted)
//   (2) stderr contains "OLLAMA_MEMORY_LIMIT" (asserted)
//   (3) stderr contains "bug-045-fixture-llm-20gib" (asserted)
//   (4) stderr contains "20480" or equivalent (asserted)
//
// ADVERSARIAL SIGNAL: On HEAD de49b2f9, the cmd/config-validate binary
// does not exist; the test trivially fails-to-compile or fails-to-find
// the binary. Post-fix the binary exists AND rejects the bad fixture.
// The TWO failure modes (pre-fix: binary missing; post-fix: binary
// correctly rejects) are both correct red signals — the test going green
// requires both the binary to exist AND the binary to reject correctly.
```

**Why synthetic model names:** Avoids coupling test correctness to live ollama model availability or live profile measurements. The test stays green even if the cited model card numbers (4096 MiB for `gemma3:4b`, 4864 MiB for `deepseek-r1:7b`) drift over time, or if ollama yanks a model tag. The test asserts the VALIDATOR'S LOGIC, not the choice of default model — those are separate concerns (default-model choice has its own AC-4 validation via AC-6 `./smackerel.sh test integration` + AC-7 healthy stack).

**Why fixtures populate ALL Config struct fields:** The validator's per-bucket loop walks every model field. If a fixture leaves some fields empty (`""`), the validator's skip-empty branch hides the field. To test the ENTIRE per-bucket coverage from DD-3, every ollama-routed field MUST have a value in AC-5(a)/(b) fixtures. The fixture builds a complete Config — not a sparse one.

---

## Affected Files (preview — final list belongs to `bubbles.plan` in scopes.md)

| File | Change kind | Owner agent |
|------|-------------|-------------|
| `internal/config/config.go` | Widen `Config` struct (add `OllamaMemoryLimitMiB int`, add ~15 new ollama-routed model fields if not already present, add `PhotosIntelligenceEmbedModel` if not present); add `OllamaMemoryLimit` parse step (mirror lines 694-700); rename and rewrite `validateMLModelEnvelope` → `validateModelEnvelopes` per DD-1 sketch; extend `requiredVars()` and `Load()` for the new env vars | `bubbles.implement` |
| `internal/config/validate_ml_envelope_test.go` (NEW or extend existing) | Add 3 tests for AC-5(a)/(b)/(c) per DD-6 fixtures | `bubbles.implement` |
| `cmd/config-validate/main.go` (NEW) | Tiny binary per DD-2 sketch — Load() + Validate() + exit | `bubbles.implement` |
| `cmd/config-validate/main_test.go` (NEW) | Unit tests for env-file loading + exit-code behavior | `bubbles.implement` |
| `scripts/commands/config.sh` | Add pre-emit invocation of `cmd/config-validate` per DD-2 sketch; TEMP write + atomic promote on success or abort on failure | `bubbles.implement` |
| `config/smackerel.yaml` | Change 9 ollama-routed default model fields per DD-5; add `gemma3:4b` and `deepseek-r1:7b` to `services.ml.model_memory_profiles`; add comment block near `llm.model` and `deploy_resources.ollama` per DD-5 | `bubbles.implement` |
| `docs/Operations.md` | Add "Model Envelope Sizing" section per AC-9 covering (a) per-service envelope contract, (b) dev/home-lab/production trade-off matrix, (c) fix-path order (raise envelope FIRST, then change model), (d) cross-reference to `services.ml.model_memory_profiles` catalog | `bubbles.implement` |
| `specs/052-bundle-secret-injection-contract/state.json` | Mark `C-A12`, `C-B5`, `C-B6` resolved per DD-4 (lightweight metadata-only update, no re-certification) | `bubbles.implement` |
| `specs/052-bundle-secret-injection-contract/scopes.md` | Flip Scope-4 DoD checkboxes for `C-A12`/`C-B5`/`C-B6` to `[x]` with inline evidence per DD-4 | `bubbles.implement` |
| `specs/052-bundle-secret-injection-contract/report.md` | Append Scope-4 resolution evidence section naming this packet's HEAD SHA + AC-6/AC-7 PASS outputs per DD-4 | `bubbles.implement` |

**Files explicitly NOT touched** (per `spec.md` § Out of Scope and per the user's design-phase constraints):
- `specs/045-deploy-resource-filesystem-hardening/spec.md` (parent spec — FR-045-002 text unchanged)
- `specs/052-bundle-secret-injection-contract/spec.md` / `design.md` / `scopes.md` (main scope text — only close-out artifacts touched)
- `deploy/compose.deploy.yml` (no new env vars introduced — `OLLAMA_MEMORY_LIMIT` already substituted)
- Any deploy adapter or overlay code (deploy adapter contract unchanged)
- Any source file outside this bug packet, during THIS design pass (design is INPUT — implement is the next agent's job)

---

## Alternative Approaches Considered

### Alternative 1 — YAML-side check in pure bash with no Go binary

**Approach:** Add the self-consistency check to `scripts/commands/config.sh` as a pure-bash loop reading model fields and profile entries from the flattened YAML, parsing compose memory strings in awk, comparing per-bucket.

**Rejected because:**
- Bash arithmetic on strings like `8G` / `2.5G` / `512M` requires re-implementing `parseComposeMemoryToMiB` in awk. The existing Go implementation is the canonical parser; duplication is fragile.
- Two-language maintenance: every future contract evolution (new envelope, new bucket, new memory unit) lands in both places or drifts out of sync.
- Loses the runtime-validator-reuse property of DD-2. The runtime validator is the contract owner; the shell check should call IT, not re-implement it.

### Alternative 2 — Move the contract to per-model YAML (model knows its own envelope key)

**Approach:** Replace the list-of-`{model, memory_mib}` profile entries with `{model, memory_mib, service}` where `service` names the deploy envelope. The validator just looks up each model's declared service.

**Rejected because:**
- Moves operator-affecting metadata into the profile catalog instead of the model-reference site. An operator changing `llm.ollama_model` to a new model would need to add a 3-field profile entry naming the service — extra surface area.
- The service is structurally determined by the YAML key path (e.g. `llm.*` and `agent.provider_routing.*.model` and `photos.intelligence.*_model` are always ollama; only `*embedding_model` is ml-sidecar). Encoding it twice (in the key path AND in the profile) is redundant and risks drift.
- Doesn't help with the runtime-env-var validator — env vars don't carry service tags. The Go side STILL needs to bucket by env var name, so the YAML-side service tag is dead weight.
- The accepted design (DD-1 bucketing by env-var-name → service in Go; YAML profile stays simple `{model, memory_mib}`) is the minimum-change-with-maximum-clarity option.

### Alternative 3 — Single-model "best fit per workload" instead of per-route choice

**Approach:** Pick ONE model (e.g. `qwen2.5:7b-instruct`) and use it for every route. No per-route differentiation; vision/OCR/reasoning routes all use the same model.

**Rejected because:**
- `qwen2.5:7b-instruct` is text-only — vision routes need a multimodal model. Forcing all routes onto a text-only model breaks the photos.intelligence.aesthetic / sensitivity / classify use cases.
- `deepseek-ocr:3b` is OCR-specialized; replacing it with a general-purpose 7B model degrades OCR quality measurably. The OCR route benefits from the specialized model.
- The reasoning route (chain-of-thought heavy workloads) benefits from the `deepseek-r1` family's CoT training. Replacing it with a general 7B model degrades reasoning quality.
- The accepted design (DD-5: `gemma3:4b` for default/vision/fast; `deepseek-r1:7b` for reasoning; `deepseek-ocr:3b` for OCR; `nomic-embed-text` for embedding) preserves per-route specialization at small budget cost.

---

## Open Questions

None. All four spec-level open questions Q-1..Q-4 are resolved as decisions DD-1..DD-4 above. Two implementation-detail questions for `bubbles.plan` to record (NOT blockers, NOT design ambiguity):

- **IQ-1 (for `bubbles.plan`):** Exact location of `cmd/config-validate/main.go` — peer to `cmd/scenario-lint`. The directory and binary name are fixed by DD-2; nothing for design to decide.
- **IQ-2 (for `bubbles.plan`):** Exact insertion site in `scripts/commands/config.sh` for the pre-emit invocation — between the env block computation (currently lines 437-1200+) and the final emission to `config/generated/<env>.env`. `bubbles.plan` reads the live `config.sh` at implement time and picks the line.

Neither blocks implementation. Both are stylistic / placement concerns for the plan/implement specialists to record in `scopes.md`.

---

## Risks (residual after design)

- **R-1 (low) — Live `gemma3:4b` resident size > 4096 MiB ceiling.** If `bubbles.implement` measures via `ollama ps` and finds resident size > 4506 MiB (10% drift threshold), the profile entry MUST be updated. The risk is small (model card numbers are conservative) but real. Mitigation: DD-5 explicitly requires live verification before commit.
- **R-2 (low) — Two concurrent ollama models exceed 8 GiB envelope in burst.** Ollama's eviction policy is single-model-at-a-time under default `OLLAMA_KEEP_ALIVE`; the validator's per-model check matches that policy. If a future change ENABLES concurrent loading (`OLLAMA_NUM_PARALLEL > 1` plus keep-alive on multiple models), the per-model envelope check would no longer be sufficient. Mitigation: cross-reference in `docs/Operations.md` "Model Envelope Sizing" section names this assumption; future spec touching `OLLAMA_NUM_PARALLEL` must revisit.
- **R-3 (low) — `cmd/config-validate` adds a `go run` invocation to every `./smackerel.sh config generate`.** Cold-start `go run` is ~1-3 seconds on the dev box and adds latency to the operator's config-generate workflow. Acceptable trade-off for failing earlier. Mitigation: `bubbles.implement` may add `make build-tools` to pre-build the binary; not required for correctness.
- **R-4 (low) — Spec 052 close-out edits cross the bug-packet boundary.** This packet's `bubbles.implement` writes into `specs/052-bundle-secret-injection-contract/` artifacts (state.json + scopes.md + report.md). Per DD-4 this is a lightweight metadata-only update and the agent ownership model preserves the existing certification. Mitigation: the bug packet's `spec.md` § Out of Scope explicitly lists what is NOT touched in spec 052 (main spec/design/scopes scope text); only the close-out artifacts are in scope.

No high or medium risks. All four risks have documented mitigations.

---

## References

- `spec.md` (this folder) — bug specification with 4 root cause categories a/b/c/d, AC-1..AC-10, severity justification, and 4 open questions Q-1..Q-4 (all resolved here).
- `state.json` (this folder) — control-plane state and transition request `TR-BUG-045-001-001`.
- `report.md` (this folder) — raw discovery evidence (read_file / grep_search outputs).
- `scenario-manifest.json` (this folder) — 4 scenario shells SCN-045-001-A/B/C/D mapped to AC-5(a)/(b)/(c) and AC-6/AC-7.
- [internal/config/config.go](../../../../internal/config/config.go) — validator at lines 1745-1801; `MLMemoryLimit` parse pattern at 694-700; `requiredVars()` at 1350+; `Config` struct at 260+.
- [scripts/commands/config.sh](../../../../scripts/commands/config.sh) — emission of `OLLAMA_MEMORY_LIMIT` at line 446; `ML_MODEL_MEMORY_PROFILES_JSON` at line 454; `AGENT_PROVIDER_*_MODEL` at 1043-1056; `PHOTOS_INTELLIGENCE_*_MODEL` at 712-716.
- [config/smackerel.yaml](../../../../config/smackerel.yaml) — exhaustive model-field sweep table above; profile catalog at lines 766-776; deploy envelopes at 788-810.
- [docs/Operations.md](../../../../docs/Operations.md) — target for the new "Model Envelope Sizing" section per AC-9.
- [specs/052-bundle-secret-injection-contract/](../../../052-bundle-secret-injection-contract/) — close-out target for concerns `C-A12`, `C-B5`, `C-B6` per DD-4.
- [.github/instructions/smackerel-no-defaults.instructions.md](../../../../.github/instructions/smackerel-no-defaults.instructions.md) — Gate G028 fail-loud SST policy. The new `OllamaMemoryLimit` parse and the new `cmd/config-validate` binary MUST fail loud on any malformed input (mirror existing `ML_MEMORY_LIMIT: %w` pattern exactly).
- [.github/skills/smackerel-no-defaults/SKILL.md](../../../../.github/skills/smackerel-no-defaults/SKILL.md) — `bubbles.implement` MUST load this skill before touching any of the affected files.
- [.github/skills/bubbles-config-sst/SKILL.md](../../../../.github/skills/bubbles-config-sst/SKILL.md) — `bubbles.implement` MUST load this skill before adding the new ollama-routed Config struct fields. Note: the new `OllamaMemoryLimit` parsed-MiB value derives entirely from the existing `OLLAMA_MEMORY_LIMIT` env value, so no new env emission in `config.sh` is required for the parse field; the 6-layer SST contract still applies to any new env emissions if `bubbles.implement` discovers an ollama-routed env var that is NOT yet emitted at implement-time HEAD.
- Ollama library model cards (cited in DD-5 for resident size CEILINGS): `https://ollama.com/library/gemma3`, `https://ollama.com/library/deepseek-r1`. `bubbles.implement` verifies live `ollama ps` resident size before commit and adjusts profile entries if > 10% drift.
