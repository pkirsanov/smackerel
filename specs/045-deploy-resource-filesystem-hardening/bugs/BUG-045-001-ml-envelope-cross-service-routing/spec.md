# Bug: BUG-045-001 — `validateMLModelEnvelope()` checks ALL model env vars against a SINGLE ML-sidecar envelope, conflating ollama-routed models (LLM_MODEL, OLLAMA_MODEL — run in the `ollama` container, 8G envelope) with ML-sidecar-routed models (EMBEDDING_MODEL — runs in the `smackerel_ml` container, 3G envelope)

## Classification

- **Type:** Configuration / SST contract defect — cross-service envelope routing bug in spec 045 FR-045-002 validator. The validator's intent (fail-loud at startup when a configured model exceeds its deploy envelope) is correct, but its implementation conflates two physically distinct deploy services (`ollama` and `smackerel_ml`) into a single envelope check, so its error messages name the wrong envelope and its rejection set is both incomplete (does not catch ollama-only overruns when `ML_MEMORY_LIMIT` is high enough) and incorrect (rejects ollama-routed models against the ml-sidecar envelope).
- **Severity:** P1 — HIGH. Three concrete observable impacts on `main` at HEAD `de49b2f9`:
  1. **Chronic CI failure** — `https://github.com/pkirsanov/smackerel/actions/workflows/ci.yml` has 10 consecutive `CI` runs in FAILURE (runs 170-179; latest run id `25931687144`); `lint-and-test` and `build` jobs pass but `integration` fails because `./smackerel.sh test integration` hits `smackerel-core` startup health-check failure consistent with this validator rejection (env file emitted with default `LLM_MODEL=gemma4:26b` + `ML_MEMORY_LIMIT=3G`; validator computes `18432 MiB > 3072 MiB` and Validate() returns the spec 045 FR-045-002 error before HTTP listener binds).
  2. **Spec 052 live canary blocked** — documented at `specs/052-bundle-secret-injection-contract/state.json` (concerns `C-A12`, `C-B5`, `C-B6` — all severity:low, all followUpOwner:operator) with verbatim error message:
     `ML model envelope validation failed (spec 045 FR-045-002): ML model envelope exceeded: LLM_MODEL="gemma4:26b" requires 18432 MiB but ML_MEMORY_LIMIT="3G" resolves to 3072 MiB`.
     Spec 052 closed `done_with_concerns` because the operator cannot perform the L2/L3 live-stack canary on the dev box (gemma4:26b needs 18.4 GiB; dev box has 11.7 GiB) and the deploy-adapter (knb / home-lab) path is gated by the same envelope mismatch.
  3. **Self-inconsistent default config** — `config/smackerel.yaml` ships with `llm.model = gemma4:26b` (profile 18432 MiB per `services.ml.model_memory_profiles`) and `deploy_resources.ollama.memory = "8G"` (8192 MiB). Even AFTER fixing the validator's cross-service routing, the DEFAULT config remains internally inconsistent: the default model needs 18 GiB but the default ollama envelope is 8 GiB. The validator MUST catch this at `./smackerel.sh config generate` time too — currently it does not because the cross-service check does not exist.
- **Parent Spec:** 045 — Deploy Resource and Filesystem Hardening (`specs/045-deploy-resource-filesystem-hardening/`) — owns `FR-045-002` (ML model memory profile envelope contract). This bug packet is a **scoped repair** to the FR-045-002 implementation; the FR itself stays unchanged in the parent spec.
- **Workflow Mode:** `bugfix-fastlane` (parent-expanded by `bubbles.goal` because the nested workflow runtime lacks `runSubagent`)
- **Status:** Fixed (post-Scope-3 working tree; owned validator chain green; 4 foreign blockers routed as RQ-QF-001 + RQ-REPORT-MD-CLEANUP-001 + RQ-BUBBLES-AGNOSTICITY-001 + RQ-BUBBLES-ARTIFACT-LINT-INFO-001 — carved out, not regressions of the fix; the 4 root cause categories are all addressed by Scopes 1+2+3 — see `scopes.md` §1, §2, §3 evidence)
- **Discovered By:** 2026-05-16 `bubbles.goal` autonomous-execution session following a `bubbles.system-review` "check deployment readiness" run (this session)
- **Discovery Date:** 2026-05-16

## Summary

The function `validateMLModelEnvelope()` at `internal/config/config.go` lines 1745-1801 currently checks THREE model env vars (`LLM_MODEL`, `OLLAMA_MODEL`, `EMBEDDING_MODEL`) all against a SINGLE service envelope (`ML_MEMORY_LIMIT` → `c.MLMemoryLimitMiB` → `deploy_resources.smackerel_ml.memory` → 3 GiB on default config).

But these models physically live in TWO DIFFERENT deploy services per `config/smackerel.yaml`:

| Env var | Default value | Runtime container | Correct envelope key | Default envelope |
|---------|---------------|--------------------|----------------------|------------------|
| `LLM_MODEL` | `gemma4:26b` | `ollama` | `deploy_resources.ollama.memory` (`OLLAMA_MEMORY_LIMIT`) | `8G` (8192 MiB) |
| `OLLAMA_MODEL` | `gemma4:26b` | `ollama` | `deploy_resources.ollama.memory` (`OLLAMA_MEMORY_LIMIT`) | `8G` (8192 MiB) |
| `EMBEDDING_MODEL` | `nomic-embed-text` | `smackerel_ml` (Python sidecar; sentence-transformers / fastembed in-process) | `deploy_resources.smackerel_ml.memory` (`ML_MEMORY_LIMIT`) | `3G` (3072 MiB) |

This category error causes `Validate()` to fail loudly at startup with:

```text
ML model envelope validation failed (spec 045 FR-045-002): ML model envelope exceeded: LLM_MODEL="gemma4:26b" requires 18432 MiB but ML_MEMORY_LIMIT="3G" resolves to 3072 MiB
```

