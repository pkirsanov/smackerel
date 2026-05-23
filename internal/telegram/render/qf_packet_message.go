// Package render provides Telegram-surface render helpers that depend
// on the qf-decisions connector. It exists so the Telegram packet
// rendering pipeline has a stable, package-typed entry point for
// emitting signed pre-MVP callback envelopes (Scope 8 / SCN-SM-041-028,
// SCN-SM-041-029).
//
// The package owns NO additional behaviour beyond delegating to
// qfdecisions.PostCallback — every signing, transport, metric, and
// audit-envelope decision lives in the qfdecisions package. Keeping
// the entry point inside internal/telegram/render/ allows the Scope 3
// Telegram render code to wire callback emission without import
// cycles or one-off boundary helpers in non-Telegram packages.
package render

import (
	"context"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

// EmitSignedCallback signs the supplied envelope via the supplied
// CallbackSigner and POSTs it through the supplied QF Companion Bridge
// Client. It is a thin pass-through to qfdecisions.PostCallback and
// inherits every behavioural guarantee documented on that function:
//
//   - On any local signature failure (NO_ACTIVE_KEY,
//     MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE) the
//     network is never reached, the signature-failure metric is
//     incremented, the Cross-Product Audit Envelope v1 record is
//     emitted with action=callback_attempt outcome=rejected
//     reason=<documented vocabulary>, and the returned error is a
//     *qfdecisions.CallbackSignatureFailure.
//   - On QF CALLBACK_DEFERRED_TO_V1 rejection the result has
//     Status=rejected_v1_deferred, the attempts metric is incremented
//     under that label, the audit envelope records the rejection,
//     and the Go-level return is (result, nil) because pre-MVP
//     deferral is the documented outcome, not a connector failure.
//     No retry is attempted; no local action acceptance is recorded.
//   - On HTTP 2xx the result has Status=ok. PP10 forbids any local
//     action acceptance even on 2xx; the callback is an emission, not
//     a state mutation.
//
// Callers MUST treat a nil signer or nil client as "callback signing
// not configured in this environment" and skip the emission rather
// than POSTing an unsigned envelope.
func EmitSignedCallback(
	ctx context.Context,
	client *qfdecisions.Client,
	signer *qfdecisions.CallbackSigner,
	env qfdecisions.CallbackEnvelope,
) (qfdecisions.CallbackAttemptResult, error) {
	return qfdecisions.PostCallback(ctx, client, signer, env)
}
