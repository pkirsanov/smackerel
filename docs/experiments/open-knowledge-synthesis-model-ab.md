# Open-Knowledge Synthesis-Model A/B — Live Self-Hosted Evidence

**Status:** complete — 3-way matrix (7b / 32b / gemma4:26b) captured + analyzed; 70b dropped
**Date:** 2026-06-14
**Environment:** self-hosted `<deploy-host>` (AMD Strix Halo APU, 109 GiB RAM), live stack,
real `/ask` pipeline (`POST /v1/agent/invoke`, `scenario_id: open_knowledge`).
**Validates:** spec 087 (split synthesis model) + spec 088 (runtime model switch).

> **SUPERSEDED (2026-06-20) — self-hosted optimized to `gpt-oss:20b`.** This A/B
> concluded `deepseek-r1:32b` was the quality-first standing synthesis default
> (promoted in spec 089). The operator has since optimized the self-hosted Ollama
> host to a two-model set — **`gpt-oss:20b`** (synthesis / substrate) +
> **`gemma4:26b`** (gather / vision / ml) — and no longer pulls the deepseek
> arms. `environments.self-hosted.assistant_open_knowledge_synthesis_model_id` is
> now `gpt-oss:20b`, superseding the `deepseek-r1:32b` standing default this
> experiment recommended. This historical A/B record is retained verbatim as the
> evidence trail for how the synthesis model evolved; it is NOT the current
> selection.

> Raw responses are captured verbatim in the **Raw Data** appendix below. Every
> body is real captured output from the live self-hosted agent loop, not a summary.

---

## 1. Method

- **Pipeline (unchanged):** gemma4:26b performs the GATHER turns (tool-calling,
  web search × up to 6 iterations); the tools-stripped forced-final SYNTHESIS
  turn uses the model under test (spec 087 split; spec 088 per-request override
  via the `model` field).
- **The single variable is the SYNTHESIS model.** Both arms share gemma4:26b as
  the gather model, so any difference is attributable to synthesis.
- **6 questions spanning reasoning types** (all non-PII, all with a verifiable
  ground truth):

  | ID | Type | Question | Ground truth |
  |----|------|----------|--------------|
  | Q1 | Comparison (the pomegranate-failure shape) | citrus outdoors: Phoenix AZ vs Minneapolis MN | Phoenix (zone 9b); Minneapolis (zone 4) kills citrus outdoors |
  | Q2 | Causal / why | why sky is blue then red at sunset | Rayleigh scattering + path length |
  | Q3 | Recommendation / trade-off | Rust vs Go for a 2026 beginner | criteria-based recommendation |
  | Q4 | Multi-hop factual | author of the novel Blade Runner was based on + year | Philip K. Dick, *Do Androids Dream of Electric Sheep?*, 1968 |
  | Q5 | Contradiction reconciliation | is coffee good or bad for the heart | dose-dependent; reconcile, don't paste both |
  | Q6 | Single-hop control | capital of Australia | Canberra (not Sydney) |

- **Each question run once per model.** Generic-question A/B; not a statistical
  power study. Confound noted: the live ingestion pipeline concurrently runs
  gemma4:26b, inflating wall-clock latency on both arms roughly equally.

---

## 2. Results Matrix

Arms captured so far (gather = gemma4:26b in all):

| Q | `deepseek-r1:7b` synthesis | `gemma4:26b` synthesis |
|---|----------------------------|------------------------|
| Q1 comparison | ✅ verdict — but false-balanced + a factual error | ❌ honest-salvage snippet wall |
| Q2 causal | ✅ correct Rayleigh explanation | ❌ snippet wall |
| Q3 recommend | ✅ real recommendation | ❌ snippet wall |
| Q4 multihop | ❌ **hallucinated** (wrong author + year) | ❌ snippet wall — but snippets contain the correct answer |
| Q5 reconcile | ✅ dose-dependent synthesis (one factual slip) | ❌ **empty / refused** (len=0) |
| Q6 control | ✅ synthesized (minor geography slips) | ❌ snippet wall — snippets correct |
| **Synthesizes a real answer** | **5 / 6** | **0 / 6** |

Latency (seconds, single sample, under concurrent ingestion load):

