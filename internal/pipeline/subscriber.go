package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// ResultSubscriber listens for ML processing results on NATS and stores them.
type ResultSubscriber struct {
	DB        *pgxpool.Pool
	NATS      *smacknats.Client
	Processor *Processor
}

// NewResultSubscriber creates a subscriber for artifacts.processed messages.
func NewResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client) *ResultSubscriber {
	return &ResultSubscriber{
		DB:        db,
		NATS:      nc,
		Processor: NewProcessor(db, nc),
	}
}

// Start begins consuming artifacts.processed messages in a background goroutine.
func (rs *ResultSubscriber) Start(ctx context.Context) error {
	consumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "ARTIFACTS", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-processed",
		FilterSubject: smacknats.SubjectArtifactsProcessed,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for artifacts.processed: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				slog.Debug("fetch artifacts.processed batch", "error", err)
				continue
			}

			for msg := range msgs.Messages() {
				rs.handleMessage(ctx, msg)
			}
		}
	}()

	slog.Info("result subscriber started", "subject", smacknats.SubjectArtifactsProcessed)
	return nil
}

// handleMessage processes a single artifacts.processed message.
func (rs *ResultSubscriber) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var payload NATSProcessedPayload
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		slog.Error("invalid artifacts.processed payload", "error", err)
		// Ack to prevent infinite redelivery of malformed messages
		_ = msg.Ack()
		return
	}

	if err := rs.Processor.HandleProcessedResult(ctx, &payload); err != nil {
		slog.Error("failed to handle processed result",
			"artifact_id", payload.ArtifactID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	slog.Debug("processed result stored", "artifact_id", payload.ArtifactID)
}
