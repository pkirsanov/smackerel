package main

// wiring_selfknowledge.go — spec 104 SCOPE-03 boot lifecycle.
//
// Runs the self-knowledge corpus ingestion once at startup (after
// migrations, gated on assistant.open_knowledge.enabled). It derives the
// CapabilityEntry corpus from the live SSTs (scenarios.yaml + shortcuts via
// the skills manifest) and publishes each entry through the SAME
// pipeline.RawArtifactPublisher every connector uses — so each entry receives
// a real embedding + content-hash dedup and lands under source_id
// "smackerel_self". Idempotent: re-running publishes nothing new and sweeps
// entries that no longer exist in the corpus. See internal/assistant/selfknowledge.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/selfknowledge"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// wireSelfKnowledgeIngestion derives + ingests the smackerel_self corpus at
// boot. No-op when open_knowledge is disabled (the corpus is only consulted by
// the /ask self_knowledge tool). Fail-loud when enabled but a required
// dependency (pool, NATS, manifest) is missing or ingestion fails — a silent
// empty self-knowledge corpus would make every product meta-question refuse.
func wireSelfKnowledgeIngestion(ctx context.Context, cfg *config.Config, svc *coreServices, agentScenarioDir string) error {
	if cfg == nil {
		return errors.New("wireSelfKnowledgeIngestion: nil config")
	}
	if !cfg.Assistant.OpenKnowledge.Enabled {
		slog.Info("self-knowledge ingestion skipped (assistant.open_knowledge.enabled=false); /ask self-knowledge corpus not populated")
		return nil
	}
	if svc == nil || svc.pg == nil || svc.pg.Pool == nil {
		return errors.New("wireSelfKnowledgeIngestion: postgres pool is required when open_knowledge is enabled")
	}
	if svc.nc == nil {
		return errors.New("wireSelfKnowledgeIngestion: NATS client is required (PublishRawArtifact enqueues embedding jobs)")
	}
	if agentScenarioDir == "" {
		return errors.New("wireSelfKnowledgeIngestion: agentScenarioDir is empty; SCOPE-03 validator should have failed first")
	}

	manifestPath := filepath.Join(filepath.Dir(agentScenarioDir), assistantManifestRelPath)
	manifest, err := assistant.LoadSkillsManifest(manifestPath, assistantEnableResolver(cfg))
	if err != nil {
		return fmt.Errorf("wireSelfKnowledgeIngestion: load skills manifest %s: %w", manifestPath, err)
	}

	// Same publisher every connector uses: real embedding job + content-hash
	// dedup, never a bespoke INSERT (G029).
	publisher := pipeline.NewRawArtifactPublisher(svc.pg.Pool, svc.nc)
	ingestor := selfknowledge.NewIngestor(manifest, publisher, svc.pg.Pool).
		// SCOPE-05 — add the curated product-overview facet (embedded, so it
		// works in the docs-less runtime image). Fail-loud on a missing anchor.
		WithDocSource(selfknowledge.NewDocCorpus())

	res, err := ingestor.Ingest(ctx)
	if err != nil {
		return fmt.Errorf("wireSelfKnowledgeIngestion: ingest self-knowledge corpus: %w", err)
	}
	slog.Info("self-knowledge corpus ingested",
		"namespace", selfknowledge.SelfKnowledgeNamespace,
		"entries", res.Entries,
		"published", res.Published,
		"swept", res.Swept)
	return nil
}
