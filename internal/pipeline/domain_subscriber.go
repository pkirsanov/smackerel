package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/metrics"
	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// domainMaxDeliver is the maximum delivery attempts for domain.extracted messages
// before dead-letter routing. Must match the MaxDeliver in the consumer config.
const domainMaxDeliver = 5

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
		// S-001: Set domain_extracted_at on failure path per SCN-026-03.
		_, err := d.DB.Exec(ctx,
			`UPDATE artifacts SET domain_extraction_status = 'failed', domain_extracted_at = NOW(), updated_at = NOW() WHERE id = $1`,
			resp.ArtifactID,
		)
		if err != nil {
			slog.Error("update artifact domain status to failed",
				"artifact_id", resp.ArtifactID,
				"error", err,
			)
			// S-004: Nak instead of Ack when DB update fails so the message retries
			// and status tracking isn't silently lost.
			d.handleDomainDeliveryFailure(ctx, msg, err)
			return
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
		// S-003: Route to dead-letter after MaxDeliver exhausted instead of silent drop.
		d.handleDomainDeliveryFailure(ctx, msg, err)
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

// handleDomainDeliveryFailure routes an exhausted domain.extracted message to the
// DEADLETTER stream when MaxDeliver is reached; otherwise Naks for retry.
// S-003: Mirrors the dead-letter routing from ResultSubscriber.handleDeliveryFailure.
func (d *DomainResultSubscriber) handleDomainDeliveryFailure(ctx context.Context, msg jetstream.Msg, lastErr error) {
	md, mdErr := msg.Metadata()
	if mdErr != nil || int(md.NumDelivered) < domainMaxDeliver {
		_ = msg.Nak()
		return
	}

	headers := nats.Header{}
	headers.Set("Smackerel-Original-Subject", smacknats.SubjectDomainExtracted)
	headers.Set("Smackerel-Original-Stream", "DOMAIN")
	headers.Set("Smackerel-Failed-At", time.Now().UTC().Format(time.RFC3339))
	if lastErr != nil {
		errStr := lastErr.Error()
		if len(errStr) > 256 {
			errStr = stringutil.TruncateUTF8(errStr, 256)
		}
		headers.Set("Smackerel-Last-Error", errStr)
	}
	if md != nil {
		headers.Set("Smackerel-Delivery-Count", strconv.FormatUint(md.NumDelivered, 10))
		if md.Consumer != "" {
			headers.Set("Smackerel-Original-Consumer", md.Consumer)
		}
	}

	dlSubject := "deadletter." + smacknats.SubjectDomainExtracted
	dlMsg := &nats.Msg{
		Subject: dlSubject,
		Data:    msg.Data(),
		Header:  headers,
	}

	if _, err := d.NATS.JetStream.PublishMsg(ctx, dlMsg); err != nil {
		slog.Error("domain dead-letter publish failed, Nak to preserve message", "error", err)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	metrics.NATSDeadLetter.WithLabelValues("DOMAIN").Inc()
	slog.Warn("domain message routed to dead-letter", "subject", dlSubject)
}
