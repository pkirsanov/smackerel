---
applyTo: "**"
---

# WSL + macOS Compatibility Policy (NON-NEGOTIABLE)

> **Portability:** This file is **project-agnostic**. Copy unchanged across projects.
> It is the binding counterpart to the [`bubbles-cross-platform-shell`](../skills/bubbles-cross-platform-shell/SKILL.md)
> skill (the how-to + full pitfall table + `guard-lib.sh` helper reference).

The framework and its downstream repos are developed and operated on both WSL2 /
Linux (GNU coreutils) and macOS (BSD userland). Any command, script, selftest,
generator, or git hook authored here MUST remain operational on both platforms.

## Core Rule

**If a command works only on one platform, it is incomplete.** Do not branch on
`uname`; guard on tool *capability*, or use a form that works on both GNU and BSD
userland. Prefer the portable helpers in
[`bubbles/scripts/guard-lib.sh`](../bubbles/scripts/guard-lib.sh)
(`bubbles_sed_inplace`, `bubbles_iso_to_epoch`, `bubbles_now_ms`,
`bubbles_file_mtime_epoch`, `bubbles_run_with_timeout`) over re-deriving a
fallback in each script.

## Timeout Compatibility (Required)

`timeout` is present on Linux/WSL and commonly absent on macOS unless GNU
coreutils is installed (`gtimeout`). Resolution order: `timeout` → `gtimeout` →
explicit watchdog fallback. Never silently drop timeout protection; preserve exit
`124` on timeout.

```bash
run_with_timeout() {
  local seconds="$1"; shift
  if command -v timeout  >/dev/null 2>&1; then timeout  "$seconds" "$@"; return $?; fi
  if command -v gtimeout >/dev/null 2>&1; then gtimeout "$seconds" "$@"; return $?; fi
  "$@" &
  local cmd_pid=$!
  ( sleep "$seconds"; kill -TERM "$cmd_pid" 2>/dev/null ) &
  local watch_pid=$!
  local rc=0
  wait "$cmd_pid" 2>/dev/null || rc=$?
  kill -TERM "$watch_pid" 2>/dev/null || true
  wait "$watch_pid" 2>/dev/null || true
  [[ "$rc" -eq 143 ]] && rc=124
  return "$rc"
}
```

(`bubbles_run_with_timeout` in `guard-lib.sh` is the framework implementation.)

## Forbidden GNU-only Forms → Required Portable Forms

| ❌ FORBIDDEN (GNU-only; aborts/degrades on BSD) | ✅ REQUIRED (both platforms) |
|---|---|
| `sed -i <prog> f` / `sed -i '' <prog> f` | `bubbles_sed_inplace [-E] <prog> f` (temp-file rewrite) |
| `date -d "<ts>" +%s` | `bubbles_iso_to_epoch "<ts>"` |
| `date -d "N days ago"` | `date -d … 2>/dev/null \|\| date -v-Nd …` |
| `date +%s%N` | `bubbles_now_ms` (numeric-guard the `%N`) |
| `stat -c %Y f` | `bubbles_file_mtime_epoch f` |
| `… \| paste -sd ' '` (no operand) | `… \| paste -sd ' ' -` (explicit `-` stdin operand) |
| `awk -v x="$multiline"` | collapse newlines first, or read `ENVIRON["x"]` (BSD awk rejects a literal newline in `-v`) |
| `awk 'match($0,/re/,arr){…}'` (3-arg capture) | prefer `gawk` via a shim (`command -v gawk && awk(){ command gawk "$@"; }`) |
| `mktemp --suffix=.X` | `f=$(mktemp); mv "$f" "$f.X"; f="$f.X"` |
| `readlink -f "$p"` to make a path absolute | preserve an already-absolute path verbatim (BSD `readlink -f` canonicalizes `/var`→`/private/var`) |
| `grep -P '…'` | detect a PCRE grep (`grep`→`ggrep`) or rewrite as `grep -E` |
| `/bin/true` / `/bin/false` | bare `true` / `false` (`/bin/true` does not exist on macOS) |
| `mapfile`/`readarray` without a bash-≥4 guard | `while IFS= read -r` loop (macOS system bash is 3.2) |
| `sort` on committed/checksummed output | `LC_ALL=C sort` (stable cross-platform order) |

## Selftest Graceful Degradation (Required)

A selftest that needs an **optional** dependency (PyYAML, jsonschema, a PCRE
grep) MUST print `<name>: SKIP (<dep> not installed)` and `exit 0` when the dep is
genuinely absent — never hard-fail. Gate the *assertion*, not the whole test, so a
real regression on a fully-provisioned box still fails. This matches how the
underlying scripts already degrade (`model-tier-advisory.sh`,
`result-envelope-validate.sh` both `SKIP` + exit 0).

## Shell / Path Rules

- `#!/usr/bin/env bash` for bash scripts (never `#!/bin/bash` — macOS `/bin/bash` is 3.2).
- Guard GNU-only utilities/flags; do not assume `realpath`, `readlink -f`, `grep -P`, or GNU `date`/`sed`/`stat` flags exist.
- Unix-style paths and LF line endings only.
- Label any genuinely unavoidable platform-specific branch explicitly (`WSL/Linux` / `macOS`); iOS-only tooling paths are the only sanctioned macOS-only exception.

## Verification Checklist

1. Every new/edited command checked for WSL + macOS compatibility.
2. Raw `timeout` / `sed -i` / `date -d` / `paste` / `awk` forms replaced with the portable helper or both-ways form.
3. Verified on BSD userland (or the `framework-validate` PATH shim: `gsed`→`sed`, `gtimeout`→`timeout`). `shellcheck -x` clean is necessary but does NOT catch GNU/BSD runtime divergence.

## Mechanical Enforcement

The forbidden-forms table above is enforced mechanically by the reusable lint
[`bubbles/scripts/macos-portability-guard.sh`](../bubbles/scripts/macos-portability-guard.sh)
(see the [`bubbles-cross-platform-shell`](../skills/bubbles-cross-platform-shell/SKILL.md)
skill § *Mechanical Enforcement* for the full class list + usage). It scans a
**caller-supplied** surface — it has NO default and is never pointed at the
framework's own `bubbles/scripts/`. A genuinely intentional raw usage is exempted
inline with `# portable-ok:<reason>`; there is no other bypass.

- **Framework:** `framework-validate` runs the guard's hermetic **selftest**
  (`macos-portability-guard-selftest.sh`) alongside every other selftest — it
  does NOT scan the framework's own scripts (those use raw forms mediated by
  `guard-lib.sh` + the PATH shim).
- **Downstream repos:** this guard is **advisory-until-wired**. Each repo wires it
  into its existing pre-push / lint gate against its OWN operator script surface,
  e.g.:

  ```bash
  bash "$BUBBLES/bubbles/scripts/macos-portability-guard.sh" scripts/ *.sh || exit 1
  ```
