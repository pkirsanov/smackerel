# Recipe: Custom Quality Gates

> *"I AM the liquor, Randy. And now there's more liquor."*

Add project-specific quality checks that run alongside Bubbles' built-in gate registry.

## Add a Gate via Agent

```
/bubbles.super  add a pre-push gate that checks license compliance using scripts/license-check.sh
```

The agent creates the gate script and registers it in `.github/bubbles-project.yaml`.

## Add a Gate via CLI

```bash
# Create your gate script
# (must exit 0 for pass, non-zero for fail)

# Register it
bash .github/bubbles/scripts/cli.sh project gates add license-compliance \
  --script scripts/license-check.sh \
  --blocking \
  --description "Verify all dependencies have approved licenses"
```

## Test a Gate

```bash
bash .github/bubbles/scripts/cli.sh project gates test license-compliance
```

## How It Works

Custom gates are defined in `.github/bubbles-project.yaml`:
```yaml
gates:
  license-compliance:
    script: scripts/license-check.sh
    blocking: true
    description: Verify all dependencies have approved licenses
  a11y-audit:
    script: scripts/a11y-check.sh
    blocking: false
    description: Accessibility audit (warning only)
```

The state transition guard automatically discovers and runs these gates. Blocking gates prevent spec completion. Non-blocking gates produce warnings.

## Gate IDs

- Framework (built-in) gates: **G001–G199 reserved**. Active IDs are listed in `bubbles/registry/gates.yaml` (the framework currently uses G001–G095 plus G110–G125).
- Project-local custom gates: **G900+** (auto-assigned). The custom band is kept clear of the framework-reserved range so Bubbles upgrades can never collide with your gate IDs. The G200–G899 span is an intentional reserved gap.
- Custom gates survive Bubbles upgrades — `bubbles-project.yaml` is never overwritten

## List Your Gates

```bash
bash .github/bubbles/scripts/cli.sh project gates
```
