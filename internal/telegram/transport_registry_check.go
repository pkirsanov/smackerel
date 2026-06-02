package telegram

// Spec 062 SCOPE-2 — legacy Telegram bot transport delegates
// per-transport SST fail-loud checks for the top-level telegram.*
// YAML namespace to the transportconfig registry. The
// assistant-side telegram namespace is owned by
// internal/telegram/assistant_adapter and validated there.

import (
	"os"

	"github.com/smackerel/smackerel/internal/assistant/transportconfig"
)

// ValidateTransportConfig returns nil when every REQUIRED legacy
// telegram.* env var is present. Several legacy keys (bot_token,
// user_mapping, assembly_*, media_group_window_seconds,
// disambiguation_timeout_seconds) are registered with DefaultedFor
// rationales and are enforced separately by this package's runtime
// constructors when the bot actually runs.
func ValidateTransportConfig() error {
	return transportconfig.ValidateOwningPackage("internal/telegram", os.LookupEnv)
}