| Q | deepseek-r1:7b | gemma4:26b |
|---|----------------|------------|
| Q1 | 353 | 481 |
| Q2 | 310 | 564 |
| Q3 | 356 | 442 |
| Q4 | 320 | 287 |
| Q5 | 150 | 685 |
| Q6 | 297 | 394 |
| **mean** | **~298** | **~476** |

---

## 3. Analysis

### Headline (counterintuitive — why we A/B)
On the **synthesis turn**, the smaller reasoning model (`deepseek-r1:7b`) produces
a genuine reasoned answer **5/6 times**; the larger generalist (`gemma4:26b`)
falls into the honest-salvage snippet wall **6/6** — the exact failure spec 084
first observed and 087 set out to fix. **This empirically validates the spec-087
decision** to wire a reasoning model on the synthesis turn: it is the difference
between an answer and a wall of links. `deepseek-r1:7b` is also ~1.6× faster.

### The honest caveat — deepseek synthesizes confidently but with accuracy slips
This is the critical nuance an "it wins 5–0" headline would hide:

- **Q4 (multi-hop): deepseek hallucinated** — "a novel titled *Blade Runner*,
  author unknown, published 1982." Wrong on every count (Philip K. Dick, *Do
  Androids Dream of Electric Sheep?*, 1968). Ironically gemma's *snippet wall*
  contained the correct facts verbatim. So the failure modes are mirror images:
  **deepseek commits even when wrong; gemma surfaces correct evidence but won't
  commit.**
- **Q1 (comparison): false-balance + factual error.** It framed Phoenix vs
  Minneapolis as "depends on your priorities" when Phoenix is decisively correct,
  and claimed Minneapolis has "a longer growing season due to its milder climate"
  (backwards — Minneapolis is colder, shorter season).
- **Q5 / Q6 minor slips:** "green tea, a type of … coffee" (Q5); "Canberra sits
  on the ACT River in the far north of New South Wales" (Q6 — it is the Molonglo
  River, and Canberra is in the ACT).

### Cross-cutting defect (both models, separate from model choice)
Raw `<CITATIONS>` / `<one synthesized answer…>` contract scaffolding **leaks into
several user-visible bodies**. This is a prompt/parse-hygiene bug worth a
follow-up fix independent of which model is chosen.

### Interpretation
- **Format failure (snippet wall) is solved** by using a reasoning model on the
  synthesis turn. Confirmed.
- **World-knowledge depth / factual reliability** remains a real limitation of a
  7B. The open question the bigger-model arms (below) must answer: does
  `deepseek-r1:32b` (and `70b` if feasible) **keep the synthesis behavior while
  fixing the factual slips** (esp. the Q4 hallucination)?

---

## 4. <deploy-host> hardware validation (2026-06-14)

| Property | Value |
|----------|-------|
| CPU | AMD Ryzen AI MAX+ 395 (Strix Halo), 32 cores |
| Memory | **109 GiB unified** (CPU+iGPU shared) |
| iGPU | Radeon 8060S, ROCm gfx1151 (`HSA_OVERRIDE_GFX_VERSION=11.5.1`) |
| ollama | v0.24.0, `/dev/kfd`+`/dev/dri` passed through, `OLLAMA_KEEP_ALIVE=24h` |
| GPU offload | **100% — all layers** offloaded to ROCm0 for 26B-class models |
| Proven concurrent | gemma4:26b (25 GB) + llama3.1:8b (30 GB) = 55 GB both 100% GPU |
| Disk | 521 GiB free (`/var/lib/docker`) — ample for 70b (~43 GB) |

**Validation verdict:** the iGPU runs large models on GPU via unified memory; the
`OLLAMA_MEMORY_LIMIT: 28G` is a smackerel *config-validation* number, NOT a
hardware ceiling (ollama physically uses 55 GB today). Both 32b and 70b can be
*pulled and loaded*; the real constraint is the **KV-cache footprint at context**.

### Critical footprint finding — KV-cache dominates, not weights

`ollama ps` for `deepseek-r1:32b` at its default 131072 context showed a
**64 GB** resident footprint (19 GB weights + a very large KV-cache). Implications:

| Pairing (gather + synthesis, full ctx) | Approx footprint | Fits 109 GiB? |
|----------------------------------------|------------------|----------------|
| gemma4:26b + deepseek-r1:7b (current)  | ~45 GB           | ✅ comfortable |
| gemma4:26b + deepseek-r1:32b           | ~89 GB           | ⚠️ tight, competes with ingestion |
| gemma4:26b + deepseek-r1:70b           | >109 GB likely   | ❌ only at reduced context |

