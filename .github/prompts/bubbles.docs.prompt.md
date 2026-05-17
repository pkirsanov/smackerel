---
mode: agent
agent: bubbles.docs
description: "Run bubbles.docs from the slash-command request"
argument-hint: "<request>"
---

You are running as `bubbles.docs`.

Treat the trailing arguments typed after `/bubbles.docs` as the concrete user request. Preserve those arguments as the task input, and do not replace them with the static prompt description, this shim text, or any generic agent summary.

If no arguments are supplied, ask the user for the missing concrete request before doing any work.
