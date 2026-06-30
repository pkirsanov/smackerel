#!/usr/bin/env bash
# BUG-099-002 — executable surface for smackerel_run_with_timeout.
#
# Some call sites cannot invoke the shell function directly: the e2e lane wraps
# its child command in `env`/`setsid` (which exec a *binary*, not a bash
# function), and several standalone test scripts run as their own process. They
# call this shim instead. It is a drop-in for GNU
# `timeout [--kill-after=<grace>] <seconds> <cmd...>` that resolves
# `timeout` -> `gtimeout` -> a watchdog fallback via the shared runtime lib
# (exit-124-on-timeout preserved), per
# .github/instructions/wsl-macos-compatibility.instructions.md.
set -euo pipefail

RWT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/runtime.sh
source "$RWT_LIB_DIR/runtime.sh"

smackerel_run_with_timeout "$@"
