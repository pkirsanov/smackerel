# Recipe: Upgrade to Bubbles v6

> *"It ain't rocket appliances."*

---

## The Situation

You're running Bubbles v5.x in a downstream repo and want to land on v6.0. The upgrade has two halves: re-running the installer to lay down the v6 framework files, and one-shot rewriting any operator-side mentions of v5 mode names to the v6 primitive+tag form.

There is no breaking change to `state.json`, no breaking change to agent contracts, and no removed operator capability. v5 mode names keep working for the full v6 cycle; v6 just gives you better forms.

---

## The Recipe

### 1. Pre-flight: confirm your repo is clean

```bash
git status               # working tree clean, on the branch you want to upgrade
git log -1 --oneline     # capture current HEAD for rollback if needed
```

### 2. Re-run the installer

Pin to v6.0.0 or `main` once that ref ships:

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- v6.0.0
```

Or from a local Bubbles source checkout (useful while v6.0.0 is still on `main`):

```bash
bash /path/to/bubbles/install.sh --local-source /path/to/bubbles
```

The installer lays down:

- `bubbles/installer/` — new typed manifest of every install step (B9)
- `bubbles/workflows/aliases.yaml` — v5 → v6 alias map (B4)
- `bubbles/cheatsheet/` — registry for the operator cheatsheet (B7)
- `bubbles/mcp/` — MCP server + tool catalog + resource catalog (Group A)

Existing files are byte-replaced. Nothing is deleted from your downstream tree.

### 3. Verify the install with framework-validate

```bash
bash .github/bubbles/scripts/framework-validate.sh
```

Expected: `Framework validation passed.` (with downstream-only checks skipped per v5.3 `G1`).

If you get a failure, the most likely culprit is a hand-edit you made to a framework-managed file. Restore from `git checkout HEAD -- <path>` and rerun.

### 4. Migrate your operator-side surface

```bash
bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --check
```

Exit 0 means you're already on v6 forms. Exit 2 means the script will rewrite some files. Inspect the list, then:

```bash
bash .github/bubbles/scripts/migrate-modes-v5-to-v6.sh --write
```

The rewrite is idempotent. Re-running `--check` afterwards should exit 0.

The default scope excludes framework internals (`agents/`, `skills/`, `bubbles/scripts/`, `bubbles/workflows/`, the alias map itself, the cheatsheet generator outputs, historical design docs, and `CHANGELOG.md`). It picks up your `*.md` docs, `Makefile`, `install.sh`, and other operator-visible surfaces. Pass `--include-instructions` to also rewrite `instructions/` and `.github/instructions/`.

### 5. Verify the install-mode is right

```bash
bash .github/bubbles/scripts/cli.sh doctor
```

Should report `install-mode=downstream` and zero failures.

### 6. Run the new installer manifest check

This is the v6 / B9 structural verification — your `install.sh` MUST implement every declared step in `bubbles/installer/installer.yaml`. If you forked `install.sh`, this will fail until you mirror the changes into the manifest.

```bash
bash .github/bubbles/scripts/generate-installer.sh
```

Expected: `generate-installer.sh: PASS`.

### 7. Run the regenerated cheatsheets against your generators (if you forked them)

If you customized your cheatsheet, port your changes into `bubbles/cheatsheet/*.json` and re-run:

```bash
bash .github/bubbles/scripts/generate-cheatsheet.sh
```

### 8. Push

```bash
git add -A
git commit -m "chore(bubbles): upgrade to v6.0.0"
git push origin <branch>
```

Your pre-push hook (if installed) runs `framework-validate` and `release-check`. Both should pass.

---

## What Changes for the Operator

- **Cheat sheet:** `/bubbles.workflow <natural language>` still works exactly the same. The explicit-mode form uses v6 primitive+tag: `/bubbles.workflow implement action:full-delivery target:spec`. Through the v6 cycle, v5 names emitted a deprecation hint but still worked; **v7.0 removed bare v5 names as input** — run `migrate-modes-v5-to-v6.sh --write` to convert any v5 names in your operator surfaces. Existing `state.json` artifacts are unaffected.
- **Evidence pipeline:** stricter by default. If a previously-passing spec now fails `diff-evidence-guard`, either (a) add a real `git diff` to your DoD evidence block, or (b) set `state.json.modernization.diffEvidence: "advisory"` for that spec.
- **Result envelopes:** malformed envelopes now block framework-validate. Missing envelopes warn only (not yet blocking); use `result-envelope-validate.sh --strict` to opt into blocking on missing.
- **Installer manifest:** if you fork `install.sh`, mirror the change into `bubbles/installer/installer.yaml` OR your pre-push will fail the new B9 check.

---

## What Does NOT Change

- `state.json` schema (unchanged in v6 AND v7 — no schema migration)
- Agent contracts (no handoff protocol changes)
- v5 mode names as REGISTRY KEYS (still the keys; `state.json` artifacts unchanged). Only bare v5 names as *new operator input* were removed in v7.0 — use the v6 form.
- Markdown evidence path (still fully valid when diff-evidence-guard is advisory)
- Bash script surface (MCP wraps it, doesn't replace it)

---

## Rollback

If something goes wrong:

```bash
git reset --hard <pre-upgrade-SHA>
git push --force-with-lease <branch>
```

Re-install the previous Bubbles version:

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- v5.3.0
```

---

## When to Use

- One-shot upgrade from any v5.x to v6.0.
- After every Bubbles release on the v6 train (steps 2 + 3 only — steps 4+ are idempotent).
- When you discover a v5 mode name in a new operator script you wrote (run step 4 to fix it).

## Related Recipes

- [docs/recipes/setup-project.md](setup-project.md) — first-time install
- [docs/recipes/check-status.md](check-status.md) — what to ask after the upgrade
- [docs/DEPRECATIONS.md](../DEPRECATIONS.md) — full v5 → v6 deprecation log
