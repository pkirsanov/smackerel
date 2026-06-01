# Route Packet PKT-022-A — Open-Knowledge Web-Provider Circuit Breaker & Budget-Exhaustion Degradation Review

| Field              | Value |
|--------------------|-------|
| **Packet ID**      | PKT-022-A |
| **Routed from**    | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-16 |
| **Routed to**      | `specs/022-operational-resilience` (next dispatch via `bubbles.workflow`) |
| **Status**         | `pending` |
| **Date**           | 2026-05-31 |
| **Kind**           | `cross_spec_request` |
| **Blocks**         | spec 064 SCOPE-16 final close-out only for the operational-playbook alignment portion; the application-layer circuit breaker + budget-exhaustion handling have shipped locally |
| **Does NOT block** | spec 064 SCOPE-17 (live-stack E2E), SCOPE-18 (docs + deploy adapter contract) |

---

## 1. Context

Spec 064 SCOPE-16 has shipped the application-layer resilience
surface for the open-knowledge subsystem:

- New `CircuitBreaker` (`internal/assistant/openknowledge/web/circuit.go`)
  is a concurrency-safe wrapper around any `WebSearchProvider`. It
  implements the canonical three-state machine
  (`Closed → Open → HalfOpen`) with operator-tunable bounds:
  - `FailureThreshold` consecutive countable failures trips the
    breaker from `Closed` to `Open`.
  - `HalfOpenAfter` elapses → the next call is forwarded as a
    single probe (`Closed` on success, back to `Open` on failure).
  - Failure classification (G028 — explicit):
    `ErrProviderUnreachable` + `ErrQuotaExceeded` count as failures;
    `ErrInvalidQuery`, `ErrProviderNotConfigured`,
    `ErrMalformedResponse`, `ErrInvalidConfig` do NOT (they are
    caller / config bugs, not provider outages).
  - Concurrency-safe via a single `sync.Mutex` around state
    transitions.
- New SST sub-block
  `assistant.open_knowledge.circuit_breaker.{failure_threshold,
  open_window_seconds, half_open_after_seconds}` — three required
  positive integers, validated fail-loud in `Validate()` with
  `[F064-SST-INVALID]` per G028. Defaults shipped in
  `config/smackerel.yaml` are `5 / 60s / 30s`.
- New ToolError code `provider_circuit_open` produced by
  `tools/web_search.go`'s `classifyProviderError` when the breaker
  returns `web.ErrCircuitOpen`.
- New agent termination reason `TerminationToolUnavailable` emitted
  by the loop the moment a tool returns `provider_circuit_open` —
  the LLM is NOT allowed to spin retries against a known-down
  provider; the turn terminates with `FinalText=""` so the
  Telegram surface can apply capture-as-fallback per design §UX.
- Updated mappings: `TerminationToolUnavailable` →
  `contracts.RefusalToolUnavailable` →
  `CanonicalRefusalBodyFor(RefusalToolUnavailable)` =
  `"A tool I needed isn't available right now — saved as an idea."`.
- New metrics (SCOPE-16):
  - `openknowledge_circuit_state{provider}` gauge (0=closed,
    1=half_open, 2=open).
  - `openknowledge_circuit_trips_total{provider}` counter
    (incremented on `Closed→Open` and `HalfOpen→Open`).
  - Bounded `provider` cardinality via the existing
    `allowedProviders` allow-set (G021).

Adversarial coverage (in `circuit_test.go`):

- `TestCircuit_OpenShortCircuits_AdversarialG021` — proves Open
  does NOT invoke the inner provider (fake provider queue
  underflow would fail loud).
- `TestCircuit_InvalidQueryDoesNotCount_AdversarialG021` — proves
  classification: 4 × `ErrInvalidQuery` + 1 ×
  `ErrProviderUnreachable` records exactly 1 failure, not 5.
- `TestCircuit_NonCountableErrorsLeaveStateAlone` — proves
  non-counting errors interleaved with countable failures don't
  reset the counter.
- `TestCircuit_HalfOpenProbeFailure_Reopens` — proves probe failure
  re-arms Open and emits a second trip.
- `TestCircuit_StateAccessor_ConcurrencySafe` — proves the State()
  accessor is race-free under `-race`.

Adversarial coverage (in `agent_test.go`):

- `TestAgent_CircuitOpen_TerminatesToolUnavailable` — proves the
  agent terminates with `TerminationToolUnavailable` + empty
  FinalText when a tool surfaces `provider_circuit_open`.
- `TestAgent_CircuitOpen_DoesNotLeakUnrelatedErrorCodes_AdversarialG021` —
  proves the check is narrow: a calculator divide-by-zero (`Error.Code`
  ≠ `provider_circuit_open`) stays recoverable; a regression that
  broadened the check would short-circuit normal recoverable errors
  and this test would fail loud.

What spec 022 owns is the **operational resilience playbook alignment**
— this is the first circuit breaker on a request-path subsystem in
the Smackerel runtime, and the application-layer bounds + the
budget-exhaustion + capture-as-fallback chain should be reviewed
against the operational-resilience design before the v1 thresholds
become the de-facto convention for future subsystems.

## 2. Requested Reviews

### 2.1 Circuit-breaker thresholds match the operational-resilience playbook?

Please review the SCOPE-16 v1 circuit-breaker bounds against any
existing operational-resilience conventions in the codebase or the
spec 022 design playbook:

- `FailureThreshold: 5`
- `OpenWindowSeconds: 60`
- `HalfOpenAfterSeconds: 30`

Specifically:

- Are these thresholds consistent with the connector-supervisor
  circuit pattern (`internal/connector/supervisor.go` —
  `maxPanicsBeforeDisable = 5`)?
