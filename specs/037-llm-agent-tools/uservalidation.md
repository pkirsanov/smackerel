# User Validation — 037 LLM Scenario Agent & Tool Registry

## Status

**Spec status:** `drafting` — partial implementation in progress.

User validation is **not yet applicable** for this spec because:

- Scopes 1-6 are implemented (config/SST, tool registry, scenario loader, intent router, executor loop, trace persistence + replay CLI)
- Scopes 7-10 (security hardening, operator UI, Telegram/API user surfaces, CI wiring) are **Not Started**
- The end-user-visible surfaces (Telegram bridge, `POST /v1/agent/invoke` API, admin web UI, CLI inspection commands) live in Scopes 8-9 and do not yet exist
- Without those surfaces, there is no user-visible behavior to validate

User validation will be authored when Scopes 8 and 9 land (operator UI + end-user failure surfaces). At that point this file will be replaced with concrete validation scenarios, acceptance criteria, and operator/end-user sign-off evidence.

## Implementation status

See [scopes.md](./scopes.md) for per-scope DoD checklists with executed evidence, and [report.md](./report.md) for the canonical execution report.

## Pending validation surfaces

| Surface | Owning scope | Status |
|---|---|---|
| `smackerel agent traces` CLI | Scope 8 | Not Started |
| `/admin/agent/traces` web UI | Scope 8 | Not Started |
| `smackerel agent scenarios` CLI | Scope 8 | Not Started |
| `/admin/agent/scenarios` web UI | Scope 8 | Not Started |
| `smackerel agent tools` CLI | Scope 8 | Not Started |
| `/admin/agent/tools/<name>` web UI | Scope 8 | Not Started |
| Telegram agent bridge replies | Scope 9 | Not Started |
| `POST /v1/agent/invoke` API | Scope 9 | Not Started |
| Outcome → user-message mapping | Scope 9 | Not Started |

## Replay CLI (Scope 6) — operator-facing, not end-user

The `smackerel agent replay <trace_id>` subcommand delivered in Scope 6 is an operator/diagnostic tool, not an end-user surface. Its acceptance is captured by integration and e2e tests in [scopes.md](./scopes.md#scope-6) (replay PASS / FAIL exit codes, content_hash drift detection, `--allow-content-drift` override). No separate user validation is required for it.

## Checklist

- [x] Scope 6 replay CLI integration tests pass against live test stack (`go test -tags=integration ./tests/integration/agent/...` → 13 PASS in 1.301s; `go test -tags=e2e ./tests/e2e/agent/...` → 2 PASS in 2.784s) — operator-facing diagnostic surface validated end-to-end via automated tests
- [x] Scope 7 security & concurrency hardening validated against live test stack: x-redact persistence-boundary redaction proven end-to-end (`tests/integration/agent/redact_e2e_test.go` G1-G7 incl. G7 handler-visible non-mutation), replay integrity gate reaffirmed (`tests/integration/agent/integrity_test.go` G1-G3), BS-018 200 parallel invocations / 4 scenarios pass with no cross-trace leakage (`tests/stress/agent/concurrency_test.go` p50=132ms p99=220ms over 234ms wallclock — `go test -tags=stress ./tests/stress/agent/...` → PASS in 0.921s), BS-020 forced-fixture allowlist-escape regression confirms write handler counter stays at zero and `OutcomeAllowlistViolation/not_in_allowlist` is recorded (`tests/e2e/agent/bs020_prompt_injection_test.go` G1-G5 — `go test -tags=e2e ./tests/e2e/agent/...` → PASS in 4.064s) — operator-facing security guarantees validated via automated adversarial tests; real-Ollama BS-020 deferred to a future scope when Ollama is added to docker-compose (per Scope 5 documented gap, scope test plan explicitly authorises forcing fixture)
- [ ] Operator can list traces via `smackerel agent traces` CLI (Scope 8)
- [ ] Operator can list traces via `/admin/agent/traces` web UI (Scope 8)
- [ ] Operator can drill into a single trace and see every required field (Scope 8)
- [ ] Operator can view rejected scenarios in `/admin/agent/scenarios` (Scope 8)
- [ ] Operator can view tool registry incl. side-effect class (Scope 8)
- [ ] Telegram bot replies match documented copy for every outcome class (Scope 9)
- [ ] `POST /v1/agent/invoke` returns documented JSON envelope for each outcome (Scope 9)
- [ ] Bot/API never invent answers on `unknown-intent` (BS-014, Scope 9)
- [ ] Replay CLI PASS/FAIL/ERROR exit codes verified by an operator end-to-end (Scope 6 — operator review)
- [ ] All user-facing copy reviewed for clarity and policy compliance (Scope 9)

End-user sign-off cannot be collected until Scopes 8 and 9 are implemented.
