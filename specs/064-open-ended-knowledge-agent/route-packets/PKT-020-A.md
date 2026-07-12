# Route Packet PKT-020-A — Open-Knowledge Egress Allowlist Review & Network-Layer Hardening

| Field              | Value |
|--------------------|-------|
| **Packet ID**      | PKT-020-A |
| **Routed from**    | `bubbles.implement` on `specs/064-open-ended-knowledge-agent` SCOPE-15 |
| **Routed to**      | `specs/020-security-hardening` (next dispatch via `bubbles.workflow`) |
| **Status**         | `pending` |
| **Date**           | 2026-05-31 |
| **Kind**           | `cross_spec_request` |
| **Blocks**         | spec 064 SCOPE-15 final close-out only for the network-layer review portion; the application-layer egress allowlist + sanitisation + API-key audit have shipped locally |
| **Does NOT block** | spec 064 SCOPE-16, SCOPE-17, SCOPE-18 |

---

## 1. Context

Spec 064 SCOPE-15 has shipped the application-layer security surface
for the open-knowledge subsystem:

- New `EgressAllowlistTransport` (`internal/assistant/openknowledge/web/egress.go`)
  is an `http.RoundTripper` that enforces a deny-by-default
  host-level allowlist on every outbound HTTP request the
  open-knowledge subsystem makes. The effective allowlist is the
  union of `assistant.open_knowledge.provider_endpoint` host (always
  implicit) and the new SST key
  `assistant.open_knowledge.allowed_egress_hosts` (defaults to `[]`).
- New `SanitizeSnippet` (`internal/assistant/openknowledge/web/sanitize.go`)
  is applied to every provider-returned snippet before the agent
  loop forwards it to the LLM. It strips ASCII control characters
  (except `\t`/`\n`), repairs invalid UTF-8, truncates to
  `MaxSnippetRunes` (2000 runes), and emits a new metric
  `openknowledge_suspicious_snippet_total{provider}` (bounded
  cardinality) when a known prompt-injection trigger pattern
  appears. Content is NOT stripped — the LLM-side prompt boundary
  (tool_output envelope from design §Security) remains the primary
  defence; this is defence-in-depth observability.
- `WebSnippet.ContentHash` is now computed over the sanitised
  snippet so the cite-back verifier (SCOPE-08) keys off the same
  canonical form the LLM actually sees.
- API-key handling: `internal/assistant/openknowledge/web/apikey_test.go`
  asserts (a) the SearxNG adapter does not log API key material via
  `slog`, (b) no API key appears in any returned error message. The
  audit confirmed that the web/ and llm/ packages perform no
  unstructured logging — the only place secrets cross a logging
  boundary is the auth-header construction in `llm/client.go`, which
  is local to a single struct-field copy and never logged.

What spec 020 owns is the **network-layer egress posture review** —
this is the first outbound HTTP path the runtime has, and the
application-layer allowlist is one of two layers we want.

## 2. Requested Reviews

### 2.1 Application-layer allowlist policy (advisory)

Please review the v1 host-allowlist policy and confirm it meets spec
020's expectations:

- Exact host match only (no wildcards in v1).
- Allowlist entries are bare hosts: no scheme, path, port, or
  userinfo. Constructor + `Validate()` reject malformed entries
  loudly (`F064-SST-INVALID`).
- Case-insensitive host comparison (`Example.COM` == `example.com`).
- Userinfo-bearing URLs (`https://user:pass@host/`) have their host
  extracted via `url.URL.Hostname()` so embedded credentials cannot
  bypass the gate.
- Non-`http`/`https` schemes (`file://`, `ftp://`, `gopher://`, …)
  are rejected outright at `RoundTrip` time so a planner-crafted URL
  cannot exfiltrate via an unexpected protocol handler.
- Deny-by-default (G021/G028): an empty allowlist denies every
  request. The provider endpoint host is always implicitly in the
  effective allowlist at wiring time so the operator does not have
  to repeat it.

Adversarial coverage (in `egress_test.go`):

- `TestEgressAllowlistTransport_DenyByDefault_Adversarial`
- `TestEgressAllowlistTransport_NormalizesMixedCaseHost`
- `TestEgressAllowlistTransport_UserinfoDoesNotBypass`
- `TestEgressAllowlistTransport_RejectsNonHTTPScheme`
- `TestNewEgressAllowlistTransport_RejectsMalformedEntries`