- Should spec 022 codify a project-wide
  resilience-policy block (e.g. `runtime.resilience.default_*`) so
  future subsystems inherit a single SST source instead of every
  subsystem inventing its own knobs?
- Should the half-open probe budget be configurable (e.g. N probes
  before re-trip) in a v2, or is the v1 single-probe contract
  sufficient for the SearxNG / Brave / Tavily provider class?
- Is the `OpenWindow` documentation field worth retaining at the
  SST surface even though the effective state-transition timer is
  `HalfOpenAfter`? (We kept it as a separate required key so the
  operator must think about both numbers; spec 022 may have a
  cleaner convention.)

### 2.2 Operator-dashboard health-check endpoint contribution

The SCOPE-16 metrics
(`openknowledge_circuit_state{provider}`,
`openknowledge_circuit_trips_total{provider}`) land on the
existing `/metrics` endpoint and will be picked up by the spec 049
Grafana dashboard once PKT-049-A is accepted.

Question: does the operational-resilience subsystem need an
explicit health-check endpoint contribution from the openknowledge
subsystem? Concretely:

- Should `/health` (or a per-subsystem `/health/openknowledge`)
  surface the current breaker state for operator quick-glance
  before pulling up Grafana?
- Should a breaker-open state degrade the overall liveness signal,
  or remain advisory? (Our recommendation: advisory — the rest of
  the open-knowledge subsystem still services internal_retrieval +
  calculator + unit_convert tools even while web search is
  short-circuiting.)
- Should the deploy adapter overlay expose a CLI shortcut to query
  the current state (e.g. `smackerel.sh deploy-target home-lab
  status --circuit-breakers`)?

### 2.3 Budget-exhaustion refusal-with-capture path

The agent loop already handles three budget-exhaustion sites
(`TerminationCapIterations`, `TerminationCapTokens`,
`TerminationCapUSD`) by returning `Status=StatusRefused` with
`FinalText=""` and a typed termination reason. The Telegram
surface (spec 064 SCOPE-13) is contractually required to apply
capture-as-fallback on the original user prompt regardless of
turn outcome. This means a budget-exhausted turn does NOT lose the
user's intent — it is preserved as a Smackerel idea.

Question: does this pattern align with spec 022's documented
degradation patterns? Specifically:

- Is the "refuse-and-capture" handshake an instance of a more
  general "graceful degradation envelope" spec 022 should
  formalise across subsystems (notifications, connectors,
  recommendations, …)?
- Should the budget-exhaustion metric
  (`openknowledge_budget_exhausted_total{scope}` — landed in
  SCOPE-14) be wired into a spec 022 alert rule (e.g. "rate
  exceeds X per hour for `scope=monthly`" → operator paged)?
- Should the per-user monthly cap trigger an out-of-band
  notification to the operator (separate from the alert rule) when
  a single user repeatedly hits it?

## 3. What This Repo Has Already Shipped (Local Items)

Use this section to scope the review: spec 022 does NOT need to
touch any of these. They are listed so reviewers can audit the
local items before opining on the cross-spec contract.

- `internal/assistant/openknowledge/web/circuit.go` (~300 lines)
- `internal/assistant/openknowledge/web/circuit_test.go` (~430
  lines; 12 tests including 3 adversarial cases)
- `internal/assistant/openknowledge/metrics/metrics.go` — added
  `NameCircuitState`, `NameCircuitTrips`, `SetCircuitState`,
  `IncCircuitTrip` (bounded-cardinality `provider` label)
- `internal/assistant/openknowledge/agent/agent.go` — added
  `TerminationToolUnavailable`, `ToolErrorCodeCircuitOpen`,
  mid-loop short-circuit on `provider_circuit_open`
- `internal/assistant/openknowledge/agent/agent_test.go` — added
  `TestAgent_CircuitOpen_TerminatesToolUnavailable` + adversarial
  no-leak guard
- `internal/assistant/openknowledge/agenttool/substrate_tool.go` —
  extended `MapTerminationToRefusalCause` to cover
  `TerminationToolUnavailable → RefusalToolUnavailable`
- `internal/assistant/openknowledge/tools/web_search.go` — added
  `ErrWebSearchCircuitOpen` sentinel + `classifyProviderError`
  branch for `web.ErrCircuitOpen`
- `internal/config/openknowledge.go` — added
  `OpenKnowledgeCircuitBreakerConfig` struct + three required env
  vars + fail-loud validation
- `internal/config/openknowledge_test.go` — added happy-path,
  rejects-non-positive (6 cases), and disabled-skips-validation
  coverage
- `config/smackerel.yaml` — added `circuit_breaker:` sub-block
  under `open_knowledge:`
- `scripts/commands/config.sh` — emits the three new env keys with
  `required_value`
- `cmd/core/wiring_assistant_openknowledge.go` — wraps the built
  WebSearchProvider in `web.NewCircuitBreaker(...)` before passing
  to `tools.RegisterAll`

## 4. Expected Response Packet Shape

Please return a response packet with:

- **Acceptance verdict** for the v1 thresholds (or a concrete
  proposed delta).
- **Decision** on the health-check endpoint contribution (yes /
  no / "advisory, defer to vN").
- **Decision** on whether the refuse-and-capture handshake should
  be lifted into spec 022 as a cross-subsystem degradation
  pattern.
- **Concrete additive requirements** (if any) — e.g. new SST
  keys, alert rules, dashboard panels — that spec 064 should
  honour before SCOPE-16 closes.

Until that response lands, spec 064 SCOPE-16 status is
`Awaiting Cross-Spec Resolution`; the local items are complete and
proven by the unit suite + lint pass.