…even though `gemma4:26b` is supposed to run in `ollama` (envelope 8G), NOT the ml sidecar. The validator's REJECTION is correct in spirit (gemma4:26b at 18432 MiB exceeds 8G too) but it names the WRONG envelope, so the operator's natural fix path (raise `ML_MEMORY_LIMIT` to 18G+) does NOT help — `gemma4:26b` runs in `ollama`, so `OLLAMA_MEMORY_LIMIT` is the value that matters.

## Discovery Brief

The 2026-05-16 `bubbles.system-review` "check deployment readiness" run flagged the chronic CI integration failures on `main` (10 consecutive FAILURE runs as of HEAD `de49b2f9`). A follow-up `bubbles.goal` autonomous-execution session attempted to reproduce locally via `./smackerel.sh test integration` and observed the smackerel-core container go unhealthy during health-check polling. Inspecting the container logs surfaced the spec 045 FR-045-002 validator rejection above.

The verbatim error message was already captured in `specs/052-bundle-secret-injection-contract/state.json` at concerns `C-A12`, `C-B5`, `C-B6` (filed 2026-05-15 during spec 052 close-out, all severity:low, all followUpOwner:operator, all referencing "pre-existing spec 045 ML envelope mismatch" as the unblocking dependency). The spec 052 closure note explicitly named the dev box's 11.7 GiB available memory as the binding constraint and routed the unblock action to "ml-config maintainer / spec 045 owner".

This bug packet (`BUG-045-001`) is the formal discovery and root-cause analysis of that referenced "spec 045 ML envelope mismatch", scoped to the validator's cross-service routing defect (root cause a + b + c + d below).

## Problem Statement

The spec 045 FR-045-002 contract (`specs/045-deploy-resource-filesystem-hardening/spec.md` line 32) reads:

> **FR-045-002:** ML-sidecar model configuration MUST validate the configured Ollama model against a documented memory profile in `services.ml.model_memory_profiles`. The Go core `Validate()` chain MUST fail-loud at startup when the model's required memory exceeds the configured ML deploy memory envelope.

The FR's wording is unambiguous about the ML sidecar — but the validator implementation (`internal/config/config.go` `validateMLModelEnvelope()`, lines 1745-1801) treats the FR as if ALL Ollama models also run in the ML sidecar. They do not.

### Actual deployment topology (per `deploy/compose.deploy.yml` and `docker-compose.yml`)

| Service | Container | Memory envelope key | Models it actually loads |
|---------|-----------|---------------------|--------------------------|
| `ollama` | `ollama/ollama` | `deploy_resources.ollama.memory` → `OLLAMA_MEMORY_LIMIT` | All `*.gguf` LLMs / vision / OCR / reasoning / fast models served via the Ollama HTTP API at `http://ollama:11434` (called by Go core AND by the ML sidecar) |
| `smackerel_ml` | Python FastAPI | `deploy_resources.smackerel_ml.memory` → `ML_MEMORY_LIMIT` | In-process embedding model (`EMBEDDING_MODEL` = `nomic-embed-text`) loaded via sentence-transformers / fastembed; no Ollama models loaded in-process |
| `smackerel_core` | Go | `deploy_resources.smackerel_core.memory` → `CORE_MEMORY_LIMIT` | No models loaded in-process; calls ollama HTTP API and ML sidecar HTTP API |

### Pre-fix validator code (HEAD `de49b2f9`)

`internal/config/config.go` lines 1763-1768 (verbatim, captured via `read_file`):

```go
	used := []modelRef{
		{"LLM_MODEL", c.LLMModel},
		{"OLLAMA_MODEL", c.OllamaModel},
		{"EMBEDDING_MODEL", c.EmbeddingModel},
	}
```

…followed at line 1786 by:

```go
		if profileMiB > c.MLMemoryLimitMiB {
			oversized = append(oversized, fmt.Sprintf("%s=%q requires %d MiB but ML_MEMORY_LIMIT=%q resolves to %d MiB", ref.envVar, ref.model, profileMiB, c.MLMemoryLimit, c.MLMemoryLimitMiB))
		}
```

ALL three model env vars are bucketed against `c.MLMemoryLimitMiB` — the ML sidecar envelope. The error message hard-codes `ML_MEMORY_LIMIT` in the format string regardless of which envelope SHOULD have been named.

### Pre-fix default config (HEAD `de49b2f9`)

`config/smackerel.yaml` lines 53/56/57/563/572/683/685/686 (verbatim):

```yaml
llm:
  model: gemma4:26b
  ollama_model: gemma4:26b
  ollama_vision_model: gemma4:26b
  ...
photos:
  intelligence:
    classify_model: gemma4:26b
    sensitivity_model: gemma4:26b
    aesthetic_model: gemma4:26b
```

…and lines 765-766 (model profile):

```yaml
model_memory_profiles:
- model: "gemma4:26b"
  memory_mib: 18432 # 18 GiB MoE 26B at q4 + 256K context buffer
```

…and lines 788/801-802 (deploy envelopes):

```yaml
deploy_resources:
  ...
  smackerel_ml:
    cpus: "2.0"
    memory: "3G"
  ollama:
    cpus: "4.0"
    memory: "8G"
```

The default config is **internally inconsistent**: `gemma4:26b` needs 18 GiB; the default ollama envelope is 8 GiB. Even if the validator's cross-service routing were correct, the default config would still fail at runtime (just with a correctly-named `OLLAMA_MEMORY_LIMIT="8G"` error instead of the current wrongly-named `ML_MEMORY_LIMIT="3G"` error). Both classes of failure must be fixed for `./smackerel.sh up` to succeed out-of-the-box on default config.

### Pre-fix SST surface (HEAD `de49b2f9`)

The good news (refinement to the original discovery brief): `OLLAMA_MEMORY_LIMIT` is **already** emitted by `scripts/commands/config.sh:446` and **already** in `internal/config/config.go` `requiredVars()` (line 1378) and **already** loaded into `Config.OllamaMemoryLimit` (line 560). What is MISSING is:

