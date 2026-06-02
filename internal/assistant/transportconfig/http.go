package transportconfig

// httpEntries enumerates assistant.transports.http.* keys from
// config/smackerel.yaml as resolved by scripts/commands/config.sh
// (spec 069 SCOPE-1c-bis). All keys are REQUIRED at the generator
// boundary (required_value); the Go validator further enforces
// non-empty resolution when enabled=true.
var httpEntries = []Entry{
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.enabled",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_ENABLED",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.enabled is required (strict bool \"true\"|\"false\")",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.schema_version",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_SCHEMA_VERSION",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.schema_version is required (pinned wire schema version, e.g. \"v1\")",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.body_size_max_bytes",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_BODY_SIZE_MAX_BYTES",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.body_size_max_bytes is required (integer >= 1)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.rate_limit_per_user_per_minute",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_RATE_LIMIT_PER_USER_PER_MINUTE",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.rate_limit_per_user_per_minute is required (integer >= 1)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.cors_allowed_origins",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_CORS_ALLOWED_ORIGINS",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.cors_allowed_origins is required (explicit origin list; empty CSV = same-origin only)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.conversation_ttl_seconds",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_CONVERSATION_TTL_SECONDS",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.conversation_ttl_seconds is required (integer >= 1)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.transport_hint_allowlist",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_TRANSPORT_HINT_ALLOWLIST",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.transport_hint_allowlist is required (non-empty closed-vocabulary CSV)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-1c-bis",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.required_scope",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_REQUIRED_SCOPE",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.required_scope is required (spec 060 scope-claim label, e.g. \"assistant:turn\")",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/060-bearer-auth-scope-claim SCOPE-2",
	},
	{
		Transport:     "http",
		YAMLKey:       "assistant.transports.http.shared_user_id",
		EnvVar:        "ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID",
		Required:      true,
		FailLoudMsg:   "assistant.transports.http.shared_user_id is required (synthetic user id for shared-token sessions)",
		OwningPackage: "internal/assistant/httpadapter",
		IntroducedBy:  "specs/069-assistant-http-transport SCOPE-2 F-069-USERID-BINDING",
	},
}
