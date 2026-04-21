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

	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// DomainResultSubscriber consumes domain.extracted messages from the ML sidecar
// and stores domain-specific structured data into the artifacts table.
type DomainResultSubscriber struct {
	DB      *pgxpool.Pool
	NATS    *smacknats.Client
	done    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	started bool
	stopped bool
}

// NewDomainResultSubscriber creates a subscriber for domain.extracted messages.
func NewDomainResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client) *DomainResultSubscriber {
	return &DomainResultSubscriber{
		DB:   db,
		NATS: nc,
	}
}

// Start begins consuming domain.extracted messages in a background goroutine.
func (d *DomainResultSubscriber) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return fmt.Errorf("domain subscriber already stopped")
	}
	if d.started {
		d.mu.Unlock()
		return fmt.Errorf("domain subscriber already started")
	}
	d.done = make(chan struct{})
	d.started = true
	d.mu.Unlock()

	consumer, err := d.NATS.JetStream.CreateOrUpdateConsumer(ctx, "DOMAIN", jetstream.ConsumerConfig{
		Durable:       "smackerel-core-domain-extracted",
		FilterSubject: smacknats.SubjectDomainExtracted,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create consumer for domain.extracted: %w", err)
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		for {
			msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
			if err != nil {
				select {
				case <-d.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				slog.Debug("fetch domain.extracted batch", "error", err)
				select {
				case <-d.done:
					return
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
				}
				continue
			}

			for msg := range msgs.Messages() {
				select {
				case <-d.done:
					return
				case <-ctx.Done():
					return
				default:
				}
				d.handleDomainExtracted(ctx, msg)
			}
		}
	}()

	slog.Info("domain result subscriber started", "subject", smacknats.SubjectDomainExtracted)
	return nil
}

// Stop signals the background goroutine to exit and waits with a bounded timeout.
func (d *DomainResultSubscriber) Stop() {
	d.mu.Lock()
	if !d.started || d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	close(d.done)
	d.mu.Unlock()

	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("domain result subscriber stopped cleanly")
	case <-time.After(5 * time.Second):
		slog.Warn("domain result subscriber stop timed out")
	}
}

// handleDomainExtracted processes a single domain.extracted message.
func (d *DomainResultSubscriber) handleDomainExtracted(ctx context.Context, msg jetstream.Msg) {
	var resp DomainExtractResponse
	if err := json.Unmarshal(msg.Data(), &resp); err != nil {
		slog.Error("invalid domain.extracted payload", "error", err)
		_ = msg.Ack()
		return
	}

	if err := ValidateDomainExtractResponse(&resp); err != nil {
		slog.Error("domain.extracted payload validation failed", "error", err)
		_ = msg.Ack()
		return
	}

	if !resp.Success {
		_, err := d.DB.Exec(ctx,
			`UPDATE artifacts SET domain_extraction_status = 'failed', updated_at = NOW() WHERE id = $1`,
			resp.ArtifactID,
		)
		if err != nil {
			slog.Error("update artifact domain status to failed",
				"artifact_id", resp.ArtifactID,
				"error", err,
			)
		}
		metrics.DomainExtraction.WithLabelValues(resp.ContractVersion, "failed").Inc()
		_ = msg.Ack()
		slog.Warn("domain extraction failed",
			"artifact_id", resp.ArtifactID,
			"error", resp.Error,
		)
		return
	}

	_, err := d.DB.Exec(ctx,
		`UPDATE artifacts SET
			domain_data = $2,
			domain_extraction_status = 'completed',
			domain_schema_version = $3,
			domain_extracted_at = NOW(),
			updated_at = NOW()
		WHERE id = $1`,
		resp.ArtifactID, resp.DomainData, resp.ContractVersion,
	)
	if err != nil {
		slog.Error("store domain extraction result",
			"artifact_id", resp.ArtifactID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	metrics.DomainExtraction.WithLabelValues(resp.ContractVersion, "completed").Inc()
	_ = msg.Ack()
	slog.Info("domain extraction completed",
		"artifact_id", resp.ArtifactID,
		"contract_version", resp.ContractVersion,
		"processing_ms", resp.ProcessingTimeMs,
	)
}