| Missing piece | File:line at HEAD `de49b2f9` | What needs to change |
|---------------|------------------------------|----------------------|
| `OllamaMemoryLimitMiB` parsed value | `internal/config/config.go` (no occurrence) | Add parse step mirroring `MLMemoryLimit` → `MLMemoryLimitMiB` at lines 694-700 |
| Per-service routing in validator | `internal/config/config.go:1763-1768` | Split `used []modelRef` into ollama-routed vs ml-sidecar-routed buckets; check each bucket against the matching envelope |
| Validator awareness of `OllamaVisionModel`, `OllamaOcrModel`, `OllamaReasoningModel`, `OllamaFastModel`, and `photos.intelligence.*_model` fields | `internal/config/config.go:1763-1768` (only 3 of ~9 model fields are checked) | Extend `used []modelRef` to cover every model field that is actually loaded into ollama at runtime |
| Self-consistency check at config-generate time | (no occurrence) | New check (in `Validate()` or in `scripts/commands/config.sh`) that compares each `llm.*` / `photos.intelligence.*_model` default in `config/smackerel.yaml` against `deploy_resources.ollama.memory` BEFORE the env file is emitted, so the operator sees the misconfiguration at `./smackerel.sh config generate` time, not at runtime startup |
| Documentation of the per-service envelope contract | `docs/Operations.md` (no "Model Envelope Sizing" section) | New section explaining the dev/home-lab/production model-selection trade-off |

## Detection

| Aspect | Detail |
|--------|--------|
| Trigger | 2026-05-16 `bubbles.system-review` "check deployment readiness" run on HEAD `de49b2f9` |
| Finding | Chronic CI integration failure (10 consecutive `CI` workflow runs in FAILURE on `main`) PLUS spec 052 close-out concerns `C-A12` / `C-B5` / `C-B6` referencing "pre-existing spec 045 ML envelope mismatch" as unblock dependency |
| Severity | P1 — HIGH (blocks main-branch CI, blocks spec 052 home-lab canary, will block any new operator running `./smackerel.sh up` on default config) |
| Audit method | (a) `read_file internal/config/config.go startLine=1730 endLine=1810` confirmed the verbatim `used []modelRef{...}` block with all 3 model env vars bucketed against `c.MLMemoryLimitMiB`. (b) `read_file config/smackerel.yaml startLine=740 endLine=820` confirmed `gemma4:26b` profile is 18432 MiB and ollama envelope is 8G + smackerel_ml envelope is 3G. (c) `grep_search OllamaMemoryLimit` confirmed `OLLAMA_MEMORY_LIMIT` is in `requiredVars()` (line 1378) but `OllamaMemoryLimitMiB` does NOT exist. (d) `grep_search` confirmed no per-service routing exists anywhere in `validateMLModelEnvelope`. (e) Cross-reference to `specs/052-bundle-secret-injection-contract/state.json` lines 349/369/377 confirmed the spec 052 close-out already captured the verbatim error message and routed the unblock to "ml-config maintainer / spec 045 owner". |
| CI evidence URL | `https://github.com/pkirsanov/smackerel/actions/workflows/ci.yml` (runs 170-179, latest `https://github.com/pkirsanov/smackerel/actions/runs/25931687144`) — workflow runs visible to the public; per-job logs require admin auth. The local reproduction at step (a) below is the authoritative evidence for this packet. |

## Reproduction

```bash
# (1) Confirm the validator's cross-service-conflating model bucket at HEAD
grep -n -A 5 'used := \[\]modelRef' internal/config/config.go
# Expected: 1 match at line 1762, showing all 3 model env vars in one bucket
# with no per-service routing.

# (2) Confirm gemma4:26b's profile (18432 MiB) exceeds BOTH envelopes
grep -n -A 1 'gemma4:26b' config/smackerel.yaml | head -10
# Expected: model_memory_profiles[gemma4:26b].memory_mib = 18432
# Default llm.model = gemma4:26b
# Default deploy_resources.ollama.memory = 8G (8192 MiB)
# Default deploy_resources.smackerel_ml.memory = 3G (3072 MiB)
# 18432 > 8192 > 3072 — so the default config is unsatisfiable on EITHER envelope.

# (3) Reproduce the wrongly-named runtime error
./smackerel.sh config generate --env dev
./smackerel.sh up
./smackerel.sh status
# Expected: smackerel-core container goes unhealthy during health-check polling.
# docker logs smackerel-dev-core shows:
#   "ML model envelope validation failed (spec 045 FR-045-002): ML model
#    envelope exceeded: LLM_MODEL=\"gemma4:26b\" requires 18432 MiB but
#    ML_MEMORY_LIMIT=\"3G\" resolves to 3072 MiB"
# NOTE: The named envelope is WRONG. gemma4:26b runs in ollama, not the
# ML sidecar. The correct error would name OLLAMA_MEMORY_LIMIT.

# (4) Reproduce the chronic CI failure (via local integration suite)
./smackerel.sh test integration
# Expected: smackerel-core startup fails before integration tests can run.
# Same root cause as step (3).

# (5) Confirm OLLAMA_MEMORY_LIMIT IS already emitted as an SST variable
grep -n 'OLLAMA_MEMORY_LIMIT' scripts/commands/config.sh internal/config/config.go
# Expected:
#   scripts/commands/config.sh:446: OLLAMA_MEMORY_LIMIT="$(required_value deploy_resources.ollama.memory)"
#   internal/config/config.go:280: OllamaMemoryLimit     string
#   internal/config/config.go:560: OllamaMemoryLimit:   os.Getenv("OLLAMA_MEMORY_LIMIT"),
#   internal/config/config.go:1378: {"OLLAMA_MEMORY_LIMIT", c.OllamaMemoryLimit},
# What is MISSING: any reference to OllamaMemoryLimitMiB (parsed value) and
# any reference to OllamaMemoryLimit inside validateMLModelEnvelope.
```

