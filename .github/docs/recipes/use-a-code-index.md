# Recipe: Use A Code Index (Derived Structural Facts)

**When to use:** A gate, lint, or review in your repo answers a *structural*
question — "does anything actually reach this endpoint?", "which tests can this
diff touch?", "what breaks if I change this?" — by grepping, or by consulting a
list somebody maintains by hand.

> **Consumer status:** one framework consumer is wired —
> [`test-impact-shadow.sh`](../../bubbles/scripts/test-impact-shadow.sh) — and it
> is deliberately **ADVISORY ONLY**. It reports the test subset a code index
> *would* have selected so you can compare it against reality; it never skips a
> test, never gates, and no exit code it returns encodes "safe to skip". No
> gating consumer exists. Opting in today gives you a uniform CLI surface for
> your own repo-local gates and lints plus that shadow report; it does not
> change any pass/fail Bubbles behavior. Treat that as the honest state, not as
> a soon-to-land promise.

---

## Why this exists

The recurring defect it targets: **coverage asserted from a hand-written list
rather than derived from the file surface.** A gate that enumerates its own
scope by hand can emit an absolute correctness claim about a scope it does not
cover — and stay green while doing it.

A real, measured example from a downstream product repo: a purpose-built
coverage audit reported `100% coverage — all route-registered handlers are
annotated`, exit 0. Its scan scope was two directories. Four other files
registered 35 routes outside that scope, 25 of them business endpoints absent
from the generated API contract — 9 on the paid public surface, including state
mutations. The audit was not wrong about what it measured; it was wrong that
"all" applied. Deriving the route inventory found it in one query.

---

## The shipped adapters

| Adapter | What it does | Requires |
|---------|--------------|----------|
| `none` | Every verb returns its neutral empty value (`[]` / `{}`). Safe default. | — |
| `codegraph` | Wraps the `codegraph` CLI (local tree-sitter → SQLite; no API key, no network) | `codegraph` on PATH or `CODEINDEX_CODEGRAPH_BIN`, plus an index in the repo |
| `codebase-memory` | Wraps the `codebase-memory-mcp` CLI (MIT, single static binary, local-only). **Parses shell**, which `codegraph` does not — so it is the only provider that can see this framework's own source. Declares `affected` and `freshness` *unsupported* rather than approximating them. | `codebase-memory-mcp` on PATH or `CODEINDEX_CODEBASE_MEMORY_BIN`, plus `python3` |

Providers are not interchangeable, and the differences are load-bearing: pick
`codegraph` when you need test-impact or a real freshness signal, and
`codebase-memory` when you need shell coverage. Ask any adapter what it actually
supports with `capabilities` instead of assuming.

Live in `bubbles/adapters/codeindex/<name>.sh`. The contract is 8 verbs:

| Verb | Args | Shape | Answers |
|------|------|-------|---------|
| `symbols` | `<query>` | array | where is this defined |
| `impact` | `<symbol>` | array | blast radius — what breaks if this changes |
| `affected` | `<file>...` | array | which tests can this diff actually reach |
| `routes` | — | array | full route/endpoint inventory |
| `indexed` | — | array | which files the index actually covers |
| `status` | — | map | index health |
| `freshness` | — | map | is the index current (**exit 2 = STALE**) |
| `sync` | — | map | bring the index up to date |

Plus two non-fact verbs: `selftest <verb>` (provider-free shape check) and
`capabilities` (a map of verb → `native` \| `derived` \| `unsupported`).

### The exit codes are the contract

| Exit | Means |
|------|-------|
| `0` + records | facts |
| `0` + `[]` / `{}` | **indexed, and genuinely found nothing** |
| `2` | `freshness` only — the index is STALE |
| `1` | **could not look** — no provider, no index, verb unsupported |

Conflating `0` + `[]` with `1` is the single trap this seam exists to prevent:
the first means "I checked, there is nothing", the second means "I could not
check". A consumer that treats them alike will report clean on an unindexed
repository. An adapter that cannot determine freshness MUST exit `2`, never `0`.

---

## Decide first: is it worth it for THIS repo?

A provider that cannot parse your dominant language provides nothing, regardless
of its benchmarks. Measure before adopting:

```bash
git ls-files | sed -E 's/.*\.//' | sort | uniq -c | sort -rn | head -10
```

