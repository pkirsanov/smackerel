---
name: bubbles-code-index-adapter
description: Author, wire, and consume an OPTIONAL code-index adapter so a downstream repository can derive structural source facts (symbols, blast radius, affected tests, route inventory) instead of asserting them from hand-maintained lists. Use when adding a code-index provider, when wiring `codeIndex.adapter` in a project config, when a gate or lint wants reachability/impact facts that grep cannot produce, when deciding whether a code-index provider suits a repository's languages, or when a consumer must degrade because no index is configured. Default is `none` — the framework never depends on an external indexer.
---

# Code-Index Adapter

## What this is

A uniform, **opt-in, default-off** seam that lets a repository expose *derived
structural facts about its own source* to Bubbles consumers, through a swappable
provider adapter.

It exists because of a recurring failure mode: **coverage asserted from
hand-written lists rather than derived from the file surface.** A gate that
enumerates its own scope by hand can emit an absolute correctness claim about a
scope it does not cover — and stay green while doing it. Deriving the surface
removes the class of defect rather than patching one instance of it.

This seam is modeled directly on the observability adapter
(`bubbles/adapters/observability/`): flat per-provider scripts, a neutral
`none` default, verb dispatch, canonical normalized shapes, and a shape
selftest that runs with no provider installed.

## Non-goals (read this before proposing a use)

- **NOT an agent context-retrieval mechanism.** It does not reduce an agent's
  prompt-bundle cost, which is dominated by always-loaded shared markdown.
  Do not justify adoption on token savings.
- **NEVER a blocking verdict.** A third-party index inside a blocking gate is a
  worse dependency than the gap it closes. Structural facts feed **advisory
  nudges**; the authoritative verdict stays with existing framework checks.
- **NOT a framework dependency.** The framework MUST behave identically when
  the adapter is `none`, which is the default everywhere.

## The contract

Eight verbs. Four return a JSON **array** of records; `status`, `freshness`, and
`sync` return a JSON **map**; `indexed` returns an **array** of file records.
Neutral empty values are `[]` and `{}`.

| Verb | Args | Shape | Neutral | Answers |
|---|---|---|---|---|
| `symbols` | `<query>` | array | `[]` | where is this defined / what matches |
| `impact` | `<symbol>` | array | `[]` | blast radius — what breaks if this changes |
| `affected` | `<file>...` | array | `[]` | which tests can this diff actually reach |
| `routes` | — | array | `[]` | full route/endpoint inventory |
| `indexed` | — | array | `[]` | **which files carry graph nodes** (see below) |
| `status` | — | map | `{}` | index health and statistics |
| `freshness` | — | map | `{}` | **is the index still true?** (see exit 2) |
| `sync` | — | map | `{}` | bring a stale index up to date (only mutating verb) |

`indexed` exists to disambiguate an empty result. "Zero affected tests" has two
causes that demand opposite reactions: nothing graph-participating changed
(correct and boring), or the graph missed an edge (a correctness risk). They look
identical in a subset count. Each record carries a `nodeCount`; **`nodeCount == 0`
means the file is known to the index but carries no symbols**, so it can never
yield a dependent. A consumer that reports a would-skip percentage without this
triage will raise false alarms — and, worse, teach its readers to ignore real ones.


`<file>` arguments are **repo-relative** — relative to `CODEINDEX_ROOT`, not to
the caller's CWD. The adapter may be invoked from anywhere in a multi-root
workspace, so caller-relative paths have no stable meaning.

Plus `selftest <verb>`, which emits the canonical shape **with no provider
installed** so a shape lint can validate any adapter offline.

### Exit-code semantics (do not blur these)

| Exit | Meaning | Consumer MUST |
|---|---|---|
| 0 + neutral value | indexed, nothing found | proceed; absence is a real answer |
| 0 + records | indexed, facts available | may use as advisory input |
| **2** (`freshness` only) | indexed but **STALE** — the code moved | treat facts as untrustworthy; `sync`, or degrade |
| 1 | provider missing / no index / provider failed | degrade to existing behavior |

A provider adapter MUST NOT emit a neutral value when it is merely unavailable.
`[]` means "I looked and found nothing"; exit 1 means "I could not look." A
consumer that conflates them will silently report a clean result for an
unindexed repository — the exact false-green this seam exists to prevent.

`freshness` extends the same principle to time. An index is a point-in-time
snapshot: the moment a file changes, every fact derived from it may be wrong,
and nothing about `symbols`/`impact`/`affected`/`routes` output looks any
different. Exit 2 makes "confidently answering from last week's code" a
detectable state rather than an invisible one.

Critically, an adapter that **cannot determine** freshness MUST exit 2, not 0.
"I don't know whether I'm stale" is not "I am fresh" — defaulting to fresh
reintroduces the silent-wrong-answer failure this verb exists to close.

`sync` is the only verb permitted to mutate anything, and it mutates only the
index — never the working tree, never git state. Because `none` returns `{}`
exit 0, a repository can wire `freshness || sync` unconditionally into a CLI or
git hook and stay correct whether or not a provider is ever adopted.

## Wiring a repository (opt-in)

Project-owned config, never framework-managed:

```yaml
# .github/bubbles-project.yaml
codeIndex:
  adapter: codegraph      # or: codebase-memory, none (default)
```

Resolve it:

```bash
bash bubbles/scripts/codeindex-resolve.sh --repo-root . --names-only
# -> adapter=none      (no config, no codeIndex block, or explicit none)
```

Resolution rules that matter:

- Absent config, absent `codeIndex:` block, and explicit `none` all resolve to
  the neutral adapter and **exit 0**.
