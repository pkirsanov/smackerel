# Recipe: Use A Code Index (Derived Structural Facts)

**When to use:** A gate, lint, or review in your repo answers a *structural*
question — "does anything actually reach this endpoint?", "which tests can this
diff touch?", "what breaks if I change this?" — by grepping, or by consulting a
list somebody maintains by hand.

> **Consumer status:** this adapter layer ships **INERT**. Unlike the
> observability adapters, **no framework consumer is wired yet**. Opting in
> today gives you a uniform CLI surface for your own repo-local gates and lints;
> it does not change any Bubbles behavior. Treat that as the honest state, not
> as a soon-to-land promise.

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

Live in `bubbles/adapters/codeindex/<name>.sh`. The contract is 5 verbs:

| Verb | Args | Shape | Answers |
|------|------|-------|---------|
| `symbols` | `<query>` | array | where is this defined |
| `impact` | `<symbol>` | array | blast radius — what breaks if this changes |
| `affected` | `<file>...` | array | which tests can this diff actually reach |
| `routes` | — | array | full route/endpoint inventory |
| `status` | — | map | index freshness and health |

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

**6. Use it.** Two starting points, in order of payoff:

- **Impact-aware validation.** If your repo has no `testImpact:` map, every
  change runs the full suite. `affected` derives the mapping from the real
  dependency graph instead of asking someone to maintain it.
- **Orphan / reachability checks.** If your repo has a written "every endpoint
  must have a consumer" policy with no mechanical enforcement, `routes` plus
  `impact` turns that prose into a check.

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
2. Implement all 5 verbs plus `selftest <verb>` in a `case` statement.
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