Expected: the validator at `internal/config/config.go:1745-1801` buckets all 3 model env vars against `c.MLMemoryLimitMiB`; default `config/smackerel.yaml` ships an internally-inconsistent (model > envelope) combination; smackerel-core fails to start on default config with a wrongly-named-envelope error; CI integration job fails consistently; `OLLAMA_MEMORY_LIMIT` is already emitted as an SST variable but is not parsed-to-MiB and is not consumed by the validator.

## Expected Behavior

After the fix, a reader inspecting the system should observe:

1. **Validator splits model checks by deploy service.** `validateMLModelEnvelope()` (or its successor) maintains TWO buckets: ollama-routed models (checked against `OllamaMemoryLimitMiB`) and ML-sidecar-routed models (checked against `MLMemoryLimitMiB`). Error messages name the CORRECT envelope key (`OLLAMA_MEMORY_LIMIT="8G"` for ollama-routed offenders; `ML_MEMORY_LIMIT="3G"` for ml-sidecar-routed offenders).
2. **Validator coverage is complete.** Every config field that names an ollama model is in the ollama-routed bucket: `LLMModel`, `OllamaModel`, `OllamaVisionModel`, `OllamaOcrModel`, `OllamaReasoningModel`, `OllamaFastModel`, and `photos.intelligence.classify_model` / `sensitivity_model` / `aesthetic_model`. Every config field that names an in-process ML-sidecar model is in the ML-sidecar-routed bucket: `EmbeddingModel`. (Future model fields are added to whichever bucket matches their runtime container.)
3. **`OllamaMemoryLimit` is parsed to `OllamaMemoryLimitMiB` and used in the validator.** A new parse step (mirroring `MLMemoryLimit` → `MLMemoryLimitMiB` at `internal/config/config.go` lines 694-700) converts the compose-style string to MiB. The validator references `c.OllamaMemoryLimitMiB` for the ollama bucket.
4. **Self-consistency check at config-generate time.** A new check (either in `Validate()` running over `config/smackerel.yaml` defaults or in `scripts/commands/config.sh`) rejects any `config/smackerel.yaml` whose default model fields reference a model whose memory profile exceeds the service envelope it would run in. Fails BEFORE the env file is written so the operator sees the misconfiguration at `./smackerel.sh config generate` time, not at runtime startup. Error message names the offending `llm.<field>` and the offending `deploy_resources.<service>.memory`.
5. **Default `config/smackerel.yaml` is internally consistent.** Every model named by a default `llm.*` / `photos.intelligence.*_model` field has a profile that fits its service envelope (ollama envelope for ollama-routed fields, ml-sidecar envelope for ml-sidecar-routed fields). For the default `deploy_resources.ollama.memory = "8G"`, the chosen default model must have profile ≤ 8192 MiB. A comment block in `config/smackerel.yaml` documents the trade-off: dev/8G fits the chosen default; home-lab operators with more RAM may opt UP to `gemma4:26b` by raising the ollama envelope first.
6. **`./smackerel.sh test integration` PASSES locally on default config**, proving the chronic CI failure is resolved at root.
7. **`./smackerel.sh up` + `./smackerel.sh status` shows the stack healthy on default config**, proving the spec 052 live-canary path is unblocked.
8. **Spec 052 close-out artifacts are updated.** `specs/052-bundle-secret-injection-contract/state.json` concerns `C-A12`, `C-B5`, `C-B6` are marked resolved with concrete evidence references (substituted-bundle dry-run + healthy stack canary). `specs/052-bundle-secret-injection-contract/scopes.md` DoD checkboxes for those concerns are flipped to `[x]`. `specs/052-bundle-secret-injection-contract/report.md` Scope 4 sections add the resolution evidence.
9. **`docs/Operations.md` has a new "Model Envelope Sizing" section** documenting per-service envelope contracts and the dev / home-lab / production model-selection trade-off.

## Actual Behavior (Pre-Fix at HEAD `de49b2f9`)

A reader inspecting the system at HEAD `de49b2f9` finds:

1. `validateMLModelEnvelope()` at `internal/config/config.go:1745-1801` buckets all 3 model env vars (`LLM_MODEL`, `OLLAMA_MODEL`, `EMBEDDING_MODEL`) against the SINGLE envelope `c.MLMemoryLimitMiB`. No per-service routing exists.
2. Only 3 of ~9 model config fields are checked. `OllamaVisionModel`, `OllamaOcrModel`, `OllamaReasoningModel`, `OllamaFastModel`, and the `photos.intelligence.*_model` fields are NOT checked at all (even on the wrong envelope), so model-profile overruns there go undetected until runtime when ollama itself fails to load the model.
3. `OllamaMemoryLimit` (string) IS loaded from env (`internal/config/config.go:560`) and IS in `requiredVars()` (line 1378), but NO `OllamaMemoryLimitMiB` parsed value exists anywhere in the codebase. `validateMLModelEnvelope()` does NOT reference `OllamaMemoryLimit` at all.
4. Default `config/smackerel.yaml` ships with `llm.model = gemma4:26b` (18432 MiB profile) and `deploy_resources.ollama.memory = "8G"` (8192 MiB). The default config is internally unsatisfiable on the ollama envelope BEFORE the spec 045 validator even runs — but no config-generate-time check catches it.
5. `./smackerel.sh up` on default config produces a smackerel-core container that fails the `Validate()` chain at startup with the wrongly-named-envelope error:
   `ML model envelope validation failed (spec 045 FR-045-002): ML model envelope exceeded: LLM_MODEL="gemma4:26b" requires 18432 MiB but ML_MEMORY_LIMIT="3G" resolves to 3072 MiB`
6. `./smackerel.sh test integration` fails the same way (smackerel-core never becomes healthy; integration tests never run); CI `integration` job has failed on 10 consecutive runs as a direct consequence.
7. Spec 052 close-out (`specs/052-bundle-secret-injection-contract/state.json` concerns `C-A12` / `C-B5` / `C-B6`) records the unblock dependency as "pre-existing spec 045 ML envelope mismatch unrelated to spec 052" with routing to "ml-config maintainer / spec 045 owner".
8. `docs/Operations.md` has NO section documenting the per-service envelope contract or the dev/home-lab/production model-selection trade-off.

