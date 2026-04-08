package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/digest"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// ResultSubscriber listens for ML processing results on NATS and stores them.
type ResultSubscriber struct {
	DB        *pgxpool.Pool
	NATS      *smacknats.Client
	Processor *Processor
	DigestGen *digest.Generator
	done      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	started   bool
	stopped   bool
}

// NewResultSubscriber creates a subscriber for artifacts.processed messages.
func NewResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client) *ResultSubscriber {
	return &ResultSubscriber{
		DB:        db,
		NATS:      nc,
		Processor: NewProcessor(db, nc),
		DigestGen: digest.NewGenerator(db, nc),
	}
}

// Start begins consuming artifacts.processed and digest.generated messages in background goroutines.
func (rs *ResultSubscriber) Start(ctx context.Context) error {
	rs.mu.Lock()
	if rs.stopped {
		rs.mu.Unlock()
		return fmt.Errorf("subscriber already stopped")
	}
	if rs.started {
		rs.mu.Unlock()
		return fmt.Errorf("subscriber already started")
	}
	rs.done = make(chan struct{})
	rs.started = true
	rs.mu.Unlock()

	// artifacts.processed consumer
	processedConsumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "ARTIFACTS", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-processed",
		FilterSubject: smacknats.SubjectArtifactsProcessed,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for artifacts.processed: %w", err)
	}

	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		for {
			msgs, err := processedConsumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				// Check for shutdown between fetch attempts
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch artifacts.processed batch", "error", err)
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				rs.handleMessage(ctx, msg)
			}
		}
	}()

	slog.Info("result subscriber started", "subject", smacknats.SubjectArtifactsProcessed)

	// digest.generated consumer
	digestConsumer, err := rs.NATS.JetStream.CreateOrUpdateConsumer(ctx, "DIGEST", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-digest",
		FilterSubject: smacknats.SubjectDigestGenerated,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for digest.generated: %w", err)
	}

	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		for {
			msgs, err := digestConsumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				// Check for shutdown between fetch attempts
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch digest.generated batch", "error", err)
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-rs.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				rs.handleDigestMessage(ctx, msg)
			}
		}
	}()

	slog.Info("result subscriber started", "subject", smacknats.SubjectDigestGenerated)
	return nil
}

// Stop signals the background goroutines to exit and waits for them to finish.
func (rs *ResultSubscriber) Stop() {
	rs.mu.Lock()
	if !rs.started || rs.stopped {
		rs.mu.Unlock()
		return
	}
	rs.stopped = true
	close(rs.done)
	rs.mu.Unlock()
	rs.wg.Wait()
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

// handleDigestMessage processes a single digest.generated message.
func (rs *ResultSubscriber) handleDigestMessage(ctx context.Context, msg jetstream.Msg) {
	var payload map[string]interface{}
	if err := json.Unmarshal(msg.Data(), &payload); err != nil {
		slog.Error("invalid digest.generated payload", "error", err)
		_ = msg.Ack()
		return
	}

	if err := rs.DigestGen.HandleDigestResult(ctx, payload); err != nil {
		slog.Error("failed to handle digest result", "error", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	slog.Debug("digest result stored")
}
