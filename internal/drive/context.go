package drive

import (
	"context"
	"errors"
)

// ErrOwnerUserIDMissing is returned when a provider call needs the owning
// user identifier but none was placed on the context. Callers (typically
// the HTTP handler that initiates BeginConnect) MUST attach the owner via
// WithOwnerUserID before invoking provider methods that persist state.
var ErrOwnerUserIDMissing = errors.New("drive: owner user id missing from context")

// ownerUserIDKey is the unexported context-key type used to attach the
// owning user identifier to a provider-call context. Using a private type
// prevents downstream packages from inadvertently colliding with this key
// via string keys.
type ownerUserIDKey struct{}

// WithOwnerUserID attaches the owning user identifier to ctx. Provider
// implementations MUST read the owner from context rather than from a
// per-instance configuration field so a single Provider instance can
// service requests for many owners.
func WithOwnerUserID(ctx context.Context, ownerUserID string) context.Context {
	return context.WithValue(ctx, ownerUserIDKey{}, ownerUserID)
}

// OwnerUserIDFromContext returns the owner attached via WithOwnerUserID
// or ErrOwnerUserIDMissing when the context carries no owner.
func OwnerUserIDFromContext(ctx context.Context) (string, error) {
	v, ok := ctx.Value(ownerUserIDKey{}).(string)
	if !ok || v == "" {
		return "", ErrOwnerUserIDMissing
	}
	return v, nil
}