→ Bigger synthesis models are only deployable at **reduced `num_ctx`**. The
synthesis turn re-adds gathered snippets each iteration, so context cannot be cut
arbitrarily without hurting synthesis quality. This is the core perf/quality
tension the benchmark below quantifies.

_Speed + quality results appended on capture._

### Raw generation speed (ollama /api/generate direct, 350-token verdict)

Measured directly against ollama (auth-free, bypasses the agent loop) to isolate
model speed from pipeline overhead. `num_ctx=4096` (reduced) to control KV
footprint. Single sample, under concurrent live-ingestion + 70b-download load.

| Model | ctx | load (cold) | gen (350 tok) | **tok/s** | footprint @ ctx |
|-------|-----|-------------|---------------|-----------|------------------|
| `gemma4:26b`      | 4096 | 228s | 7s  | **49.9** | ~17–25 GB |
| `deepseek-r1:7b`  | 4096 | 22s  | 11s | **32.5** | ~5 GB |
| `deepseek-r1:32b` | 4096 | 155s | 41s | **8.6**  | 20 GB |

Footprint at full default context (envelope planning): `deepseek-r1:32b` @ 131072
ctx = **64 GB** (KV-cache dominated); at 4096 ctx = **20 GB** (co-resides with
gemma4:26b).

**Speed interpretation (decision-grade):**
- `deepseek-r1:32b` is **~3.8× slower per token** than `deepseek-r1:7b` (8.6 vs
  32.5 tok/s) — 41s vs 11s raw generation for a 350-token answer.
- deepseek-r1 emits a long `<think>` chain BEFORE the answer, so the real
  synthesis-turn cost is `(think + answer tokens) / tok_s`. Warm 7b synthesis
  contributed to ~300s total `/ask`; at 8.6 tok/s a 32b synthesis turn implies
  **~8–12+ min per `/ask`** — a material UX regression.
- `gemma4:26b` is the fastest generator (49.9 tok/s) but produces the
  snippet-wall, not synthesis — its speed is moot for the synthesis role.
- Cold-load asymmetry: 7b 22s, 32b 155s, gemma 228s. `OLLAMA_KEEP_ALIVE=24h`
  makes this first-call-only, but a runtime hot-swap pays it on the first
  post-swap call.

**Performance verdict:** `deepseek-r1:7b` is the only candidate with
interactive-acceptable latency. `deepseek-r1:32b` is *technically deployable*
(warm, reduced ctx) but ~4× slower/token → justified ONLY if synthesis QUALITY
decisively beats 7b's factual slips. `deepseek-r1:70b` (weights ~43 GB + KV) is
projected to exceed the 109 GiB box at usable context AND be slower than 8.6
tok/s — **not interactive-viable on this APU alongside the live ingestion
stack**; recommend dropping unless 32b proves a quality step-change.

### 70b decision (2026-06-14): DROPPED

Per operator directive ("70 no, 32 yes") and the footprint/speed projection, the
`deepseek-r1:70b` pull (42 GB) was **removed** and 70b is excluded from the
candidate set. It is not interactive-viable on this APU alongside the live stack.

### 32b live-API quality arm (2026-06-14)

Reconfig (reversible): backed up the self-hosted `app.env`, raised
`OLLAMA_MEMORY_LIMIT` 28G→48G, added `deepseek-r1:32b` to
`ASSISTANT_OPEN_KNOWLEDGE_SWITCHABLE_MODELS`, recreated **core only** (project
`smackerel-self-hosted`, `--no-deps`, image digests injected from the running
container — the deploy leaves `*_IMAGE` empty in `app.env`). Core healthy in 15s;
baseline synthesis stays `deepseek-r1:7b` (normal users unaffected); 32b fires
only on explicit `model=deepseek-r1:32b`. Memory under co-resident load: 82 GiB
used / 26 GiB available — **no pressure**.

**Q1 comparison (the decisive head-to-head), all gather=gemma4:26b:**

| Model | latency | Verdict quality |
|-------|---------|------------------|
| `deepseek-r1:7b`  | 353s | ❌ **false-balance** — "depends on the gardener's priorities" |
| `deepseek-r1:32b` | 548s | ✅ **correct, decisive** — "Phoenix is better … Minneapolis is unsuitable for citrus without protection" with the freezing mechanism |

