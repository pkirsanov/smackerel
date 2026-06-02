package transportconfig

// telegramEntries enumerates both the assistant.transports.telegram.*
// keys (spec 061 SCOPE-05 assistant adapter) and the top-level
// telegram.* legacy bot keys consumed by internal/telegram. Values
// are verbatim from config/smackerel.yaml as resolved by
// scripts/commands/config.sh. Required==true mirrors required_value
// in the generator; entries using yaml_get (permissively-empty at
// generate time) are marked Required==false even though the runtime
// validator may further constrain them (e.g. webhook_secret_ref
// becomes non-empty when mode=webhook).
var telegramEntries = []Entry{
	// --- assistant.transports.telegram.* (spec 061 SCOPE-05) ---
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.enabled",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_ENABLED",
		Required:      true,
		FailLoudMsg:   "assistant.transports.telegram.enabled is required (at least one transport MUST be enabled when assistant.enabled=true)",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.markdown_mode",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_MARKDOWN_MODE",
		Required:      true,
		FailLoudMsg:   "assistant.transports.telegram.markdown_mode is required (\"MarkdownV2\" | \"plain\" | \"HTML\")",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.max_message_chars",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_MAX_MESSAGE_CHARS",
		Required:      true,
		FailLoudMsg:   "assistant.transports.telegram.max_message_chars is required (Telegram per-message hard cap)",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.mode",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_MODE",
		Required:      true,
		FailLoudMsg:   "assistant.transports.telegram.mode is required (\"long_poll\" | \"webhook\")",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05 §17",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.webhook_secret_ref",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_SECRET_REF",
		Required:      false,
		FailLoudMsg:   "assistant.transports.telegram.webhook_secret_ref is required (non-empty env-var name when mode=webhook; \"\" permitted when mode=long_poll)",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05 §17.5",
		DefaultedFor:  "permissively-empty at generator boundary; Go validator enforces non-empty when mode=webhook",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "assistant.transports.telegram.webhook_path",
		EnvVar:        "ASSISTANT_TRANSPORTS_TELEGRAM_WEBHOOK_PATH",
		Required:      true,
		FailLoudMsg:   "assistant.transports.telegram.webhook_path is required (path starting with \"/\"; recommended /v1/telegram/webhook)",
		OwningPackage: "internal/telegram/assistant_adapter",
		IntroducedBy:  "specs/061-assistant-evaluation-harness SCOPE-05 §17.5",
	},

	// --- top-level telegram.* (legacy bot transport, internal/telegram) ---
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.bot_token",
		EnvVar:        "TELEGRAM_BOT_TOKEN",
		Required:      false,
		FailLoudMsg:   "telegram.bot_token is required (Telegram bot token from @BotFather; empty permitted in dev/test only)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/002-phase1-foundation",
		DefaultedFor:  "dev/test allows empty bot_token; production-class targets resolve via SHELL_SECRET_KEYS placeholder",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.chat_ids",
		EnvVar:        "TELEGRAM_CHAT_IDS",
		Required:      true,
		FailLoudMsg:   "telegram.chat_ids is required (comma-separated allow-list of Telegram chat ids)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/002-phase1-foundation",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.user_mapping",
		EnvVar:        "TELEGRAM_USER_MAPPING",
		Required:      false,
		FailLoudMsg:   "telegram.user_mapping is required (\"<chat_id>:<user_id>\" pairs, comma-separated; empty permitted in dev/test)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/044 SCOPE-03",
		DefaultedFor:  "empty mapping permitted in dev/test single-user flows; production drops unmapped chats",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.assembly_window_seconds",
		EnvVar:        "TELEGRAM_ASSEMBLY_WINDOW_SECONDS",
		Required:      false,
		FailLoudMsg:   "telegram.assembly_window_seconds is required (message-assembly window in seconds)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/002-phase1-foundation",
		DefaultedFor:  "generator yaml_get with literal fallback \"10\"; carry-over from pre-NO-DEFAULTS era",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.assembly_max_messages",
		EnvVar:        "TELEGRAM_ASSEMBLY_MAX_MESSAGES",
		Required:      false,
		FailLoudMsg:   "telegram.assembly_max_messages is required (max messages per assembly window)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/002-phase1-foundation",
		DefaultedFor:  "generator yaml_get with literal fallback \"100\"; carry-over from pre-NO-DEFAULTS era",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.media_group_window_seconds",
		EnvVar:        "TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS",
		Required:      false,
		FailLoudMsg:   "telegram.media_group_window_seconds is required (media-group debouncing window in seconds)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/002-phase1-foundation",
		DefaultedFor:  "generator yaml_get with literal fallback \"3\"; carry-over from pre-NO-DEFAULTS era",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.disambiguation_timeout_seconds",
		EnvVar:        "TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS",
		Required:      false,
		FailLoudMsg:   "telegram.disambiguation_timeout_seconds is required (disambiguation prompt expiry in seconds)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/004-phase3-intelligence",
		DefaultedFor:  "generator yaml_get with literal fallback \"120\"; carry-over from pre-NO-DEFAULTS era",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.cook_session_timeout_minutes",
		EnvVar:        "TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES",
		Required:      true,
		FailLoudMsg:   "telegram.cook_session_timeout_minutes is required (inactivity timeout before cook session auto-expires; range [5, 480])",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/045-recipe-cook-mode",
	},
	{
		Transport:     "telegram",
		YAMLKey:       "telegram.cook_session_max_per_chat",
		EnvVar:        "TELEGRAM_COOK_SESSION_MAX_PER_CHAT",
		Required:      true,
		FailLoudMsg:   "telegram.cook_session_max_per_chat is required (max concurrent cook sessions per chat; always 1 in v1)",
		OwningPackage: "internal/telegram",
		IntroducedBy:  "specs/045-recipe-cook-mode",
	},
}
