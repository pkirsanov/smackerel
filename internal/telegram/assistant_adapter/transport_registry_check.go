package assistant_adapter

// Spec 062 SCOPE-2 — Telegram (assistant) adapter delegates
// per-transport SST fail-loud checks to the transportconfig
// registry. The registry covers both the
// assistant.transports.telegram.* keys (this package's namespace)
// and the legacy top-level telegram.* keys (owned by
// internal/telegram). This validator is scoped to the assistant
// namespace; the legacy package owns its own ValidateTransportConfig.

import (
	"os"

	"github.com/smackerel/smackerel/internal/assistant/transportconfig"
)

// ValidateTransportConfig returns nil when every REQUIRED
// assistant.transports.telegram.* env var is present. The
// webhook_secret_ref entry is registered with DefaultedFor (legal
// empty when mode=long_poll) and is enforced separately by this
// package's runtime constructors.
func ValidateTransportConfig() error {
	return transportconfig.ValidateOwningPackage("internal/telegram/assistant_adapter", os.LookupEnv)
}
