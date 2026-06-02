package httpadapter

// Spec 062 SCOPE-2 — HTTP adapter delegates per-transport SST
// fail-loud checks to the transportconfig registry. Calling
// ValidateTransportConfig at adapter startup ensures every
// REQUIRED assistant.transports.http.* env var is present before
// any HTTP route is mounted. The registry owns the exact
// operator-facing message; this adapter forwards that message
// verbatim.

import (
	"os"

	"github.com/smackerel/smackerel/internal/assistant/transportconfig"
)

// ValidateTransportConfig returns nil when every REQUIRED
// assistant.transports.http.* env var is present. On the first
// missing env var it returns an error whose message is the
// registry FailLoudMsg verbatim. cmd/core/main.go MUST call this
// (directly or via transportconfig.ValidateAllFromOSEnv) before
// adapter wiring proceeds — see spec 062 design.md §4 Migration
// Strategy step 2.
func ValidateTransportConfig() error {
	return transportconfig.ValidateOwningPackage("internal/assistant/httpadapter", os.LookupEnv)
}
