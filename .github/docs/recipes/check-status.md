# Recipe: Check Status

> *"Decent!"*

---

## The Situation

You want to know where things stand.

## The Command

```
/bubbles.status
```

Shows:
- Active specs and their status
- Scope completion progress
- Current phase
- Any blocked items

## For a Specific Feature

```
/bubbles.status  show status of 042-catalog-assistant
```

## If You Want The Narrative Version

Use `bubbles.recap` when you want the quick human summary instead of the structured state report.

```
/bubbles.recap
```

## What Status Reports

- **Spec status:** not_started / in_progress / done / blocked
- **Scope progress:** N of M scopes Done
- **DoD completion:** N of M items checked
- **Current phase:** What's running now
- **Blockers:** Any gate failures or issues

## Profile-Aware Health Checks

When you want to inspect the framework layer and your onboarding posture together, run:

```bash
bash .github/bubbles/scripts/cli.sh doctor
bash .github/bubbles/scripts/cli.sh repo-readiness .
```

`doctor` now shows the active adoption profile separately from framework integrity and keeps foundation-profile onboarding gaps advisory. `repo-readiness` remains advisory for every profile and does not replace `bubbles.validate` certification.

For first-time adoption, the docs-recommended path is a foundation bootstrap:

```bash
curl -fsSL https://raw.githubusercontent.com/pkirsanov/bubbles/main/install.sh | bash -s -- --bootstrap --profile foundation
```

If you omit `--profile foundation`, the installer still defaults to `delivery`.
