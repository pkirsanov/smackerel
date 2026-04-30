package monitor

import (
	"context"
	"fmt"

	"github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/scan"
)

// Service applies provider change-feed deltas to the scan read model.
type Service struct {
	provider drive.Provider
	store    scan.Store
}

// NewService returns a monitor service.
func NewService(provider drive.Provider, store scan.Store) *Service {
	return &Service{provider: provider, store: store}
}

// RunOnce consumes one provider change page and persists cursor-backed deltas.
func (service *Service) RunOnce(ctx context.Context, connectionID string) (scan.Result, error) {
	if service.provider == nil {
		return scan.Result{}, fmt.Errorf("drive monitor: provider is nil")
	}
	if service.store == nil {
		return scan.Result{}, fmt.Errorf("drive monitor: store is nil")
	}
	conn, err := service.store.LoadConnection(ctx, connectionID)
	if err != nil {
		return scan.Result{}, err
	}
	cursor, err := service.store.LoadCursor(ctx, connectionID)
	if err != nil {
		return scan.Result{}, err
	}
	jobID, err := service.store.StartJob(ctx, connectionID, "monitor")
	if err != nil {
		return scan.Result{}, err
	}
	changes, nextCursor, err := service.provider.Changes(ctx, connectionID, cursor)
	if err != nil {
		_ = service.store.RecordProviderError(ctx, connectionID, "monitor", err)
		_ = service.store.FailJob(ctx, jobID, err)
		return scan.Result{}, err
	}

	result := scan.Result{}
	for _, change := range changes {
		result.SeenCount = result.SeenCount + 1
		switch change.Kind {
		case drive.ChangeCursorInv:
			if err := service.store.MarkRescanStarted(ctx, connectionID); err != nil {
				return result, err
			}
			scanResult, scanErr := scan.NewService(service.provider, service.store).InitialScan(ctx, connectionID)
			if scanErr != nil {
				_ = service.store.FailJob(ctx, jobID, scanErr)
				return result, scanErr
			}
			result.IndexedCount = result.IndexedCount + scanResult.IndexedCount
			result.UpsertedCount = result.UpsertedCount + scanResult.UpsertedCount
			if err := service.store.MarkRescanCompleted(ctx, connectionID); err != nil {
				return result, err
			}
		case drive.ChangeUpsert:
			if change.Item.ProviderFileID == "" {
				return result, fmt.Errorf("drive monitor: upsert change for %s missing item metadata", change.ProviderFileID)
			}
			if _, err := service.store.UpsertFile(ctx, conn, change.Item); err != nil {
				_ = service.store.FailJob(ctx, jobID, err)
				return result, err
			}
			result.IndexedCount = result.IndexedCount + 1
			result.UpsertedCount = result.UpsertedCount + 1
		case drive.ChangeMove:
			if change.Item.ProviderFileID == "" {
				return result, fmt.Errorf("drive monitor: move change for %s missing item metadata", change.ProviderFileID)
			}
			if _, err := service.store.UpsertFile(ctx, conn, change.Item); err != nil {
				_ = service.store.FailJob(ctx, jobID, err)
				return result, err
			}
			result.MovedCount = result.MovedCount + 1
		case drive.ChangeTrash, drive.ChangeDelete, drive.ChangePermLost:
			if err := service.store.MarkRemoved(ctx, connectionID, change.ProviderFileID, change.Kind); err != nil {
				_ = service.store.FailJob(ctx, jobID, err)
				return result, err
			}
			result.TombstonedCount = result.TombstonedCount + 1
		default:
			return result, fmt.Errorf("drive monitor: unsupported change kind %q", change.Kind)
		}
		if err := service.store.UpdateJob(ctx, jobID, result); err != nil {
			return result, err
		}
	}
	if nextCursor != "" {
		if err := service.store.UpsertCursor(ctx, connectionID, nextCursor); err != nil {
			return result, err
		}
	}
	if err := service.store.RecordProviderSuccess(ctx, connectionID); err != nil {
		return result, err
	}
	if err := service.store.CompleteJob(ctx, jobID, result); err != nil {
		return result, err
	}
	return result, nil
}