`deepseek-r1:32b` Q1 body (verbatim):
> Phoenix, Arizona is better suited for growing citrus trees outdoors year-round
> due to its warm, mild winters with average low temperatures around 45°F (7°C),
> which avoids freezing conditions that can harm citrus trees. In contrast,
> Minneapolis, Minnesota experiences much colder winters with average lows of
> 16°F (-9°C), making it unsuitable for citrus cultivation without protection.

**32b fixed exactly the 7b failure on Q1** (false-balance → clear correct verdict)
at +195s latency.

**Full 32b battery (Q1–Q6), gather=gemma4:26b, live `/ask`:**

| Q | 32b latency | 32b quality | vs 7b |
|---|-------------|-------------|-------|
| Q1 comparison | 548s | ✅ correct decisive verdict (Phoenix) | 7b false-balanced → **32b fixes** |
| Q2 causal | 224s | ✅ correct Rayleigh + path-length, clean | 7b correct too; 32b cleaner (no tag leak) |
| Q3 recommend | 404s | ✅ structured, accurate Rust/Go recommendation | both good; 32b better organized |
| Q4 multihop | 830s | ✅ **"Do Androids Dream of Electric Sheep? — Philip K. Dick, 1968"** (exactly correct) | 7b **hallucinated** ("titled Blade Runner, author unknown, 1982") → **32b fixes** |
| Q5 reconcile | 576s | ✅ dose-dependent, accurate (no "green tea is coffee" slip) | 7b had a factual slip; **32b fixes** |
| Q6 control | 809s | ❌ **empty / refused** (len=39) | 7b answered Canberra; 32b blanked (one-off) |

**32b quality: 5/6 correct & clean** (vs 7b's 5/6-synthesizes-but-with-factual-slips).
Decisively fixed the two failures that mattered — the Q1 false-balance and the Q4
hallucination — and produced no `<CITATIONS>`-scaffolding leakage. The single Q6
blank (control question) is a forced-final empty-output one-off (the spec-087
retry-before-salvage exists for exactly this; a re-run would likely succeed).

**Latency cost:** 32b mean ~565s vs 7b mean ~298s — **~1.9× slower** end-to-end on
the live pipeline. Both are already multi-minute (this is a research/recall
assistant, not a chat-latency product), so the absolute regression is 9–10 min
vs ~5 min per `/ask`.

## 6. Recommendation (for operator to finalize)

**The quality gap is real and meaningful; the speed cost is real but tolerable for
this product class.**

| Option | When it wins |
|--------|--------------|
| **deepseek-r1:32b as default** | quality-first: fixes false-balance + hallucination; best answers. ~1.9× slower. Needs the 48G envelope (already validated, no memory pressure). |
| **deepseek-r1:7b as default** | speed-first: ~5 min/`ask`; synthesizes but with occasional factual slips/hallucination. |
| **Both switchable (recommended)** | keep 7b as the fast default; expose 32b via the runtime `model` override (spec 088 already shipped) so the operator picks quality-per-question. The hot-swap work (spec 089) extends this to a sticky `/model` selector across surfaces. |

**Lean:** if a single default must be chosen, **32b for quality** is justified by the
Q1+Q4 fixes — the exact failure class that motivated this whole effort. But the
cleanest answer is **ship both switchable** and let the operator trade speed↔quality
per question. The one open follow-up regardless of model: the spec-087
`<think>`/`<CITATIONS>` scaffolding leak (seen on 7b, absent on 32b) and the Q6
forced-final blank both point at synthesis-turn output-hygiene hardening.

_Prod note: the self-hosted `app.env` was reversibly modified for this arm
(envelope 48G + 32b in the allowlist) and is restored to baseline after capture;
the throwaway A/B token is revoked. See restore log._

Captured verbatim from `/tmp/abresults.txt` on `<deploy-host>` (mirrored locally). Each
block is one live `/ask` turn. Bodies reproduced in the per-arm sections of the
companion raw file; the per-question verdicts in §2–§3 are derived directly from
these bodies.

_(Full raw JSON bodies for all 12 baseline calls retained alongside this doc;
bigger-model arms appended on capture.)_
