---
name: bubbles-long-running-commands
description: How to run long commands (builds, tests, deploys, profiling, large validations) without burning turns on polling — background/async execution, end-the-turn-and-await-notification, and an optional signal-file heartbeat for cheap mid-flight status peeks. Use when a command will run for minutes (Docker build, full test suite, pre-push, framework-validate, deploy), when you catch yourself re-reading streaming build output, or when a wrapper keeps getting killed by a short timeout.
---

# Long-Running Commands

## Mental Model

A long command (minutes, not seconds) must not be babysat. Polling its terminal
repeatedly — re-reading truncated Docker/cargo/test output — wastes turns and
tells you almost nothing. Two disciplines replace polling:

1. **Run it in the background and end your turn.** Modern agent runtimes notify
   you automatically when a backgrounded command finishes (success, failure, or
   needs-input). Ending the turn and awaiting that notification is the default.
2. **For a cheap mid-flight peek, read a signal file — not the terminal.** A tiny
   wrapper writes one-line progress to a status file; a single read of that file
   is instant and meaningful, where re-reading the terminal is noisy and stale.

## Decision Tree

```
Is the command going to run for more than ~1 minute?
├── No  → run it synchronously, read the result, continue.
└── Yes → run it in the BACKGROUND (async mode).
          ├── You have nothing to do until it finishes
          │     → END YOUR TURN. Await the completion notification. Do NOT poll.
          └── You want an occasional "is it still healthy?" peek
                → wrap it with a signal-file heartbeat; read the FILE (one line),
                  never the streaming terminal. Still end the turn between peeks.
```

## Rules

- **Never poll a build/test terminal in a loop.** Re-reading the same streaming
  output 5–10 times for identical noise is the anti-pattern this skill exists to
  kill. Either await the completion notification or read the signal file once.
- **Give long commands a generous timeout (or none).** A short timeout that fires
  mid-compilation kills real work and produces misleading "stall" output. Size the
  timeout to the job (large installs/builds/validations can be 20–40 min), or run
  unbounded in the background and await notification.
- **A quiet long command is usually still working, not stalled.** Confirm with one
  signal-file read or one status check before concluding failure. Validation hooks
  and big compiles legitimately produce no output for minutes.
- **Resolve the actual command through repository-standard entry points** (the
  repo CLI / `.specify/memory/agents.md`), never an ad-hoc bypass.
- **Route execution through repository policy.** This skill governs HOW to wait,
  not WHICH command to run — keep it product-agnostic.

## Optional: Signal-File Heartbeat

When you genuinely need periodic mid-flight status without parsing streaming
output, wrap the command so it records one-line progress to a status file. Read
the FILE for status; keep the FULL output in a separate log for post-mortem only.

A minimal, product-agnostic wrapper shape (adapt paths to the repo's runtime
conventions; do not hardcode hosts/ports):

```bash
#!/usr/bin/env bash
set -uo pipefail
cmd="$1"
status_file="${HEARTBEAT_STATUS_FILE:?set HEARTBEAT_STATUS_FILE}"
log_file="${HEARTBEAT_LOG_FILE:?set HEARTBEAT_LOG_FILE}"
start=$(date +%s)

printf 'STATUS=RUNNING\nCOMMAND=%s\nSTARTED=%s\n' "$cmd" "$(date -Iseconds)" >"$status_file"
eval "$cmd" >"$log_file" 2>&1 &
pid=$!
printf 'PID=%s\n' "$pid" >>"$status_file"

while kill -0 "$pid" 2>/dev/null; do
  elapsed=$(( $(date +%s) - start ))
  last=$(tail -n 1 "$log_file" 2>/dev/null | head -c 200)
  { printf 'STATUS=RUNNING\nCOMMAND=%s\nPID=%s\nELAPSED=%ss\nLAST_LINE=%s\n' \
      "$cmd" "$pid" "$elapsed" "$last"; } >"$status_file"
  sleep 30
done

wait "$pid"; rc=$?
elapsed=$(( $(date +%s) - start ))
{ [[ $rc -eq 0 ]] && printf 'STATUS=DONE\n' || printf 'STATUS=FAILED\n'
  printf 'COMMAND=%s\nEXIT_CODE=%s\nELAPSED=%ss\nFINISHED=%s\n' \
    "$cmd" "$rc" "$elapsed" "$(date -Iseconds)"; } >"$status_file"
```

Status file fields: `STATUS=RUNNING|DONE|FAILED`, `COMMAND`, `PID`, `ELAPSED`,
`LAST_LINE`, and on completion `EXIT_CODE`. Checking status = one read of the
status file. On `DONE`+`EXIT_CODE=0` proceed; on `FAILED` open the log's tail and
diagnose.

## Anti-Patterns (FORBIDDEN)

- Re-reading a build/test/deploy terminal in a polling loop for the same output.
- Claiming "I'll be notified" for a SYNCHRONOUS command that is mid-run — the
  notification fires for BACKGROUND commands; a sync command must either finish or
  be backgrounded first.
- Wrapping a long build in a short timeout that kills it mid-compilation.
- Concluding "stalled / network-blocked" from a quiet long command without one
  signal-file read or status check (a quiet validation hook or compile is normal).
- Parsing streaming Docker/cargo/test output to infer completion instead of
  checking the exit notification or the signal file.

## Relationship To Terminal Discipline

This skill is the "long command" companion to the repo's terminal-discipline
rules: it never redirects command output into tracked files (the signal/log files
are scratch state), never truncates a command's real result, and always routes the
underlying command through the repository-standard runtime entry point.