### 2.2 Should wildcards be added (and if so, when)?

The v1 allowlist is exact-match only. Wildcards (`*.example.com`,
suffix-match, IP-range CIDR) are intentionally out of scope for
spec 064 because the only first-party host pattern in scope is the
single `provider_endpoint`. Please confirm:

(a) is exact-match sufficient for spec 020's posture, or
(b) does spec 020 want to layer a `bubbles/skills/`-owned wildcard
    matcher on top in a follow-up?

### 2.3 Network-layer egress enforcement

The application-layer allowlist runs inside the `smackerel-core`
process. A compromised goroutine that bypassed the wired
transport (e.g., a future code path that constructed its own
`http.Client` without `WithTransport`) would defeat the gate. Please
review whether spec 020 should require an additional network-layer
egress restriction:

- Container egress firewall (Docker network policy, nftables on the
  host, or a `network_mode: bridge` ACL).
- Per-container outbound DNS allowlist (the self-hosted overlay could
  ship a CoreDNS forwarder that resolves only allowed hosts).
- Tailscale egress ACL when the self-hosted adapter binds to the
  tailnet.

Recommendation in this packet: the application-layer allowlist
should remain the primary fast-path gate (cheap, in-process,
observable via the suspicious-snippet metric); a network-layer gate
should be additive defence-in-depth, not a replacement. Spec 020 to
decide.

### 2.4 SearxNG self-hosted profile

When `assistant.open_knowledge.searxng_enabled=true` (test profile,
future self-hosted profile), the core makes outbound HTTP requests to
the in-cluster SearxNG container (`http://searxng:8080`). That
traffic is allowed by the application-layer transport (the
provider_endpoint host is implicitly in the allowlist) and never
leaves the compose network. **SearxNG itself** then issues outbound
HTTPS requests to upstream search backends (Google, DuckDuckGo,
Bing, Wikipedia, …). Those requests are NOT subject to the
core's `EgressAllowlistTransport` because they originate from a
different process.

Please review:

(a) should the deploy adapter constrain SearxNG's upstream backends
    (the `engines:` block in `searxng/settings.yml`) to an
    operator-approved subset?
(b) should SearxNG's outbound traffic be subject to the same
    container-egress policy as `smackerel-core`?

This is a configuration question for spec 020 + the deploy adapter
overlay; spec 064 does not own the SearxNG `settings.yml` content.

## 3. Suggested Acceptance Surface

Spec 020 may respond by:

- Acknowledging the application-layer allowlist is sufficient for
  v1 and recommending wildcard / network-layer follow-ups for a
  later milestone.
- Filing a route-packet back to spec 064 (or to the deploy adapter
  overlay) requesting concrete additional hardening (e.g., a
  container egress policy snippet in `deploy/compose.deploy.yml`).
- Updating its own `docs/Operations.md` with the operator-facing
  guidance on configuring `assistant.open_knowledge.allowed_egress_hosts`
  alongside the rest of the egress posture.

## 4. Cross-References

- `internal/assistant/openknowledge/web/egress.go` —
  `EgressAllowlistTransport` implementation.
- `internal/assistant/openknowledge/web/sanitize.go` —
  `SanitizeSnippet` + suspicious-pattern detector.
- `internal/assistant/openknowledge/web/searxng.go` — sanitiser
  wired into every returned snippet; `ContentHash` covers sanitised
  body.
- `internal/assistant/openknowledge/web/apikey_test.go` — API-key
  non-leakage regression guard.
- `cmd/core/wiring_assistant_openknowledge.go` —
  `buildOpenKnowledgeWebProvider` constructs the egress-wrapped
  `http.Client` and installs the suspicious-snippet recorder.
- `internal/config/openknowledge.go` — `AllowedEgressHosts` field +
  format validation.
- `config/smackerel.yaml` — new SST key
  `assistant.open_knowledge.allowed_egress_hosts: []` (deny-by-default).
- `scripts/commands/config.sh` — env var emission.
- `internal/assistant/openknowledge/metrics/metrics.go` —
  `openknowledge_suspicious_snippet_total{provider}` collector,
  `IncSuspiciousSnippet(provider)` method.
