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
		nats.MaxReconnects(60),
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

// Healthy checks if the NATS connection is active.
func (c *Client) Healthy() bool {
	return c.Conn.IsConnected()
}

// Close drains and closes the NATS connection with a timeout
// to prevent shutdown from hanging if drain cannot complete.
func (c *Client) Close() {
	// Start drain in background; if it doesn't complete within 5 seconds,
	// force-close the connection so shutdown can proceed.
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
	case <-time.After(5 * time.Second):
		slog.Warn("NATS drain timed out after 5s, force-closing connection")
		c.Conn.Close()
	}
}
