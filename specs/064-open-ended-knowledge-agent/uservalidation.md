# User Validation — Spec 064 (Open-Ended Knowledge Agent)

> Items default to checked `[x]` once validated. Owner unchecks `[ ]`
> to report regressions. Pending items use `[ ]` until the owner
> ratifies them.

## Ratification Status

**Status: Pending full delivery.** The owner has approved the spec goal
verbatim ("we must support open-ended knowledge, we must not add each
new scenario, llm agent must be used and search web") and has accepted
the two principle deviations recorded in
`state.json.policySnapshot.principleDeviations`:

- `outbound-web-egress-from-runtime`
- `third-party-saas-dependency-in-request-path-mitigated-by-searxng`

Implementation scopes 02-08 are complete pending validation; scopes 09-18
have not started. This checklist will be flipped to `[x]` by the owner
after `bubbles.validate` / `bubbles.audit` / `bubbles.chaos` complete on
the full scope chain.

## Checklist

- [x] Owner approves the open-ended knowledge goal verbatim (a single
  LLM agent that can search the web rather than a per-scenario skill).
- [x] Owner ratifies the `outbound-web-egress-from-runtime` deviation,
  mitigated by SST egress allowlisting (SCOPE-15).
- [x] Owner ratifies the
  `third-party-saas-dependency-in-request-path-mitigated-by-searxng`
  deviation, mitigated by self-hosted SearxNG (SCOPE-07).
- [ ] Outcome Contract acknowledged: the agent must return cite-backed
  answers or a typed refusal — never a fabricated source.
- [ ] Capture-as-fallback remains inviolable: an open-ended question that
  the agent declines still becomes an `idea` artifact.
- [ ] Budgets behave as advertised end-to-end (iteration cap, per-query
  token cap, per-query USD cap, monthly cap, per-user monthly cap).
- [ ] Web egress is restricted to the configured `provider_endpoint`;
  no requests escape the allowlist (verifiable via SCOPE-15 adversarial
  tests).
- [ ] Telegram surface renders citations and refusals exactly per the
  UX packet (SCOPE-13).
- [ ] Observability dashboards expose per-tool / per-refusal metrics
  without leaking API keys (SCOPE-14).
- [ ] End-to-end live-stack scenarios SCN-064-A01..A08 pass, including
  the adversarial fabricated-citation regression (SCOPE-17).

## Sign-off

> Owner fills this section after the full delivery chain completes.

- Signed off by: _pending_
- Date: _pending_
- Notes: _pending_