## Root Cause Analysis

The defect has FOUR distinct root-cause categories (a/b/c/d), each requiring its own fix work:

### Root Cause (a) — Code: validator lacks per-service envelope routing

`validateMLModelEnvelope()` at `internal/config/config.go:1745-1801` was authored as a single-bucket check against one envelope. The author treated `EMBEDDING_MODEL` (ml-sidecar) and `LLM_MODEL` / `OLLAMA_MODEL` (ollama-served via HTTP) as if they all loaded into the same container, which is not the case. The Go core calls ollama via HTTP at `http://ollama:11434` (see `llm.ollama_url` in `config/smackerel.yaml` line 55); the actual GGUF weights live in the `ollama` container's memory, not the smackerel-core or smackerel-ml container. `EMBEDDING_MODEL` is different — it loads in-process inside the Python ML sidecar via sentence-transformers / fastembed.

The fix is to split `used []modelRef` into two buckets by deploy service, parse `OllamaMemoryLimit` to `OllamaMemoryLimitMiB` (mirroring the `MLMemoryLimit` parse pattern at lines 694-700), and check each bucket against its matching envelope. Error messages must reference the named envelope key for the bucket the offender is in.

### Root Cause (b) — SST surface: `OLLAMA_MEMORY_LIMIT` is not parsed-to-MiB

`scripts/commands/config.sh:446` already emits `OLLAMA_MEMORY_LIMIT="$(required_value deploy_resources.ollama.memory)"` and `internal/config/config.go` already loads it into `Config.OllamaMemoryLimit` (line 560) and validates it via `requiredVars()` (line 1378). What is missing is the parse step that converts the compose-style string (e.g. `"8G"`) to MiB so the validator can compare against integer model profiles. The parse helper `parseComposeMemoryToMiB` already exists (used at line 695 for `MLMemoryLimit`) and just needs to be called for `OllamaMemoryLimit` too. This is a small SST-surface widening, NOT a NO-DEFAULTS contract change — the value is already required-loud, only the parsed integer field is missing.

### Root Cause (c) — Config consistency: no config-generate-time self-consistency check

`config/smackerel.yaml` defaults (`llm.model = gemma4:26b` with profile 18432 MiB vs `deploy_resources.ollama.memory = "8G"`) are mutually unsatisfiable, but no validator catches this at `./smackerel.sh config generate` time. The defect is only surfaced at runtime startup when the Go core's `Validate()` chain runs against the EMITTED env file. A new check should compare each `llm.*` / `photos.intelligence.*_model` default in `config/smackerel.yaml` against the appropriate `deploy_resources.<service>.memory` BEFORE the env file is emitted. This gives the operator earlier failure (and a clearer fix path) than waiting for runtime startup. Implementation can live in either `internal/config/config.go` (Validate() over the in-memory parsed config) or in `scripts/commands/config.sh` (shell-side pre-emit check); the chosen location is for `bubbles.design` to decide.

### Root Cause (d) — Documentation: no per-service envelope sizing guide

`docs/Operations.md` does not document the per-service model envelope contract, so operators do not know how to right-size for their hardware. The dev/home-lab/production trade-off (dev box: ≤ 8G ollama envelope → smaller model; home-lab: 16-32G ollama envelope → larger model; production: tuned per workload) is implicit in the SST surface but not surfaced anywhere operator-facing. A new "Model Envelope Sizing" section in `docs/Operations.md` should explain the contract: (1) ollama envelope sizes the LLM / vision / OCR / reasoning / fast models; (2) ml-sidecar envelope sizes the in-process embedding model; (3) each model field in `config/smackerel.yaml` must be a model whose profile fits its target envelope; (4) raise the envelope FIRST before configuring a larger model.

### Why the bug went undetected until now

- The validator's single-bucket check happens to be correct for `EMBEDDING_MODEL` (nomic-embed-text profile 768 MiB ≤ 3G ml-sidecar envelope), so the EMBEDDING_MODEL path silently passes its check against the correct envelope by coincidence.
- The validator's single-bucket check is wrong for `LLM_MODEL` / `OLLAMA_MODEL`, but the WRONG envelope (3G ml-sidecar) is SMALLER than the CORRECT envelope (8G ollama), so the check fails LOUDER than it should — which masked the routing bug behind a real "model too big for any envelope" failure. The validator was effectively "right by accident" for any model that would have failed ollama's envelope too (which is the case for gemma4:26b at 18432 MiB).
- The fix surfaces the routing bug only because the operator's natural remediation (raise `ML_MEMORY_LIMIT`) does not work — gemma4:26b runs in ollama, not the ml sidecar, so changing `ML_MEMORY_LIMIT` has zero effect on whether gemma4:26b can load. The operator then wastes time debugging the wrong envelope.

## Acceptance Criteria

These ACs are the contract for the eventual fix (to be authored by `bubbles.design` + `bubbles.plan` + `bubbles.implement` in the next phases). The ACs collectively close all four root cause categories (a/b/c/d).

- **AC-1 (Root cause a — per-service routing):** `validateMLModelEnvelope()` (or its successor) routes each model env var to its CORRECT service envelope. Ollama-routed models (`LLM_MODEL`, `OLLAMA_MODEL`, `OLLAMA_VISION_MODEL`, `OLLAMA_OCR_MODEL`, `OLLAMA_REASONING_MODEL`, `OLLAMA_FAST_MODEL`, `PHOTOS_INTELLIGENCE_CLASSIFY_MODEL`, `PHOTOS_INTELLIGENCE_SENSITIVITY_MODEL`, `PHOTOS_INTELLIGENCE_AESTHETIC_MODEL`) are checked against `OllamaMemoryLimitMiB`. ML-sidecar-routed models (`EMBEDDING_MODEL`) are checked against `MLMemoryLimitMiB`. Error messages name the correct envelope (e.g. `OLLAMA_MEMORY_LIMIT="8G"` not `ML_MEMORY_LIMIT="3G"` when the offending model is ollama-routed).

