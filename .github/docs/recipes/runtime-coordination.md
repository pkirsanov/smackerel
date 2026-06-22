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
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight heavy
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

### Stop Two Heavy Builds From OOMing The Host (Weighted Admission)

When several sessions share one host, two independent heavy builds (each a multi-GB
compile) can start at once and OOM-kill each other. Weighted admission gives the
host a single concurrent-work budget so the second heavy build is held or refused
instead of crashing the box.

Enable it once per host by setting a budget in `.specify/memory/bubbles.config.json`
under `runtime` (it is **disabled by default** — `capacityWeight: 0` changes
nothing):

```json
{
  "defaults": {
    "runtime": {
      "capacityWeight": 10,
      "weightClasses": { "light": 1, "medium": 4, "heavy": 8 }
    }
  }
}
```

`weightClasses` is optional; the built-in default is `{ light: 1, medium: 4, heavy: 8 }`.
The `capacityWeight` is the total weight the host will run at once.

Then weight each acquire by how heavy the work is:

```bash
# Refuse immediately if the host is already full:
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight heavy

# Or wait up to 600s for a slot to free, then refuse:
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight heavy --wait 600

# Explicit units instead of a class (overrides --weight):
bash .github/bubbles/scripts/cli.sh runtime acquire --purpose build --weight-units 6
```

The current budget shows up in `summary` and `doctor`:

```bash
bash .github/bubbles/scripts/cli.sh runtime summary
# Runtime leases: active=1 stale=0 released=0 conflicts=0
# Runtime capacity: 8/10 weight units
```

A stale or released lease does **not** count against the budget, so a dead session's
heavy lease frees its slot automatically once its TTL expires — run `reclaim-stale`
or just let `effective_status` downgrade it. Release your own lease as soon as the
build finishes so the next session can start.

### Intended Downstream Usage (documentation only)

Product repo CLIs are **not yet wired** to this surface (that is a separate later
task). The intended pattern, once a repo adopts it, is:

- Acquire a weighted `build` lease before a heavy build, and release it after:
  `runtime acquire --purpose build --weight heavy --wait <sec>` … build … `runtime release <lease-id>`.
- Acquire a short **`exclusive`** `land` lease before a push so only one session
  pushes at a time: `runtime acquire --purpose land --share-mode exclusive` … push … `runtime release <lease-id>`.

Until that wiring lands, the commands above are run manually (or by an agent) and
the budget is advisory-but-authoritative for Bubbles-controlled runtime actions.

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
