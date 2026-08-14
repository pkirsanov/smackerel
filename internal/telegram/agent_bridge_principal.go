// Production PrincipalResolver for the Telegram → agent bridge (BUG-061-012
// R3.2).
//
// The resolver reuses the SAME two steps the per-user token minter already
// uses — Bot.resolveActorUserID for chat → user, then the principal's recorded
// grant set narrowed through auth.DeriveTelegramBridgeGrants — rather than
// deriving authority a second way. A second derivation is how the bridge and
// the token surface drift apart, which is the class of defect BUG-061-012 is.
//
// Nothing here confers a grant. DeriveTelegramBridgeGrants returns
// recorded ∩ ceiling, so the session can only ever hold a subset of what the
// principal actually holds. A user without corpus:read reaches the agent as a
// real, named principal that the retrieval tool then refuses — which is the
// distinction SCN-03 needs and an absent session could not express.

package telegram

import (
	"context"
	"errors"
	"fmt"

	"github.com/smackerel/smackerel/internal/auth"
)

// NewBotPrincipalResolver returns the production PrincipalResolver.
//
// grants is required. A nil reader is refused at construction rather than
// degraded to "no grants", because those two states are not the same: the
// first is a wiring bug that must be loud, the second is a legitimate
// authorization outcome. Collapsing them would make a misconfigured deployment
// look exactly like a correctly-configured one serving unprivileged users.
func NewBotPrincipalResolver(b *Bot, grants PrincipalGrantReader) (PrincipalResolver, error) {
	if b == nil {
		return nil, errors.New("telegram.NewBotPrincipalResolver: bot is required")
	}
	if grants == nil {
		return nil, errors.New("telegram.NewBotPrincipalResolver: principal grant reader is required")
	}
	return func(ctx context.Context, chatID int64) (auth.Session, error) {
		userID, err := b.resolveActorUserID(chatID)
		if err != nil {
			// Production + unmapped chat.
			return auth.Session{}, err
		}
		if userID == "" {
			// Dev/test + unmapped chat: resolveActorUserID returns ("", nil)
			// here, so the error alone does NOT separate mapped from unmapped
			// outside production. Converting the empty user into an explicit
			// refusal is what keeps a non-production chat out of the
			// authenticated path instead of handing the agent a principal
			// that identifies nobody.
			return auth.Session{}, fmt.Errorf("telegram.principalResolver: chat_id %d has no user mapping (set TELEGRAM_USER_MAPPING)", chatID)
		}

		recorded, err := grants.GrantsForPrincipal(ctx, userID)
		if err != nil {
			// Unreadable grants deny. Proceeding with an empty set would be
			// indistinguishable from a user who genuinely holds nothing, and
			// a read failure is not an authorization answer.
			return auth.Session{}, fmt.Errorf("telegram.principalResolver: grants for %q: %w", userID, err)
		}
		if !recorded.Recorded {
			// granted_scopes IS NULL — the standing token predates grant
			// recording, so what this principal holds is UNKNOWN, not empty.
			// auth.RecordedGrants exists precisely to keep those apart
			// (principal_grants.go doc), and DeriveTelegramBridgeGrants would
			// erase the distinction: passing the meaningless nil Scopes yields
			// an empty set indistinguishable from a user who genuinely holds
			// nothing. The operator remedy differs — rotate to record, not
			// re-issue — so the diagnostic names the token to rotate. Same
			// sentinel the per-user token minter uses, because it is the same
			// condition with the same remedy.
			return auth.Session{}, fmt.Errorf("telegram.principalResolver: principal %q (token %q): %w", userID, recorded.TokenID, ErrPrincipalGrantsUnrecorded)
		}

		// A recorded-but-undelegable set is NOT a refusal here, unlike in the
		// token minter. The user reaches the agent as a real, named principal
		// holding nothing, which is what lets retrieval answer
		// `retrieval_search_grant_required` (SCN-03) rather than
		// `retrieval_search_no_principal` (SCN-02).
		return auth.Session{
			UserID: userID,
			Source: auth.SessionSourcePerUserToken,
			Scopes: auth.DeriveTelegramBridgeGrants(recorded.Scopes),
		}, nil
	}, nil
}