- **AC-2 (Root cause b — parse OllamaMemoryLimit to MiB):** A new parse step (mirroring the `MLMemoryLimit` → `MLMemoryLimitMiB` parse at `internal/config/config.go:694-700`) populates `c.OllamaMemoryLimitMiB` from `c.OllamaMemoryLimit`. The new `Config.OllamaMemoryLimitMiB` integer field exists and is consumed by `validateMLModelEnvelope()` (or successor). Note: `OLLAMA_MEMORY_LIMIT` is already emitted by `scripts/commands/config.sh:446` and already in `requiredVars()` at `internal/config/config.go:1378` — no SST-surface widening beyond adding the parsed-MiB field is required.

- **AC-3 (Root cause c — config-generate-time self-consistency check):** A new self-consistency check at config-generate time (or in `Validate()` running over `config/smackerel.yaml` defaults BEFORE the env file is written) rejects any `config/smackerel.yaml` whose default model fields reference a model whose memory profile exceeds the service envelope it would run in. Fails LOUDLY at `./smackerel.sh config generate` time, not at runtime startup. The error message names the offending `llm.<field>` (e.g. `llm.ollama_model`) and the offending `deploy_resources.<service>.memory` (e.g. `deploy_resources.ollama.memory = "8G"`) plus the model's profile from `services.ml.model_memory_profiles`. The chosen implementation location (Go `Validate()` over the YAML in-memory OR shell-side pre-emit check in `config.sh`) is for `bubbles.design` to decide.

- **AC-4 (Root cause c — fix default config consistency):** `config/smackerel.yaml` default `llm.model`, `llm.ollama_model`, `llm.ollama_vision_model`, `llm.ollama_ocr_model`, `llm.ollama_reasoning_model`, `llm.ollama_fast_model`, `photos.intelligence.classify_model`, `photos.intelligence.sensitivity_model`, and `photos.intelligence.aesthetic_model` are CHANGED from `gemma4:26b` to a model that FITS the default `deploy_resources.ollama.memory = "8G"` envelope. Acceptable candidate must have profile ≤ 8192 MiB. The chosen model is added to `services.ml.model_memory_profiles` if not already present, with a cited resident-size measurement (e.g. `ollama ps` output reference). A comment block in `config/smackerel.yaml` documents the trade-off: dev/8G fits this model; home-lab operators with more RAM may opt UP to `gemma4:26b` by raising `deploy_resources.ollama.memory` FIRST.

- **AC-5 (Adversarial regression tests in `internal/config/validate_ml_envelope_test.go`):** Three test cases:
  - **(a)** An ollama-routed model whose profile FITS `OLLAMA_MEMORY_LIMIT` but EXCEEDS `ML_MEMORY_LIMIT` is ACCEPTED. (Example: configure `LLM_MODEL` to a model with profile 6000 MiB; `OLLAMA_MEMORY_LIMIT=8G`; `ML_MEMORY_LIMIT=3G`. 6000 < 8192 so ollama bucket passes. 6000 > 3072 so a single-bucket check would WRONGLY reject. This case proves the old conflation is gone.)
  - **(b)** The OLD bug pattern (gemma4:26b with `OLLAMA_MEMORY_LIMIT=10G`, `ML_MEMORY_LIMIT=3G`, gemma4:26b profile=18432 MiB) is REJECTED with an error that NAMES `OLLAMA_MEMORY_LIMIT` (not `ML_MEMORY_LIMIT`). (10G < 18432 MiB, so the ollama bucket correctly rejects; the error message names the right envelope.) This case proves the new correct envelope is named.
  - **(c)** The SST config-generator fails loud when `config/smackerel.yaml` references a model that does not fit its service envelope. (Run `./smackerel.sh config generate` against a yaml fixture with `llm.model = gemma4:26b` and `deploy_resources.ollama.memory = "8G"`; expect exit 1 with an error naming `llm.model`, `gemma4:26b`, and `deploy_resources.ollama.memory`.)

  **Adversarial signal requirement:** At least one of (a)/(b)/(c) MUST be a case that would have PASSED on HEAD `de49b2f9` (the pre-fix state) but is FAILING under the post-fix contract — making the regression detectable. Test case (a) is the obvious candidate: on HEAD `de49b2f9`, a model with profile 6000 MiB configured as `LLM_MODEL` with `ML_MEMORY_LIMIT=3G` would FAIL with the wrong-envelope error; under the post-fix contract it must PASS (because the ollama envelope, not the ml-sidecar envelope, governs `LLM_MODEL`).

- **AC-6 (`./smackerel.sh test integration` PASSES locally):** With the fix applied AND `config/smackerel.yaml` updated to a default model that fits the default ollama envelope, `./smackerel.sh test integration` exits 0 with no smackerel-core startup health-check failures. This proves the chronic CI failure is resolved at root.

- **AC-7 (`./smackerel.sh up` + `./smackerel.sh status` healthy):** With the fix applied AND default config consistent, `./smackerel.sh up` brings the stack to healthy state and `./smackerel.sh status` reports all services healthy. This proves the spec 052 live canary path is unblocked.

- **AC-8 (Spec 052 close-out artifacts updated):** `specs/052-bundle-secret-injection-contract/state.json` concerns `C-A12`, `C-B5`, `C-B6` are marked RESOLVED (or moved out of the `concerns` array) with concrete evidence references (substituted-bundle dry-run + healthy stack canary). `specs/052-bundle-secret-injection-contract/scopes.md` Scope-4 DoD checkboxes for the concerns are flipped to `[x]` with raw evidence inline. `specs/052-bundle-secret-injection-contract/report.md` Scope-4 sections add the resolution evidence (which run / which timestamp / which commit / which test output). **Scope discipline:** ONLY the close-out artifacts (state.json + report.md scope-4 sections + scopes.md DoD checkboxes) are touched. The main spec.md / design.md / scopes.md scope text of spec 052 stays unchanged.

