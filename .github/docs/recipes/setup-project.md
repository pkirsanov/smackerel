# Recipe: Set Up a New Project

> *"Smokes, let's go."*

First time using Bubbles? Here's how to go from zero to a working project.

## Step 1: Install Bubbles

This recipe is for a downstream project repo, not the Bubbles source repository itself. If you are maintaining the framework, work directly in the source checkout and use `bash bubbles/scripts/cli.sh ...` validation surfaces instead of rerunning `install.sh` there.

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- --bootstrap --profile foundation
```

This installs agents, scripts, prompts, and scaffolds your project config.

If you want the current installer default instead of the lighter first-run path, omit `--profile foundation`. The default remains `delivery` until later canaries justify any change.

## Step 2: Check Health

First verify the framework setup itself:

```
/bubbles.setup  mode: refresh
```

Then check overall project health:

```
/bubbles.super  check my project health
```

Or:
```bash
bash .github/bubbles/scripts/cli.sh doctor
bash .github/bubbles/scripts/cli.sh repo-readiness .
```

Fix any issues:
```
/bubbles.super  doctor --heal
```

If the repo already contains Claude Code, Roo Code, Cursor, or Cline rules, review the migration surfaces before you start authoring new Bubbles behavior:

1. Open the [Interop Migration Guide](../guides/INTEROP_MIGRATION.md).
2. Open the generated [Interop Migration Matrix](../generated/interop-migration-matrix.md).
3. Use review-only interop intake first, then supported apply only for the explicit project-owned targets recorded in the import manifest.

## Step 3: Fill In Your Config

The bootstrap created files with `TODO` markers. Update them:

1. **`.specify/memory/agents.md`** — Add your CLI commands (build, test, lint, etc.)
2. **`.github/copilot-instructions.md`** — Add project policies, testing requirements
3. **`.specify/memory/constitution.md`** — Add project principles

## Step 4: Generate Command Registry

```
/bubbles.commands
```

This auto-detects your project and fills in the command registry.

If you change framework files or install additional Bubbles assets later, rerun:

```
/bubbles.setup  mode: refresh
```

## Step 5: Install Git Hooks

```
/bubbles.super  install hooks
```

## Step 6: Start Your First Feature

```
/bubbles.analyst  <describe your feature>
```

Then follow the [New Feature recipe](new-feature.md).
