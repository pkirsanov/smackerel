# Per-Transport Configuration Surface

This document is the operator-facing mirror of the
`internal/assistant/transportconfig` registry. It enumerates every
configuration key consumed by the assistant's three transports — HTTP,
WhatsApp, and the legacy/assistant Telegram surfaces — together with
the generator-emitted env var, whether the key is required at the
generator boundary, and the Go package that consumes it at startup.

**Source of truth:** the registry in
[`internal/assistant/transportconfig/`](../internal/assistant/transportconfig/).
This doc is enforced to stay in lock-step with the registry by
`TestRegistry_DocSync` (SCN-062-A06) under
`./smackerel.sh test unit`. Any change to a registry entry (add, rename,
remove, or required-flag flip) MUST update the matching row below in
the same commit, or the doc-sync test fails the build.

**No-defaults policy:** every entry marked `Required: yes` aborts
`smackerel-core` startup if the corresponding env var is unset or
empty, with the registry's `FailLoudMsg` printed verbatim to stderr.
Entries marked `Required: no` carry an explicit `DefaultedFor`
justification in the registry source — they are NOT silent fallbacks.
See
[`smackerel-no-defaults`](../.github/instructions/smackerel-no-defaults.instructions.md).

**Cross-references:**
- Operator runbook configuration section: [docs/Operations.md → Configuration SST](Operations.md#configuration-sst).
- Per-transport specs: [069](../specs/069-assistant-http-transport/) (HTTP), [072](../specs/072-whatsapp-business-transport/) (WhatsApp), [061](../specs/061-assistant-evaluation-harness/) (assistant Telegram), [002](../specs/002-phase1-foundation/) / [044](../specs/044-per-user-bearer-authentication/) / [045](../specs/045-recipe-cook-mode/) (legacy Telegram).
- Audit spec: [062](../specs/062-per-transport-configuration-audit/).

---

## HTTP Transport

Consumer: `internal/assistant/httpadapter`. Activated when
`assistant.transports.http.enabled=true`.

| YAML Key | Env Var | Required | Owning Package |
|----------|---------|----------|----------------|
| `assistant.transports.http.enabled` | `ASSISTANT_TRANSPORTS_HTTP_ENABLED` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.schema_version` | `ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.body_size_max_bytes` | `ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.rate_limit_per_user_per_minute` | `ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.cors_allowed_origins` | `ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.conversation_ttl_seconds` | `ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.transport_hint_allowlist` | `ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.required_scope` | `ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE` | yes | `internal/assistant/httpadapter` |
| `assistant.transports.http.shared_user_id` | `ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID` | yes | `internal/assistant/httpadapter` |

---

## WhatsApp Transport

Consumer: `internal/whatsapp/assistant_adapter`. Activated when
`assistant.transports.whatsapp.enabled=true`.

| YAML Key | Env Var | Required | Owning Package |
|----------|---------|----------|----------------|
| `assistant.transports.whatsapp.enabled` | `ASSISTANT_TRANSPORTS_WHATSAPP_ENABLED` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.webhook_path` | `ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_PATH` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.phone_number_id` | `ASSISTANT_TRANSPORTS_WHATSAPP_PHONE_NUMBER_ID` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.business_account_id` | `ASSISTANT_TRANSPORTS_WHATSAPP_BUSINESS_ACCOUNT_ID` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.webhook_verify_token_ref` | `ASSISTANT_TRANSPORTS_WHATSAPP_WEBHOOK_VERIFY_TOKEN_REF` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.app_secret_ref` | `ASSISTANT_TRANSPORTS_WHATSAPP_APP_SECRET_REF` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.access_token_ref` | `ASSISTANT_TRANSPORTS_WHATSAPP_ACCESS_TOKEN_REF` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.identity_hash_key_ref` | `ASSISTANT_TRANSPORTS_WHATSAPP_IDENTITY_HASH_KEY_REF` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.api_base_url` | `ASSISTANT_TRANSPORTS_WHATSAPP_API_BASE_URL` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.api_version` | `ASSISTANT_TRANSPORTS_WHATSAPP_API_VERSION` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.rate_limit_per_user_per_minute` | `ASSISTANT_TRANSPORTS_WHATSAPP_RATE_LIMIT_PER_USER_PER_MINUTE` | yes | `internal/whatsapp/assistant_adapter` |
| `assistant.transports.whatsapp.max_text_chars` | `ASSISTANT_TRANSPORTS_WHATSAPP_MAX_TEXT_CHARS` | yes | `internal/whatsapp/assistant_adapter` |

---

## Telegram Transports

### Assistant Telegram adapter (spec 061 SCOPE-05)

Consumer: `internal/telegram/assistant_adapter`. Activated when
`assistant.transports.telegram.enabled=true`.

| YAML Key | Env Var | Required | Owning Package |
|----------|---------|----------|----------------|
| `assistant.transports.telegram.enabled` | `ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED` | yes | `internal/telegram/assistant_adapter` |
| `assistant.transports.telegram.markdown_mode` | `ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE` | yes | `internal/telegram/assistant_adapter` |
| `assistant.transports.telegram.max_message_chars` | `ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS` | yes | `internal/telegram/assistant_adapter` |
| `assistant.transports.telegram.mode` | `ASSISTANT_TRANSPORTS_TELEGRAM_MODE` | yes | `internal/telegram/assistant_adapter` |
| `assistant.transports.telegram.webhook_secret_ref` | `ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF` | no | `internal/telegram/assistant_adapter` |
| `assistant.transports.telegram.webhook_path` | `ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH` | yes | `internal/telegram/assistant_adapter` |

### Legacy top-level Telegram bot (`internal/telegram`)

Consumer: `internal/telegram`. The pre-assistant bot transport, still
in use for capture, cook-mode, and digest delivery.

| YAML Key | Env Var | Required | Owning Package |
|----------|---------|----------|----------------|
| `telegram.bot_token` | `TELEGRAM_BOT_TOKEN` | no | `internal/telegram` |
| `telegram.chat_ids` | `TELEGRAM_CHAT_IDS` | yes | `internal/telegram` |
| `telegram.user_mapping` | `TELEGRAM_USER_MAPPING` | no | `internal/telegram` |
| `telegram.assembly_window_seconds` | `TELEGRAM_ASSEMBLY_WINDOW_SECONDS` | no | `internal/telegram` |
| `telegram.assembly_max_messages` | `TELEGRAM_ASSEMBLY_MAX_MESSAGES` | no | `internal/telegram` |
| `telegram.media_group_window_seconds` | `TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS` | no | `internal/telegram` |
| `telegram.disambiguation_timeout_seconds` | `TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS` | no | `internal/telegram` |
| `telegram.cook_session_timeout_minutes` | `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` | yes | `internal/telegram` |
| `telegram.cook_session_max_per_chat` | `TELEGRAM_COOK_SESSION_MAX_PER_CHAT` | yes | `internal/telegram` |

---

## How Fail-Loud Wiring Works

`cmd/core/main.go` (the `run` function) calls four package-level
`ValidateTransportConfig()` wrappers BEFORE `config.Load()`:

```go
httpadapter.ValidateTransportConfig()
whatsappadapter.ValidateTransportConfig()
telegramassistant.ValidateTransportConfig()
telegram.ValidateTransportConfig()
```

Each wrapper delegates to
`transportconfig.ValidateOwningPackage(<package>, os.LookupEnv)`, which
iterates every `Required: yes` registry entry for that package and, for
any missing or empty env var, returns an error whose message is the
registry's `FailLoudMsg` verbatim. `main.go` prefixes the message with
the offending scope (e.g. `"http transport configuration: …"`) and
exits non-zero.

Operators see the YAML key name in the error message because the
registry's package-level `init()` asserts every `FailLoudMsg` begins
with its `YAMLKey`. The end-to-end behavior is covered by
`TestHTTPAdapter_MissingRequiredKey_FailsLoud` (SCN-062-A05) under
`go test -tags e2e ./tests/e2e/`.

## Adding or Changing an Entry

1. Edit the appropriate file under `internal/assistant/transportconfig/`
   (`http.go`, `whatsapp.go`, or `telegram.go`).
2. Update the matching row in this document.
3. If the key is new in `config/smackerel.yaml`, update
   [`scripts/commands/config.sh`](../scripts/commands/config.sh) to emit
   the env var via `required_value` (or `yaml_get` with an explicit
   `DefaultedFor:` annotation in the registry).
4. Run `./smackerel.sh test unit` — `TestRegistry_CoversYAMLNamespaces`,
   `TestRegistry_NoOrphanedEntries`,
   `TestRegistry_RequiredEntriesHaveFailLoud`,
   `TestRegistry_NoForbiddenFallbacks`, and `TestRegistry_DocSync` all
   run by default.