- **AC-9 (`docs/Operations.md` "Model Envelope Sizing" section):** A new section in `docs/Operations.md` documents:
  - The per-service envelope contract (ollama envelope sizes ollama-routed models; ml-sidecar envelope sizes in-process ml-sidecar models).
  - The dev / home-lab / production model-selection trade-off (dev: 8G ollama envelope, small model; home-lab: 16-32G ollama envelope, gemma4:26b possible; production: tuned per workload).
  - The fix path order: raise the envelope FIRST in `config/smackerel.yaml` `deploy_resources.<service>.memory`, then change the model field.
  - A cross-reference to `services.ml.model_memory_profiles` for the catalog of measured model sizes.

- **AC-10 (All bubbles validators green):** All of the following exit 0:
  - `./smackerel.sh check`
  - `./smackerel.sh lint`
  - `./smackerel.sh test unit`
  - `./smackerel.sh test integration`
  - `bash .github/bubbles/scripts/cli.sh doctor`
  - `bash .github/bubbles/scripts/artifact-lint.sh specs/045-deploy-resource-filesystem-hardening`
  - `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/045-deploy-resource-filesystem-hardening`

## Out of Scope

- **Refactoring spec 045 main `spec.md`** — this is a bug under spec 045; spec evolution stays in the main spec.
- **Editing spec 052 main `spec.md` / `design.md` / `scopes.md` scope text** — only spec 052's close-out artifacts (state.json + report.md Scope-4 sections + scopes.md DoD checkboxes for C-A12/C-B5/C-B6) are updated to reflect the now-resolved concerns. The main spec/design/scopes scope text stays unchanged because the concerns themselves are not invalidated; they are simply resolved by the external (spec 045) unblock.
- **Editing other connector or feature specs** — out of scope. The fix only touches spec 045 implementation, spec 052 close-out artifacts, and `docs/Operations.md`.
- **Authoring a new model selection helper UX** — out of scope. The fix path is operator-facing (edit `config/smackerel.yaml`, run `./smackerel.sh config generate`), not a new tool.
- **Changing the deploy adapter contract** — out of scope. The deploy-target adapter consumes `OLLAMA_MEMORY_LIMIT` and `ML_MEMORY_LIMIT` from the bundled `app.env` exactly as today; no new env vars or contract changes are introduced by this fix.
- **Editing `deploy/compose.deploy.yml`** — out of scope unless the validator fix introduces a new env var (it does not — `OLLAMA_MEMORY_LIMIT` is already emitted and already substituted into `deploy/compose.deploy.yml` under `${OLLAMA_MEMORY_LIMIT:?...}`).
- **Increasing the default ollama envelope above 8G** — out of scope. The default `deploy_resources.ollama.memory = "8G"` is intentional for dev-box-fit; the fix is to change the default model to fit that envelope, not to grow the envelope. Home-lab and production operators may grow the envelope in their overlay.
- **Committing the fix** — out of scope for the discovery phase. Implementation commits happen in the `implement` phase under `bubbles.implement`.

## Severity Justification

**P1 — HIGH, NOT P2 — MEDIUM:**

- **Chronic CI failure on `main`** — 10 consecutive `CI` workflow runs on `main` have FAILED at HEAD `de49b2f9`. The CI signal is broken; future commits to `main` cannot be evaluated for CI regression because the integration suite always fails for the same root cause. This is a P1 condition for any project with CI as a quality gate.
- **Spec 052 home-lab canary blocked** — concerns `C-A12`, `C-B5`, `C-B6` were filed at severity:low but the SUM of three independent blockers on the same root cause is a P1 condition: the spec 052 L2/L3 defense-in-depth live-stack canary cannot be exercised until this is fixed.
- **Out-of-the-box `./smackerel.sh up` is broken on default config** — any new operator who clones the repo and runs `./smackerel.sh up` on a dev box will hit the wrongly-named-envelope error and be misled into raising `ML_MEMORY_LIMIT` (which does nothing for gemma4:26b). This is a P1 condition for product onboarding.

**Not P0 — CRITICAL** because:
- The system DOES fail loudly (not silently), so no data loss or security exposure.
- An operator with sufficient ollama envelope (16G+) and the cross-service routing knowledge CAN make `./smackerel.sh up` work by hand-editing `config/smackerel.yaml` (raise `deploy_resources.ollama.memory` to 32G or larger and use a host with enough RAM).
- The defect is contained to envelope validation, not to runtime data plane.

## Suggested Fix Direction

This section provides direction for `bubbles.design` and `bubbles.plan` in the next phases; the design specialist owns the final design and may choose alternatives.

### Suggested implementation sketch

In `internal/config/config.go`:

1. Add `OllamaMemoryLimitMiB int` field to the `Config` struct (mirroring `MLMemoryLimitMiB`).
2. Add a parse step at the appropriate location (next to the existing `MLMemoryLimit` parse at lines 694-700):
   ```go
   if cfg.OllamaMemoryLimit != "" {
       mib, err := parseComposeMemoryToMiB(cfg.OllamaMemoryLimit)
       if err != nil {
           return nil, fmt.Errorf("OLLAMA_MEMORY_LIMIT: %w", err)
       }
       cfg.OllamaMemoryLimitMiB = mib
   }
   ```
3. Rewrite `validateMLModelEnvelope()` (consider renaming to `validateModelEnvelopes()` for clarity — `bubbles.design` decides) to maintain two model buckets keyed by deploy service:
   ```go
   ollamaRouted := []modelRef{
       {"LLM_MODEL", c.LLMModel},
       {"OLLAMA_MODEL", c.OllamaModel},
       {"OLLAMA_VISION_MODEL", c.OllamaVisionModel},  // if field exists; verify with bubbles.design
       // ... full set per AC-1
   }
   mlSidecarRouted := []modelRef{
       {"EMBEDDING_MODEL", c.EmbeddingModel},
   }
   // Check each bucket against its envelope; name the correct envelope in errors.
   ```
