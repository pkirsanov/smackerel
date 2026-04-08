# Recipe: Coordinate Runtime Leases

> *"Lease the lot first, boys. Then nobody burns down the wrong trailer."*

Use this when multiple sessions might start, reuse, or tear down the same Docker or Compose stack.

## When To Use It

Use this recipe when any of these are true:
- Two sessions may share the same validation or dev stack
- One session wants to reuse a running stack without guessing whether it is compatible
- Cleanup might accidentally tear down another session's runtime
- You need to prove who owns a stack before stopping or reclaiming it

## Core Commands

```bash
bash .github/bubbles/scripts/cli.sh runtime leases
bash .github/bubbles/scripts/cli.sh runtime summary
bash .github/bubbles/scripts/cli.sh runtime doctor
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml
bash .github/bubbles/scripts/cli.sh runtime release <lease-id>
```

If you are working inside the Bubbles source repo instead of a downstream install, use the source path:

```bash
bash bubbles/scripts/cli.sh runtime leases
bash bubbles/scripts/cli.sh runtime summary
bash bubbles/scripts/cli.sh runtime doctor
```

## Common Flows

### Reuse A Compatible Validation Stack

```bash
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose validation --share-mode shared-compatible --fingerprint-file docker-compose.yml --resource container:backend
```

Use this before starting a stack that another session might already own. If the compatibility fingerprint matches, the lease is reused. If not, you get a new isolated lease instead.

### Diagnose Session Collisions

```bash
bash .github/bubbles/scripts/cli.sh runtime doctor
```

Use this before cleanup or when two sessions appear to be stepping on each other. It surfaces stale leases and active conflicts.

### Reclaim A Dead Session Safely

```bash
bash .github/bubbles/scripts/cli.sh runtime reclaim-stale
bash .github/bubbles/scripts/cli.sh runtime doctor
```

First mark expired leases stale, then re-run doctor so you know whether the runtime is actually safe to reclaim.

## Recovery Rule

Never patch `.github/bubbles` by hand in a consumer repo just because the runtime commands are missing.

Refresh the framework layer from the downstream repo root by using the upstream Bubbles checkout instead:

```bash
bash /path/to/bubbles/install.sh --local-source /path/to/bubbles --bootstrap
```

That updates the installed framework scripts and also scaffolds the runtime ignore rules under `.specify/runtime/` when needed.

Do not run that installer command inside the Bubbles source repository itself. Source-repo maintainers should work directly in the framework checkout and validate with `bash bubbles/scripts/cli.sh framework-validate` or `bash bubbles/scripts/cli.sh release-check`.

## Ask The Super Instead

If you do not want to remember the command:

```text
/bubbles.super  why are my parallel sessions colliding?
/bubbles.super  reuse the validation stack if it is compatible
/bubbles.super  reclaim stale runtime leases
```

The super should translate those directly into the correct `bubbles runtime ...` command.
