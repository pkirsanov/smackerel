# User Validation — 082 MVP / evo-x2 Readiness Hardening

Checkbox items default to `[ ]` until audit-certified, then `[x]`. The user
unchecks an item to report a regression.

## Acceptance items

- [ ] UV-082-1 — Telegram identity is blank by default; no real chat-id literal remains.
- [ ] UV-082-2 — Over-subscribed home-lab interactive ollama set is caught fail-loud; dev/test still pass.
- [ ] UV-082-3 — Embedding-model cache persists across restart without HuggingFace reachability.
- [ ] UV-082-4 — `clean` can never wipe queued NATS capture events.
- [ ] UV-082-5 — SearxNG resource limits are SST-sourced and fail loud with a cpus cap.
- [ ] UV-082-6 — Third-party infra images are digest-pinned in lockstep.
- [ ] UV-082-7 — `promote.sh` accepts both CI and local-operator manifest shapes.
- [ ] UV-082-8 — A single operator go-live readiness checklist exists in `docs/`.
- [ ] UV-082-9 — ROCm host GIDs are routed to a fail-loud adapter env; gfx target stays generic.
- [ ] UV-082-10 — The five external dependencies are documented as tracked blockers, not faked.
