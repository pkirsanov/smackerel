package assistant_adapter

// Spec 062 SCOPE-2 — WhatsApp adapter delegates per-transport SST
// fail-loud checks to the transportconfig registry. See
// internal/assistant/transportconfig/registry.go for the canonical
// per-key FailLoudMsg strings.

import (
	"os"

	"github.com/smackerel/smackerel/internal/assistant/transportconfig"
)

// ValidateTransportConfig returns nil when every REQUIRED
// assistant.transports.whatsapp.* env var is present. On the first
// missing env var it returns an error whose message is the registry
// FailLoudMsg verbatim. cmd/core/main.go MUST call this (directly
// or via transportconfig.ValidateAllFromOSEnv) before adapter
// wiring proceeds.
func ValidateTransportConfig() error {
	return transportconfig.ValidateOwningPackage("internal/whatsapp/assistant_adapter", os.LookupEnv)
}
