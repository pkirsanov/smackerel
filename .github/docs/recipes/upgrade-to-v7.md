# Recipe: Upgrade to Bubbles v7

> *"You gotta let the old decals go, Ricky. The trailer rolls the same."*

---

## The Situation

You're on Bubbles v6.x and want to land on v7.0. v7 completes the v6 mode
collapse: **bare v5 mode names are removed as operator input.** That is the only
intentional breaking change — and it does not touch any work you've already
done.

- There is **no `state.json` schema change.**
- There is **no per-spec migration.** Every already-complete spec, scope, bug,
  and ops artifact keeps validating exactly as it did on v6.
- The v5 names remain the canonical **registry keys**. Your `state.json`
  artifacts already store those keys, and the guards resolve their status
  ceilings by direct registry lookup — unchanged.

The single thing that changes: if you *type* a bare v5 mode name to start new
work, `mode-resolver.sh` now rejects it (exit 3) and prints the v6
primitive+tag form to use instead.

---

## The Recipe

### 1. Pre-flight: confirm your repo is clean

```bash
git status               # working tree clean
git log -1 --oneline     # capture current HEAD for rollback if needed
```

### 2. Re-run the installer

From a local Bubbles source checkout:

```bash
bash /path/to/bubbles/install.sh --local-source /path/to/bubbles
```

Or pin to the tag once published:

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- v7.0.0
```

Existing framework files are byte-replaced. Nothing in your application tree,
specs, or `state.json` files is touched.

### 3. Verify the install with framework-validate

```bash
bash .github/bubbles/scripts/framework-validate.sh
```

Expected: `Framework validation passed.` (with the source-only checks skipped
under `install-mode=downstream`). v7.0 also fixed a v6.x regression where six
maintainer-only selftests FAILed in downstream installs — they now skip
cleanly.

### 4. Migrate any v5 names in your operator surfaces

```bash
bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --check
```

Exit 0 means you're already on v6 forms. Exit 2 means there are v5 names in
your docs / Makefile / CLI / prompts to rewrite:

```bash
bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --write
```

The rewrite is idempotent. After it, typing the v6 form works and the old v5
names are gone from your operator surface.

### 5. (Rarely needed) Resolve a stored mode programmatically

If you have tooling that resolves a *persisted* mode value through
`mode-resolver.sh`, pass `--grandfather` (or set `BUBBLES_MODE_GRANDFATHER=1`)
so the stored v5-key resolves with a deprecation notice instead of being
rejected. The built-in guards already do this automatically.

```bash
BUBBLES_MODE_GRANDFATHER=1 bash .github/bubbles/scripts/mode-resolver.sh <stored-mode>
```

---

## What Changes for the Operator

- **New work:** start it with the v6 primitive+tag form (`fix target:bug
  action:fastlane`), not the v5 name (`bugfix-fastlane`). Typing the v5 name
  is rejected with the exact v6 form to use.
- **Existing work:** nothing. No re-validation, no migration, no schema change.
- **Parallel dispatch:** the `BUBBLES_PARALLEL_PHASES` opt-out flag was removed.
  Parallel fan-out for parallel-eligible phases is now mandatory (determinism
  guarantees still enforced by `parallel-fanout.sh`).

---

## What Does NOT Change

- `state.json` schema (unchanged in v6 AND v7)
- Agent contracts (no handoff protocol changes)
- Registry mode keys (the v5 names are still the keys)
- Gates (still 102), phases, and the MCP surface
- Any already-complete spec/scope/bug/ops artifact

---

## Rollback

```bash
git checkout <pre-upgrade-HEAD> -- .github/
```

Re-running the v6 installer restores the v6 framework files. Your application
tree and specs were never modified by the upgrade.