4. For root cause (c), add a `validateConfigDefaultsAgainstEnvelopes()` step that runs over the parsed `config/smackerel.yaml` (NOT just the env values) BEFORE the env file is emitted. `bubbles.design` to choose location (Go-side or shell-side).

### Suggested default model

`bubbles.design` (or `bubbles.implement` with cited resident-size measurements) should pick a default model that:
- Fits `deploy_resources.ollama.memory = "8G"` envelope (profile ≤ 8192 MiB).
- Is competent at the workloads the smackerel core uses ollama for (text, vision if vision-capable, OCR, reasoning).
- Has a measured resident-size cited from `ollama ps` or similar.

The bug discovery phase does NOT pick the model — that decision is for `bubbles.design` informed by current model availability and quality. Candidate families to consider: `qwen2.5:7b-instruct` (~5G), `llama3.2:8b` (~6G), `gemma3:4b` (~4G), or similar. The model MUST be added to `services.ml.model_memory_profiles` with a cited resident-size measurement.

### Open questions for `bubbles.design`

- **Q-1:** Should `validateMLModelEnvelope` be renamed to `validateModelEnvelopes` (or similar) to reflect the multi-envelope reality? This breaks no external API but affects scope size.
- **Q-2:** Should the config-generate-time self-consistency check (AC-3) live in Go (`internal/config/config.go` `Validate()` over the yaml in-memory) or in shell (`scripts/commands/config.sh`)? Go gives a unified validation surface; shell gives earlier failure (before any Go binary is invoked). Trade-off for design to weigh.
- **Q-3:** Should the photos.intelligence.*_model fields be added to the validator's coverage now, or scoped to a follow-up packet? The user request includes them in AC-4 (default change) AND in AC-1 (validator coverage); recommend in-scope to keep the discovery → fix → close-out cycle clean.
- **Q-4:** What is the resolution path for the spec 052 close-out artifacts (AC-8)? Specifically: should `bubbles.validate` re-certify spec 052 after the close-out artifacts are updated, or is a lighter-weight "concern-marked-resolved" update sufficient? The spec 052 status is `done_with_concerns` (not `done`), so completion certification was already issued; the question is whether resolving a concern requires re-running the validate gate matrix. Recommend `bubbles.design` decide based on the agent ownership model.

## Workflow Mode Justification

**`bugfix-fastlane`** is the correct workflow mode because:
- The defect is scoped, with a clear contract (the four root causes a/b/c/d) and adversarial regression coverage available.
- The fix loop is bounded (a single repair to one validator function + a config default change + a doc section + spec 052 close-out updates).
- The reproduction (chronic CI failure + spec 052 concerns) provides red-state evidence already; the workflow mode's `forceTddMode: scenario-first` constraint applies cleanly.
- The `bubbles.goal` orchestrator parent-expanded this workflow because the nested runtime lacks `runSubagent`. The expanded `bugfix-fastlane` `phaseOrder` is: `select → bootstrap → implement → test → regression → simplify → gaps → harden → stabilize → devops → security → validate → audit → finalize`.

## References

- **Parent Spec:** `specs/045-deploy-resource-filesystem-hardening/` — owns FR-045-002 (the contract this bug repairs). This packet does NOT modify the parent spec's `spec.md`; the FR text stays unchanged.
- **Affected file (validator):** [`internal/config/config.go`](../../../../internal/config/config.go) lines 1745-1801 — `validateMLModelEnvelope()` (the function with the cross-service routing bug).
- **Affected file (config defaults):** [`config/smackerel.yaml`](../../../../config/smackerel.yaml) lines 53/56/57/563/572/683/685/686 (model fields default to `gemma4:26b`); lines 765-787 (`services.ml.model_memory_profiles`); lines 788-810 (`deploy_resources`).
- **Affected file (SST emitter):** [`scripts/commands/config.sh`](../../../../scripts/commands/config.sh) line 446 — `OLLAMA_MEMORY_LIMIT` already emitted, no widening required.
- **Affected file (documentation):** [`docs/Operations.md`](../../../../docs/Operations.md) — needs new "Model Envelope Sizing" section per AC-9.
- **Cross-spec close-out target:** [`specs/052-bundle-secret-injection-contract/`](../../../052-bundle-secret-injection-contract/) — concerns `C-A12`, `C-B5`, `C-B6` in `state.json`; matching DoD items in `scopes.md` Scope 4; matching evidence sections in `report.md` Scope 4.
- **CI evidence:** GitHub Actions `CI` workflow runs 170-179 on `main` (all FAILURE). Latest run: `https://github.com/pkirsanov/smackerel/actions/runs/25931687144`. Spec 052 close-out commit `d1e74a1f` triggered run `25904480081` which exhibits the same failure pattern. Per-job logs require admin auth; the local reproduction in the "Reproduction" section above is the authoritative evidence for this packet.
- **Binding policy (workspace-facing):** [`.github/copilot-instructions.md`](../../../../.github/copilot-instructions.md) — "SST Zero-Defaults Enforcement (NON-NEGOTIABLE)" subsection inside Required Runtime Standards.
- **Binding policy (agent-facing):** [`.github/instructions/smackerel-no-defaults.instructions.md`](../../../../.github/instructions/smackerel-no-defaults.instructions.md) — Gate G028 NO-DEFAULTS / fail-loud SST policy. The fix MUST preserve fail-loud behavior at every read site; the parse step for `OllamaMemoryLimit` MUST fail loud if the value is malformed (mirroring the `MLMemoryLimit` parse pattern that returns `fmt.Errorf("ML_MEMORY_LIMIT: %w", err)`).
- **Skill:** [`.github/skills/smackerel-no-defaults/SKILL.md`](../../../../.github/skills/smackerel-no-defaults/SKILL.md) and [`.github/skills/bubbles-config-sst/SKILL.md`](../../../../.github/skills/bubbles-config-sst/SKILL.md) — load and apply during the implement phase.
- **HEAD at discovery:** `de49b2f9ef01ad7477f75799bfb4db726ee43490` (short `de49b2f9`), 2026-05-16T16:29:50Z.
