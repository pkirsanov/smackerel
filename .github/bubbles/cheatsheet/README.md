# Cheatsheet Registry (v6.0 / B7)

Single source of truth for the operator cheatsheet. `bubbles/scripts/generate-cheatsheet.sh` reads these JSON files and emits the regenerable blocks of both `docs/CHEATSHEET.md` (markdown) and `docs/its-not-rocket-appliances.html` (visual). Because both outputs come from one input, drift is structurally impossible — the v5.0.1 H7 drift check is retired.

## Files

- `modes.json` — workflow modes. One entry per `(mode, alias)` tuple so the same mode can have multiple aliases. The MD modes table and the HTML "Workflow Mode Aliases" cards both come from this.
- `aliases.json` — sunnyvale command aliases (alias → agent/command). Drives the MD "Command Aliases" table and the HTML "Sunnyvale Command Aliases" table.
- `vocabulary.json` — TPB vocabulary terms (term + meaning). Drives the MD "TPB Vocabulary" table and the HTML "TPB Vocabulary" cards. Backticks in `meaning` are translated to `<code>` for the HTML rendering.

## Schema

### `modes.json`

```json
[
  {
    "name": "value-first-e2e-batch",
    "alias": "boys-plan",
    "description": "Auto-discover highest-value work, full delivery pipeline.",
    "description_html": "Auto-discover highest-value work, full delivery pipeline. The master plan.",
    "html_quote": "Boys, I got a plan. And this time it's a good one."
  }
]
```

| Field | Required | Used by |
|---|---|---|
| `name` | yes | MD + HTML — the workflow mode name (must exist in `bubbles/workflows.yaml`, with explicit exception list documented inline in the generator) |
| `alias` | yes | MD + HTML — the sunnyvale alias |
| `description` | yes | MD — short description for the table row |
| `description_html` | no | HTML — when present, overrides `description` in the HTML card (lets HTML keep slightly longer prose) |
| `html_quote` | no | HTML only — flavor quote for the card |

### `aliases.json`

```json
[
  {
    "alias": "pull-the-strings",
    "maps_to": "bubbles.workflow",
    "quote": "Bubbles is pulling the strings, boys."
  }
]
```

| Field | Required |
|---|---|
| `alias` | yes — the part after `sunnyvale ` |
| `maps_to` | yes — agent name or compound command (e.g. `bubbles.implement + bubbles.test`) |
| `quote` | yes — Rickyism shown in the right column |

### `vocabulary.json`

```json
[
  {
    "term": "scenario replay",
    "meaning": "Validate reruns the linked live-system `SCN-*` user journeys from `scenario-manifest.json` before certification."
  }
]
```

| Field | Required |
|---|---|
| `term` | yes — the vocabulary term |
| `meaning` | yes — markdown prose; backticks become `<code>` in the HTML output |

## Generator

```bash
bash bubbles/scripts/generate-cheatsheet.sh          # update both files in place
bash bubbles/scripts/generate-cheatsheet.sh --check  # verify both files are current; non-zero on drift
```

Run after editing any registry file. `--check` is wired into `framework-validate.sh` and `release-check.sh` so stale cheatsheets block release.

## Adding a workflow mode

1. Add an entry to `bubbles/workflows.yaml` `modes:` block (Sonny/DVS-owned for release+train modes; otherwise a normal spec).
2. Add a `(mode, alias)` entry to `modes.json`.
3. Run `bash bubbles/scripts/generate-cheatsheet.sh`.
4. Commit registry + regenerated cheatsheets together.

## Adding a TPB vocabulary term

1. Add an entry to `vocabulary.json`.
2. Run the generator.
3. Commit.
