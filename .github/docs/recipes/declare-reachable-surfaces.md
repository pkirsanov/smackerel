# Recipe: Declare Reachable Surfaces (Give Reachability Checks A Denominator)

**When to use:** You want to know whether the capability your repo just built
can actually be reached by anybody — and you want that answer derived from
source, not asserted from a list somebody maintains by hand.

> **Consumer status:** one framework consumer is wired —
> [`surface-reachability-guard.sh`](../../bubbles/scripts/surface-reachability-guard.sh)
> — and it is deliberately **REPORT-ONLY**. It reconciles the surfaces your repo
> derives against the exposure your specs declare, prints both orphan
> directions, and exits 0 either way. It never blocks a transition. Registering
> a blocking gate on top of it is a separate, conditional decision that has not
> been taken. Treat that as the honest state, not as a soon-to-land promise.

---

## Why this exists

A capability can be complete, tested, reviewed and merged, and still be
unreachable. Nothing in a normal test suite objects: the unit tests exercise the
function directly, the integration tests exercise the service directly, and no
check ever asks the question a user asks — *how do I get to it?*

The framework audited itself and found five scripts in that state: complete,
selftested, documented, and invoked by nothing. Two were wired into the
transition guard, two were exposed as CLI subcommands, one was retired. None of
them were broken. They were unreachable, which is a different defect, and no
existing check was shaped to see it.

The reason no check was shaped to see it is a missing denominator. Asking "is
this reachable?" requires knowing the full set of ways a caller can reach
anything in this repo — and the framework cannot know that. It does not know how
your repo routes HTTP, registers CLI subcommands, or mounts screens. Guessing
would produce a denominator nobody could trust, and a green check over an
untrustworthy denominator is worse than no check at all.

So the repo supplies it.

---

## The two halves

| Half | Who writes it | What it answers |
|---|---|---|
| `surfaces:` in your project config | the repo | "what classes of surface exist here, and what command derives each one from source?" |
| `## Exposure Contract` in `spec.md` | the feature | "for this capability, which surface reaches it, and is that surface delivered, planned, or internal?" |

The guard reconciles the two and reports both orphan directions:

- **Orphaned surface** — the derivation found it in source; no spec declares it.
  Something is reachable that nobody wrote down.
- **Undelivered claim** — a spec declares it `delivered`; the derivation cannot
  find it. Something is claimed reachable that is not.

Neither direction is inferred. Both come from a command your repo owns.

---

## Step 1 — Declare your surface classes

Add a `surfaces:` block to `.github/bubbles-project.yaml`. It is optional and
opt-in; absent means the guard reports "not applicable" and exits 0.

```yaml
surfaces:
  schemaVersion: 1
  classes:
    httpRoute:  { derive: "scripts/inventory/http-routes.sh" }
    cliCommand: { derive: "scripts/inventory/cli-commands.sh" }
    uiRoute:    { derive: "scripts/inventory/ui-routes.sh" }
```

Each `derive` command prints one TAB-separated record per reachable surface:

```
<class>	<id>	<path>	<sourceFile>
```

Blank lines and `#` comments are ignored. A record labelled with a class other
than the one it was derived for is rejected, because a derivation that mislabels
its own output cannot be reconciled against anything.

---

## Step 2 — Make the derivation fail loud

This is the rule that keeps the whole contract honest. A derivation command
that errors, is missing, or returns zero records is a **hard failure** — exit 1,
with the class and the command named. It is never a pass.

The failure being guarded against is specific and has happened: a coverage
audit in a product repo reported `100% coverage`, exit 0, over a scan scope of
two directories, while 35 routes lived in four files outside it. The audit was
not wrong about what it measured. It was wrong that "all" applied. An empty
denominator must never look like success.

Everything downstream of derivation — the reconciliation verdict itself — is
report-only. Input integrity is strict; the verdict is advisory. The two are
different jobs and they get different exit codes.

---

## Step 3 — Declare exposure in the spec

Every feature spec carries an `## Exposure Contract` table:

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| Restore drill runner | `cliCommand` | `upkeep restore-drill` | `delivered` | — |
| Drill history view | `uiRoute` | `/ops/drills` | `planned` | `specs/061-ops-console` |

`delivered` must correspond to a record the derivation actually produces.
`planned` must name a target spec or phase — "Later" is not a plan. `internal`
must name its in-repo caller. A capability with neither `delivered` nor
`planned` is the orphan condition, stated by the spec itself.

---

## Step 4 — Run it

```bash
bash bubbles/scripts/surface-reachability-guard.sh --repo-root .
```

In a downstream install:

```bash
bash .github/bubbles/scripts/surface-reachability-guard.sh --repo-root .
```

There is no `--skip`, no `--force`, and no `--ignore`. There is nothing to
suppress, because the guard does not block.

---

## Starting classes per repo

Open-classed on purpose, so it fits a repo with no HTTP runtime as well as one
with three front ends. Each repo owns and refines its own set; these are
starting points, not a schema.

| Repo | Surface classes | Derivation source |
|---|---|---|
| **bubbles** (framework source, no runtime) | `cliCommand`, `guardScript`, `gate`, `agent`, `promptShim`, `skill`, `mcpTool` | `cli.sh` dispatch table, `bubbles/scripts/`, `gates.yaml`, `agents/`, `prompts/`, `skills/`, `bubbles/mcp/tools/` |
| **knb** (adapter overlay, no CLI) | `adapterVerb`, `homeLabService`, `lintScript` | `<product>/<target>/{apply,verify,rollback,bootstrap,preconditions,teardown}.sh`; `shared/<svc>/{install,verify}.sh`; `scripts/lint/` |
| **QuantitativeFinance** | `httpRoute`, `uiRoute`, `cliCommand` | `scripts/audit-openapi-coverage.sh`, already hardened for builder-wired routes |
| **WanderAide** | `httpRoute`, `uiRoute`, `mobileScreen`, `cliCommand` | gateway routes, admin portal routes, Flutter screens |
| **GuestHost** | `httpRoute`, `uiRoute`, `rokuScreen`, `cliCommand` | backend routes, dashboard routes, **both** Roku modes — Draw2D and SceneGraph, where the parity constraint makes a missing surface a P0 |
| **smackerel** | `httpRoute`, `cliCommand`, `mcpTool`, `telegramCommand` | `./smackerel.sh` surface, MCP knowledge-server tools, connector commands |
| **research-lab** (build-free) | `htmlTool` | the `index.html` `TOOLS` array plus `tools.json` — a tool file on disk absent from both registries is the orphan condition |

**Rollout order.** QuantitativeFinance first: it already owns the extractor and
has a known true positive to regress against. Then knb, whose adapter-verb set
is small and fully enumerable, which makes it a cheap high-confidence check.
Then the rest.

**The onboarding bar.** No repo is onboarded until it can name a derivation
command that satisfies Step 2. A class declared without a working derivation is
a hard failure by design, so onboarding early with a stub is not an option and
was never meant to be.

The framework source tree itself resolves `EXEMPT`: it has no product runtime,
and its own reachability audit is the SCOPE-8 disposition recorded in
`CHANGELOG.md` rather than a live `surfaces:` block.

---

## Related

- [Use A Code Index](use-a-code-index.md) — `derive: codeIndex` routes a class
  through the configured code-index adapter instead of a hand-written script.
- [`project-config-contract.md`](../../agents/bubbles_shared/project-config-contract.md)
  — the full `surfaces:` field reference.
- [`feature-templates.md`](../../agents/bubbles_shared/feature-templates.md) —
  the `## Exposure Contract` table definition.
