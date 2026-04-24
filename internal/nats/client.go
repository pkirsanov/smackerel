// Package nats provides the NATS JetStream sdk integration layer for Smackerel.
// It wraps the nats-io/nats.go client with stream management and publish helpers.
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Subjects used by Smackerel.
const (
	SubjectArtifactsProcess   = "artifacts.process"
	SubjectArtifactsProcessed = "artifacts.processed"
	SubjectSearchEmbed        = "search.embed"
	SubjectSearchEmbedded     = "search.embedded"
	SubjectSearchRerank       = "search.rerank"
	SubjectSearchReranked     = "search.reranked"
	SubjectDigestGenerate     = "digest.generate"
	SubjectDigestGenerated    = "digest.generated"
	SubjectKeepSyncRequest    = "keep.sync.request"
	SubjectKeepSyncResponse   = "keep.sync.response"
	SubjectKeepOCRRequest     = "keep.ocr.request"
	SubjectKeepOCRResponse    = "keep.ocr.response"

	// Phase 5: Advanced Intelligence subjects
	SubjectLearningClassify   = "learning.classify"
	SubjectLearningClassified = "learning.classified"
	SubjectContentAnalyze     = "content.analyze"
	SubjectContentAnalyzed    = "content.analyzed"
	SubjectMonthlyGenerate    = "monthly.generate"
	SubjectMonthlyGenerated   = "monthly.generated"
	SubjectQuickrefGenerate   = "quickref.generate"
	SubjectQuickrefGenerated  = "quickref.generated"
	SubjectSeasonalAnalyze    = "seasonal.analyze"
	SubjectSeasonalAnalyzed   = "seasonal.analyzed"

	// Proactive alert notification subject
	SubjectAlertsNotify = "alerts.notify"

	// Knowledge synthesis subjects
	SubjectSynthesisExtract           = "synthesis.extract"
	SubjectSynthesisExtracted         = "synthesis.extracted"
	SubjectSynthesisCrossSource       = "synthesis.crosssource"
	SubjectSynthesisCrossSourceResult = "synthesis.crosssource.result"

	// Domain extraction subjects (spec 026)
	SubjectDomainExtract   = "domain.extract"
	SubjectDomainExtracted = "domain.extracted"

	// Annotation subjects (spec 027)
	SubjectAnnotationsCreated = "annotations.created"

	// Actionable list subjects (spec 028)
	SubjectListsCreated   = "lists.created"
	SubjectListsCompleted = "lists.completed"

	// Agent subjects (spec 037 — LLM Scenario Agent & Tool Registry)
	// agent.invoke.request / agent.invoke.response form the per-turn cross-language
	// pair between the Go executor and the Python ML sidecar. tool_call.executed and
	// agent.complete are core-internal events consumed by the tracer.
	SubjectAgentInvokeRequest    = "agent.invoke.request"
	SubjectAgentInvokeResponse   = "agent.invoke.response"
	SubjectAgentToolCallExecuted = "agent.tool_call.executed"
	SubjectAgentComplete         = "agent.complete"

	// Weather enrichment subjects (spec 016 — Weather Connector, Scope 05).
	// Other connectors and the digest generator publish historical-weather
	// requests on weather.enrich.request; the weather connector publishes the
	// matching reply on weather.enrich.response. Correlation is by request_id
	// inside the payload (logical request/response over JetStream pub-sub).
	SubjectWeatherEnrichRequest  = "weather.enrich.request"
	SubjectWeatherEnrichResponse = "weather.enrich.response"
)

// StreamConfig defines a JetStream stream and its subjects.
type StreamConfig struct {
	Name     string
	Subjects []string
}

// AllStreams returns the stream configurations for Smackerel.
func AllStreams() []StreamConfig {
	return []StreamConfig{
		{Name: "ARTIFACTS", Subjects: []string{"artifacts.>"}},
		{Name: "SEARCH", Subjects: []string{"search.>"}},
		{Name: "DIGEST", Subjects: []string{"digest.>"}},
		{Name: "KEEP", Subjects: []string{"keep.>"}},
		{Name: "INTELLIGENCE", Subjects: []string{"learning.>", "content.>", "monthly.>", "quickref.>", "seasonal.>"}},
		{Name: "ALERTS", Subjects: []string{"alerts.>"}},
		{Name: "SYNTHESIS", Subjects: []string{"synthesis.>"}},
		{Name: "DOMAIN", Subjects: []string{"domain.>"}},
		{Name: "ANNOTATIONS", Subjects: []string{"annotations.>"}},
		{Name: "LISTS", Subjects: []string{"lists.>"}},
		{Name: "AGENT", Subjects: []string{"agent.>"}},
		{Name: "WEATHER", Subjects: []string{"weather.>"}},
		{Name: "DEADLETTER", Subjects: []string{"deadletter.>"}},
	}
}

// Client wraps a NATS connection with JetStream support.
type Client struct {
	Conn      *nats.Conn
	JetStream jetstream.JetStream
}

