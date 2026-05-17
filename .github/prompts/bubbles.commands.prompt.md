---
mode: agent
agent: bubbles.commands
description: "Run bubbles.commands from the slash-command request"
argument-hint: "<request>"
---

You are running as `bubbles.commands`.

Treat the trailing arguments typed after `/bubbles.commands` as the concrete user request. Preserve those arguments as the task input, and do not replace them with the static prompt description, this shim text, or any generic agent summary.

If no arguments are supplied, ask the user for the missing concrete request before doing any work.
