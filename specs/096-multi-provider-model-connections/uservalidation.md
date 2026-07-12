# User Validation — Spec 096 Multi-Provider AI Model Connections

> Items default to checked `[x]` (validated by the delivering agent via
> automated unit + live-stack integration evidence — see report.md). The
> operator unchecks an item to report it is not working as expected. Items
> left unchecked `[ ]` require a real provider API key + the self-hosted live
> dispatch and are pending operator validation (postponed: only an Anthropic
> key is currently available).

## Checklist

### Operator configuration (web UI, operator-global)
- [x] The operator can see the declared provider slots (ollama enabled; anthropic, openai, azure-foundry, google, bedrock shipped disabled) in the web UI.
- [x] The operator can enter a provider credential through the web UI; the secret is write-only — it is encrypted into the AES-256-GCM vault and never returned or logged (redacted last-4 only).
- [x] The operator can test a connection before enabling it; a failed test is reported truthfully and is never reported as success.
- [x] A connection cannot be enabled until it has a stored credential and a passing test (the effective-enabled single gate).
- [x] The admin surface is operator-gated: with no operator id configured it runs fail-closed (every request 401/403).

### Model selection + dispatch (Telegram + web)
- [x] Models from enabled connections appear in the picker, provider-qualified (`<kind>/<backend-id>`); ollama models remain available and free.
- [x] The existing 088/089 Telegram `/model` picker and web model picker continue to work unchanged (additive parity; allowed_models byte-for-byte preserved).
- [x] A misconfigured or disabled connection never silently falls back to Ollama — dispatch fails loud.

### Cost + budget (trust)
- [x] Ollama-qualified models cost $0; paid models are priced from `llm.model_costs`, and a paid model with no rate refuses fail-loud (never a silent $0).
- [x] An over-budget paid request is refused BEFORE the provider is called (global + per-user month-to-date caps).
- [x] Usage attribution is provider-qualified in the model_usage_ledger.

### Boot / operability (post-implementation fix)
- [x] A fresh / dev / test stack boots with all hosted slots disabled and no `LLM_PROVIDER_SECRET_MASTER_KEY` set (proven: integration `PASS: go-integration`, core Healthy). The master key becomes mandatory only once a db-mode slot is enabled.

### Live self-hosted proof (PENDING — operator postponed; needs real provider keys)
- [ ] Live: the operator enters a real Anthropic API key via the web UI, tests it OK, enables `anthropic`, picks the model in Telegram `/model`, and `/ask` returns an answer served by Anthropic. (Postponed by operator; only the Anthropic key is available.)
- [ ] Live: enabling/disabling a connection is reflected in the model catalog membership against a running core (`TestEnableDisable_CatalogMembershipFollows_Spec096`, deferred live leg).
- [ ] Live: an over-budget paid `/ask` is refused before the provider call against a running core (`TestAsk_PaidModelExhaustedBudget_RefusedBeforeProviderCall_Spec096`, deferred live leg).