// Connect establishes a NATS connection and returns a Client.
// authToken is used for NATS token-based authentication; pass empty string to skip.
func Connect(ctx context.Context, url string, authToken string) (*Client, error) {
	opts := []nats.Option{
		nats.Name("smackerel-core"),
		nats.ReconnectWait(2 * time.Second),
		// Infinite reconnect (-1): NATS is co-deployed in Docker Compose and should
		// always be reachable eventually; a finite cap risks permanent disconnection
		// during container restarts or brief network blips.
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			slog.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
	}

	if authToken != "" {
		opts = append(opts, nats.Token(authToken))
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS at %s: %w", url, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}

	slog.Info("connected to NATS", "url", nc.ConnectedUrl())
	return &Client{Conn: nc, JetStream: js}, nil
}

// EnsureStreams creates or updates all required JetStream streams.
func (c *Client) EnsureStreams(ctx context.Context) error {
	for _, sc := range AllStreams() {
		cfg := jetstream.StreamConfig{
			Name:      sc.Name,
			Subjects:  sc.Subjects,
			Retention: jetstream.WorkQueuePolicy,
			MaxAge:    7 * 24 * time.Hour, // 7 days — prevent message loss during extended ML outages
			Storage:   jetstream.FileStorage,
		}

		// DEADLETTER stream uses LimitsPolicy (inspectable, not consumed-and-deleted)
		if sc.Name == "DEADLETTER" {
			cfg.Retention = jetstream.LimitsPolicy
			cfg.MaxAge = 30 * 24 * time.Hour // 30 days retention for forensic inspection
			cfg.MaxMsgs = 10000              // prevent unbounded growth
		}

		_, err := c.JetStream.CreateOrUpdateStream(ctx, cfg)
		if err != nil {
			return fmt.Errorf("create/update stream %s: %w", sc.Name, err)
		}
		slog.Info("ensured NATS stream", "name", sc.Name, "subjects", sc.Subjects)
	}
	return nil
}

// Publish publishes a message to a NATS subject via JetStream.
func (c *Client) Publish(ctx context.Context, subject string, data []byte) error {
	_, err := c.JetStream.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}

// PublishWithHeaders publishes a message to a NATS subject via JetStream
// with optional headers. Used for trace context propagation (W3C traceparent).
func (c *Client) PublishWithHeaders(ctx context.Context, subject string, data []byte, headers nats.Header) error {
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  headers,
	}
	_, err := c.JetStream.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", subject, err)
	}
	return nil
}

// Request issues a synchronous core-NATS request to subject and waits up to
// timeout for a single reply. Used for cross-language request/reply where the
// Go core blocks on an LLM-enhanced response from the Python ML sidecar.
//
// This intentionally uses the underlying core nats.Conn (not JetStream),
// because JetStream Publish is fire-and-forget and does not deliver the
// responder's reply on the auto-generated inbox. Callers that want delivery
// guarantees with no reply should keep using Publish.
//
// SST: timeout MUST be > 0. There is no hidden default — callers pass an
// explicit per-feature timeout (see internal/intelligence callers and the
// BUG-003 scope DoD for the agreed values).
//
// Returns the reply payload on success. On timeout, no responder, or transport
// failure, returns a wrapped error so the caller can decide whether to fall
// back. The provided ctx is also honored: if it is cancelled before the reply
// arrives, RequestWithContext returns ctx.Err wrapped.
func (c *Client) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error) {
	if c == nil || c.Conn == nil {
		return nil, fmt.Errorf("nats request to %s: client is nil", subject)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("nats request to %s: timeout must be > 0", subject)
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg, err := c.Conn.RequestWithContext(reqCtx, subject, data)
	if err != nil {
		return nil, fmt.Errorf("nats request to %s: %w", subject, err)
	}
	if msg == nil {
		return nil, fmt.Errorf("nats request to %s: nil reply message", subject)
	}
	return msg.Data, nil
}

// Healthy checks if the NATS connection is active.
func (c *Client) Healthy() bool {
	return c.Conn.IsConnected()
}

// Close drains and closes the NATS connection with a timeout
// to prevent shutdown from hanging if drain cannot complete.
// The drain timeout is 2s to stay within the shutdown step budget
// allocated by shutdownAll (IMP-022-R29-002).
func (c *Client) Close() {
	// Start drain in background; if it doesn't complete within 2 seconds,
	// force-close the connection so shutdown can proceed without leaking
	// a background goroutine that races with subsequent shutdown steps.
	done := make(chan struct{})
	go func() {
		if err := c.Conn.Drain(); err != nil {
			slog.Warn("NATS drain error", "error", err)
		}
		close(done)
	}()
	select {
	case <-done:
		// Drain completed
	case <-time.After(2 * time.Second):
		slog.Warn("NATS drain timed out after 2s, force-closing connection")
		c.Conn.Close()
	}
}