| Your repo is mostly | Guidance |
|---------------------|----------|
| Compiled/typed app source (Rust, Go, TypeScript, Java, C#, Swift, Kotlin, Scala, Dart) | Good fit |
| Shell scripts | Check the provider's grammar list — several indexers do **not** parse shell at all |
| Markdown / docs | Needs a provider with a doc/semantic pass; that pass usually needs a model and is no longer fully local |

---

## What to do in your repo (downstream checklist)

**1. Install the provider in an isolated prefix.** Do not install globally; keep
removal to one `rm -rf`.

```bash
npm install --prefix ~/.cache/codegraph-eval @colbymchenry/codegraph
```

**2. Keep the index out of git BEFORE you build it.** The index directory is
large and is usually *not* in your `.gitignore`. An untracked multi-hundred-MB
directory can be swept into an unrelated commit — especially if more than one
agent session is active in the repo.

Add `.codegraph/` to `.gitignore` (shared) or `.git/info/exclude` (local only).

**3. Build the index, with provider telemetry off.**

```bash
CODEGRAPH_TELEMETRY=0 DO_NOT_TRACK=1 CODEGRAPH_NO_DAEMON=1 \
  ~/.cache/codegraph-eval/node_modules/.bin/codegraph init
```

> Provider telemetry is ON by default upstream. The adapter forces it off on
> every invocation; the manual `init` above is outside the adapter, so set it
> yourself. On WSL, keep the repo on the Linux-native filesystem — a repo under
> `/mnt/c` cannot use SQLite WAL and will be slow or flaky.

**4. Opt in.** In `.github/bubbles-project.yaml` (or `bubbles-project.yaml`):

```yaml
codeIndex:
  adapter: codegraph      # default is: none
```

**5. Verify the resolution and the index.**

```bash
bash .github/bubbles/scripts/codeindex-resolve.sh --repo-root . --names-only
# -> adapter=codegraph

bash .github/bubbles/adapters/codeindex/codegraph.sh status
```

**6. Use it.** Start with `affected`, and run it in shadow before trusting it:

- **Impact-aware validation.** If your repo has no `testImpact:` map, every
  change runs the full suite. `affected` derives the mapping from the real
  dependency graph instead of asking someone to maintain it. Run
  [`test-impact-shadow.sh`](../../bubbles/scripts/test-impact-shadow.sh)
  alongside the real suite first and keep a divergence log; it never gates, and
  no exit code it returns means "safe to skip". Promote to gating only after
  that log stays empty. Measured on an 8,276-file Go+TS repo: one changed file
  selected 141 of 1,793 tests — a *candidate* 92% skip, not a result.
- **Blast radius before a change.** `impact` and `symbols` answer "what calls
  this" far better than grep, need no freshness beyond a sync, and nothing gates
  on them — so they are safe to adopt on day one.
- **Coverage-claim honesty.** `indexed` reports which files the index actually
  covers, so a gate can refuse to claim "all" beyond its own scope — the exact
  defect class in *Why this exists* above.

> **Do NOT use `routes` for orphan-endpoint detection.** It is the obvious
> reading of this seam and it does not work on either shipped provider. Measured
> on a Go+TS repo, codegraph's 955 `route` nodes are frontend React-Router
> entries and *test-file* routes, not backend HTTP handlers; a Go repo using
> chi's nested `r.Route()` yields fragments like `ANY /` because the mount-prefix
> chain is never reconstructed. `codebase-memory` is no better: route
> `file_path` is empty and only 34 of 1,427 routes carry a `HANDLES` edge.
> Closing an "every endpoint must have a consumer" policy mechanically needs
> language-aware router-tree analysis that neither provider performs today.

**To reverse everything:** `codegraph uninit --force`, remove the
`.gitignore`/exclude line, `rm -rf ~/.cache/codegraph-eval`, and set
`adapter: none`. Nothing else in the repo is touched.

---

## Test the adapter directly

```bash
bash bubbles/adapters/codeindex/codegraph.sh routes
bash bubbles/adapters/codeindex/codegraph.sh impact someFunction
```

Exit 0 with JSON on success. **Exit 1 means "I could not look"** — provider
missing, no index, or provider failure. That is informational, not a framework
failure.

---

## The distinction that matters most

`[]` and exit 1 are **not** the same answer:

| Result | Means | Your check MUST |
|--------|-------|-----------------|
| exit 0, `[]` | indexed, genuinely nothing found | treat absence as a real answer |
| exit 1 | could not look at all | degrade to existing behavior — never report clean |

A consumer that conflates them silently reports a clean result for an unindexed
repository. That is the same false-green this recipe exists to eliminate.

---

## Rules for consuming derived facts

1. **Advisory, never the verdict.** A third-party index inside a blocking gate
   is a worse dependency than the gap it closes. Structural facts feed nudges;
   the authoritative verdict stays with existing checks.
2. **Verify a sample before escalating.** A derived finding is still a claim.
3. **Reconcile disagreements, don't defer to the tool.** When a derived count
   and a hand count disagree, one of them is wrong and you do not yet know
   which. During the investigation above, the *hand* count was the wrong one —
   which is the point, but it cuts both ways.
4. **Do not adopt this for agent context or token savings.** It does not reduce
   an agent's prompt-bundle cost, and delegation to file-reading sub-agents
   erases the benefit anyway.

---

## Authoring a new adapter

See [skill: bubbles-code-index-adapter](../../skills/bubbles-code-index-adapter/SKILL.md).

Short version:
1. Create `bubbles/adapters/codeindex/<name>.sh`, `chmod +x`.
2. Implement all 8 verbs plus `selftest <verb>` and `capabilities` in a `case`
   statement. A verb you cannot support honestly should report `unsupported` in
   `capabilities` and exit 1 — never return a confident approximation.
3. Normalize provider output to the canonical shapes — never pass a raw
   provider envelope through.
4. Fail loud (exit 1) when the provider or index is missing. No auto-install,
   no network fetch, no default binary path.
5. Run `bash bubbles/scripts/codeindex-resolve-selftest.sh` — should exit 0.

---

## Graceful degradation

With `adapter: none` — the default everywhere — every verb returns its neutral
value and the framework behaves exactly as it did before. Nothing in Bubbles
requires a code index. It is derivation, not infrastructure.

---

## Quote

> *"It's not rocket appliances."* — Ricky
