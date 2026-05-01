package photos

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrWriterUnauthorized is returned when ProviderWriter mutations are
// invoked without a valid (consumed) action token. This is the last-line
// guard that backstops the API confirmation flow.
var ErrWriterUnauthorized = errors.New("photos: provider writer requires action-token confirmation")

// ConfirmedWriter wraps a ProviderWriter and only allows mutating
// methods when an authorised ActionToken is supplied. Read-only callers
// are not affected because they never reach this layer.
type ConfirmedWriter struct {
	delegate ProviderWriter
	token    *ActionToken
}

// NewConfirmedWriter returns a ConfirmedWriter bound to a consumed
// action token. The token MUST have ConsumedAt set; the constructor
// returns ErrWriterUnauthorized otherwise so callers cannot accidentally
// pass an unconfirmed token through.
func NewConfirmedWriter(delegate ProviderWriter, token *ActionToken) (*ConfirmedWriter, error) {
	if delegate == nil {
		return nil, fmt.Errorf("photos: confirmed writer requires a delegate")
	}
	if token == nil || token.ConsumedAt == nil {
		return nil, ErrWriterUnauthorized
	}
	return &ConfirmedWriter{delegate: delegate, token: token}, nil
}

// allowsAction returns nil if the bound action-token authorises the
// supplied action kind. Mismatches return ErrWriterUnauthorized so the
// caller can map to the API limitation_code path.
func (writer *ConfirmedWriter) allowsAction(kind ActionKind) error {
	if writer == nil {
		return ErrWriterUnauthorized
	}
	if writer.token.Action != kind {
		return fmt.Errorf("%w: token action %q does not match requested %q", ErrWriterUnauthorized, writer.token.Action, kind)
	}
	return nil
}

func (writer *ConfirmedWriter) photoIsInScope(photoID string) bool {
	if writer == nil || writer.token == nil {
		return false
	}
	for _, allowed := range writer.token.Scope.PhotoIDs {
		if allowed == photoID {
			return true
		}
	}
	return false
}

// AddToAlbum delegates to the wrapped writer when the bound token grants
// the tag/album_remove/album-write action.
func (writer *ConfirmedWriter) AddToAlbum(ctx context.Context, photo string, album string) error {
	if err := writer.allowsAction(ActionAlbumRemove); err != nil && writer.token.Action != ActionTag {
		return err
	}
	if !writer.photoIsInScope(photo) {
		return fmt.Errorf("%w: photo %s not in token scope", ErrWriterUnauthorized, photo)
	}
	return writer.delegate.AddToAlbum(ctx, photo, album)
}

// Tag delegates to the wrapped writer with the same scope guard.
func (writer *ConfirmedWriter) Tag(ctx context.Context, photo string, tag string) error {
	if err := writer.allowsAction(ActionTag); err != nil {
		return err
	}
	if !writer.photoIsInScope(photo) {
		return fmt.Errorf("%w: photo %s not in token scope", ErrWriterUnauthorized, photo)
	}
	return writer.delegate.Tag(ctx, photo, tag)
}

// Favorite delegates with scope guard.
func (writer *ConfirmedWriter) Favorite(ctx context.Context, photo string, on bool) error {
	if err := writer.allowsAction(ActionFavorite); err != nil {
		return err
	}
	if !writer.photoIsInScope(photo) {
		return fmt.Errorf("%w: photo %s not in token scope", ErrWriterUnauthorized, photo)
	}
	return writer.delegate.Favorite(ctx, photo, on)
}

// Archive delegates with scope guard. Archive requires the token's
// action to be ActionArchive.
func (writer *ConfirmedWriter) Archive(ctx context.Context, photo string) error {
	if err := writer.allowsAction(ActionArchive); err != nil {
		return err
	}
	if !writer.photoIsInScope(photo) {
		return fmt.Errorf("%w: photo %s not in token scope", ErrWriterUnauthorized, photo)
	}
	return writer.delegate.Archive(ctx, photo)
}

// Delete delegates with scope guard. Delete additionally requires the
// token to have RequiresText=true (enforced at confirm time, but we
// re-check here so a misuse path cannot bypass it).
func (writer *ConfirmedWriter) Delete(ctx context.Context, photo string) error {
	if err := writer.allowsAction(ActionDelete); err != nil {
		return err
	}
	if !writer.token.RequiresText {
		return fmt.Errorf("%w: delete requires text-confirmed token", ErrWriterUnauthorized)
	}
	if !writer.photoIsInScope(photo) {
		return fmt.Errorf("%w: photo %s not in token scope", ErrWriterUnauthorized, photo)
	}
	return writer.delegate.Delete(ctx, photo)
}

// TokenID exposes the bound token id for audit logging.
func (writer *ConfirmedWriter) TokenID() uuid.UUID {
	if writer == nil || writer.token == nil {
		return uuid.Nil
	}
	return writer.token.ID
}