- An `adapter:` key **outside** the `codeIndex:` block is ignored (block-scoped).
- An unknown or unsafe adapter value **fails loud (exit 1)**. It never degrades
  silently to `none`, because a silent degrade hides an operator typo behind a
  green run.

## Adding a provider adapter

1. Create `bubbles/adapters/codeindex/<provider>.sh`, `chmod +x`.
2. Implement all eight verbs plus `selftest <verb>` and `capabilities`.
3. Normalize provider output to the canonical shapes — never pass a raw
   provider envelope through.
4. Fail loud on missing provider/index (exit 1). No auto-install, no network
   fetch, no default binary path.
5. Force provider telemetry **off** if the provider ships it on by default.
6. Add the provider to the selftest's contract sweep and confirm all verb
   selftests pass with no provider installed.
7. Declare any verb you cannot support honestly as `unsupported` in
   `capabilities` and exit 1 for it. Do **not** approximate. A plausible-looking
   wrong answer is worse than a refusal: an `affected` derivation that returns 6
   test files when the true blast radius is 1,193 will be trusted, and the 1,187
   skipped tests fail silently. Refusal degrades the consumer to the full suite,
   which is merely slow.

## Choosing a provider

Match the provider's grammar set to the repository's actual composition before
adopting — this is the most common selection error.

| Repository shape | Guidance |
|---|---|
| Compiled/typed application source (Rust, Go, TypeScript, Java, C#, Swift, Kotlin) | Well served by AST/tree-sitter indexers |
| Shell-dominant or documentation-dominant | Verify grammar coverage first; several indexers do **not** parse shell at all |
| Mixed source + docs/PDF/media | Requires a provider with a semantic pass; that pass typically needs a model and is no longer fully local |

Measure the repository first (`git ls-files | sed -E 's/.*\.//' | sort | uniq -c
| sort -rn`) rather than assuming. A provider that cannot parse the dominant
language of a repository provides no value there regardless of its benchmarks.

## Two access paths (do not confuse them)

| Path | Consumer | Mechanism | Freshness |
|---|---|---|---|
| **MCP** | agents, interactively | the provider's own MCP server, registered in the editor's MCP config | provider-managed (watcher + debounced sync + connect-time catch-up) |
| **Adapter** | scripts, gates, CLIs | `adapters/codeindex/<provider>.sh <verb>` | caller-managed: `freshness \|\| sync` |

An agent asking "who calls this?" should use the MCP path. A lint deriving an
inventory should use the adapter path. Registering one does not give you the
other, and **neither is created by setting `codeIndex.adapter`** — that config
only tells the adapter path which provider to use.

## When an agent should reach for the index

Availability is not usage. An agent handed a new tool with no guidance keeps
grepping, and the integration stays inert. Reach for the index FIRST on
structural questions — these are exactly the cases where grep is weakest:

| Question | Why grep is worse |
|---|---|
| Where is this symbol defined? | grep finds every mention, not the definition |
| Who calls this? | grep cannot distinguish call from comment, string, or import |
| What breaks if I change this? | grep has no transitive closure |
| Which tests can this diff reach? | grep has no dependency graph at all |
| What endpoints exist? | grep misses builder-wired and nested registrations |

Fall back to grep or direct reads when the index returns empty, reports stale,
or exits non-zero — and when the question is textual rather than structural
("where do we mention this TODO"), grep was always the right tool.

Two habits that keep this honest:

- **Confirm before escalating.** A derived fact is a claim. Open the source for a
  sample before acting on a count.
- **Prefer it while editing, not only while investigating.** Blast radius is most
  useful *before* the change, not after the review.

## Legitimate consumers

Both are **advisory** and both must degrade cleanly on `none` or exit 1:

1. **Reachability nudges (integration-completeness family).** "Every shipped
   artifact has a real consumer" is a graph-reachability question. Where it is
   currently satisfied by an existence check plus authoring discipline, an
   `impact` derivation can surface artifacts with no inbound edge as a
   nudge — never as the verdict.
   **`routes` does NOT close this today**, on either shipped provider. Measured:
   codegraph's route nodes on a Go+TS repo are frontend router entries and
   test-file routes rather than backend handlers, a chi-nested Go router yields
   bare fragments (`ANY /`) because the mount-prefix chain is not reconstructed,
   and `codebase-memory` emits an empty route `file_path` with a `HANDLES` edge
   on a small minority of routes. Treat `routes` as a rough inventory only.
2. **Impact-aware validation planning.** Where a test-impact map is
   hand-maintained (or absent, in which case the full suite always runs),
   `affected` derives the same mapping from the real dependency graph.

## Evidence standard

A derived fact is still a claim. When a consumer reports one:

- Record the adapter name and the exact verb invocation.
- Treat a derived finding as **advisory until hand-verified**; confirm a sample
  against the source before escalating it.
- Never present derived counts as certified. Derivation removes whole classes of
  omission, but it does not exempt a claim from the framework's evidence rules —
  a structural count that disagrees with a hand count must be reconciled, not
  assumed correct because a tool produced it.

## See also

- `bubbles/adapters/codeindex/` — `none.sh` (default), `codegraph.sh`,
  `codebase-memory.sh` (the only provider that parses shell)
- `bubbles/scripts/codeindex-resolve.sh` — resolver
- `bubbles/scripts/codeindex-resolve-selftest.sh` — hermetic contract selftest
- `bubbles-observability-adapter` — the adapter pattern this mirrors
- `bubbles-quality-gates-catalog` — the gates whose advisory inputs this can feed
