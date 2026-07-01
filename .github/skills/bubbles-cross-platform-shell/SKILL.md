---
name: bubbles-cross-platform-shell
description: Write shell (guards, scripts, selftests, generators, hooks) that runs identically on Linux (GNU coreutils) and macOS (BSD userland). Use when authoring or editing any `*.sh` under `bubbles/scripts/`, a repo CLI, a git hook, or a selftest; when a script aborts on macOS with `usage: paste …`, `sed: -i requires an argument`, `awk: syntax error`, `awk: newline in string`, `illegal option -- d` (date), `mktemp: unrecognized option`, or `command not found: timeout`; or when a selftest hard-fails because an optional Python module (PyYAML / jsonschema) is absent. Prefer the `guard-lib.sh` helpers over hand-rolling each fallback.
---

# Cross-Platform Shell (Linux + macOS / BSD userland)

## Mental Model

The framework ships **200+ shell scripts** that agents and operators run directly.
Contributors work on both Linux (GNU coreutils) and macOS (BSD userland — `sed`,
`awk`, `date`, `paste`, `stat`, `grep`, `readlink`, `mktemp` are all the BSD
variants, and `timeout` is absent). A script that uses a GNU-only form **aborts or
silently degrades** on macOS. "Runs on my machine" is not the bar — **runs on both
GNU and BSD userland** is.

Do not detect the OS by name (`uname`). Detect the *tool behavior* or use a form
that works on both. The canonical portable primitives already live in
[`guard-lib.sh`](../../bubbles/scripts/guard-lib.sh) — prefer them over
re-deriving a fallback in every script.

## Portable Helpers (`bubbles/scripts/guard-lib.sh`)

Source guard-lib and call these instead of the raw GNU form:

| Helper | Replaces (GNU-only) | What it does |
|---|---|---|
| `bubbles_sed_inplace [-E] <prog> FILE` | `sed -i <prog> FILE` | Temp-file rewrite (GNU `sed -i` and BSD `sed -i ''` are mutually incompatible). FILE is the **last** arg. |
| `bubbles_iso_to_epoch "<ts>"` | `date -d "<ts>" +%s` | Parses ISO-UTC (`…Z`) **and** bare `YYYY-MM-DD` on GNU (`date -d`) and BSD (`date -j -f`). |
| `bubbles_now_ms` | `date +%s%N` | Millisecond clock; falls back to second-resolution when BSD `date` lacks `%N`. |
| `bubbles_file_mtime_epoch FILE` | `stat -c %Y FILE` | Pairs `stat -c %Y` (GNU) with `stat -f %m` (BSD). |
| `bubbles_run_with_timeout <secs> <cmd…>` | `timeout <secs> <cmd…>` | `timeout` → `gtimeout` → watchdog fallback; preserves exit `124` on timeout. |

A script whose selftest **copies it alone** into an isolated fixture repo cannot
source guard-lib there — in that one case, define a **local** self-contained copy
of the helper (see `artifact-lint.sh` / `done-spec-audit.sh` for the pattern).

## Pitfall → Portable Form

| ❌ GNU-only (aborts/degrades on BSD) | ✅ Portable (both) |
|---|---|
| `sed -i -E 's/…/…/' f` | `bubbles_sed_inplace -E 's/…/…/' f` (temp-file rewrite) |
| `date -d "$ts" +%s` | `bubbles_iso_to_epoch "$ts"` |
| `date -d "7 days ago" +%F` | `date -d … 2>/dev/null \|\| date -v-7d +%F` |
| `date +%s%N` | `bubbles_now_ms` (numeric-guard the `%N`) |
| `stat -c %Y f` | `bubbles_file_mtime_epoch f` |
| `… \| paste -sd ' '` (no operand) | `… \| paste -sd ' ' -` (explicit `-` reads stdin on BSD too) |
| `awk -v x="$multiline"` | collapse newlines first (`tr '\n' ','`) or read `ENVIRON["x"]` — BSD awk rejects a literal newline in `-v` |
| `awk 'match($0,/re/,arr){…}'` (3-arg) | prefer `gawk`: `command -v gawk >/dev/null && awk(){ command gawk "$@"; }` (3-arg `match` is a GNU extension) |
| `mktemp --suffix=.yaml` | `f=$(mktemp); mv "$f" "$f.yaml"; f="$f.yaml"` |
| `readlink -f "$p"` (to make absolute) | preserve an already-absolute path verbatim; BSD `readlink -f` canonicalizes symlinks (`/var`→`/private/var`) |
| `grep -P '…'` | detect a PCRE grep (`grep` → `ggrep`) or rewrite as ERE (`grep -E`) |
| `/bin/true`, `/bin/false` in a fixture | bare `true` / `false` — `/bin/true` does **not** exist on macOS (it is `/usr/bin/true`) |
| `timeout 60 cmd` | `bubbles_run_with_timeout 60 cmd` |
| `mapfile -t arr < <(…)` | fine only if you require bash ≥ 4; macOS system bash is 3.2 — guard or read in a `while read` loop |
| `sort` (locale-dependent order) | `LC_ALL=C sort` for a stable, cross-platform order (matters for checksummed/committed output) |

## Selftest Graceful Degradation

A selftest that needs an **optional** dependency (PyYAML, jsonschema, a PCRE
grep) MUST **SKIP** — print `… : SKIP (<dep> not installed)` and `exit 0` — not
hard-fail, when the dep is genuinely absent. This matches how the underlying
scripts already degrade (`model-tier-advisory.sh`, `result-envelope-validate.sh`
both print `SKIP …` and exit 0). A real regression on a fully-provisioned box
still fails: gate the *assertion*, not the whole test, so `A did not happen`
becomes `elif <dep present>; then fail else SKIP`.

## `framework-validate` PATH Shim

`framework-validate.sh` exposes the `g`-prefixed GNU tools (macOS Homebrew /
MacPorts coreutils) under their unprefixed names for the duration of the run —
`gsed`→`sed`, `gtimeout`→`timeout` — so selftests that still call GNU `sed -i` /
`timeout` directly run on macOS unchanged (a no-op on Linux). When you invoke a
single selftest by hand on macOS, replicate it:

```bash
shim="$(mktemp -d)"; ln -sf "$(command -v gsed)" "$shim/sed"; ln -sf "$(command -v gtimeout)" "$shim/timeout"
PATH="$shim:$PATH" bash bubbles/scripts/<name>-selftest.sh
rm -rf "$shim"
```

## Authoring Rules

- `#!/usr/bin/env bash` — never `#!/bin/bash` (macOS `/bin/bash` is 3.2).
- Never guard behavior on `uname`; guard on tool capability or use a both-ways form.
- Never assume `realpath`, `readlink -f`, `grep -P`, or GNU `date`/`sed`/`stat` flags exist.
- Keep committed/checksummed generator output deterministic across platforms (`LC_ALL=C sort`, no symlink canonicalization, no locale-dependent formatting).
- Verify a new/edited script on BSD userland (or the PATH shim above) before claiming it runs; `shellcheck -x` clean is necessary but not sufficient — shellcheck does not catch GNU/BSD runtime divergence.

## See Also

- Instruction: [`wsl-macos-compatibility.instructions.md`](../../instructions/wsl-macos-compatibility.instructions.md) — the binding policy (loaded `applyTo: "**"` in every repo).
- Source: [`bubbles/scripts/guard-lib.sh`](../../bubbles/scripts/guard-lib.sh) — the portable helpers.
- Skill: [`bubbles-long-running-commands`](../bubbles-long-running-commands/SKILL.md) — the timeout/background discipline these helpers support.
