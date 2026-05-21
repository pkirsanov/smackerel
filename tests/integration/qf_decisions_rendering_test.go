//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestQFDecisionPacketMetadataPersistsIntoSearchRenderCard proves Scope 3's
// rendering surfaces have the QF-authored packet metadata after connector
// publication. It normalizes a QF packet, persists it through the real
// RawArtifactPublisher (PostgreSQL + NATS), then reads it through the search
// engine's text fallback path and asserts the qf_card contract is read-only,
// generic for unknown decision types, signed-link-first, and limited to public
// trust fields.
func TestQFDecisionPacketMetadataPersistsIntoSearchRenderCard(t *testing.T) {
	pool := testPool(t)
	natsClient := qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sourceID := "qf-decisions-it-render-" + uniqueSuffix()
	cleanupQFDecisionsRows(t, pool, sourceID)
	t.Cleanup(func() { cleanupQFDecisionsRows(t, pool, sourceID) })

	packetID := "packet-render-" + uniqueSuffix()
	uniqueThesis := "QF render integration thesis " + uniqueSuffix()
	envelope := validIntegrationEnvelope(packetID, "intent-render", "scenario-render", "trace-render", uniqueThesis)
	envelope.DecisionType = "future_qf_decision_type"
	envelope.PacketURLSigned = "https://qf.example.test/signed/" + packetID
	envelope.SignatureExpiresAt = time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339)
	envelope.PreferredSurface = qfdecisions.PreferredSurfaceSmackerelTelegram
	envelope.CalibrationBadge = publicTrustObject("calibration", "medium", "calibration public summary")
	envelope.DataProvenanceBadge = publicTrustObject("provenance", "low", "provenance public summary")
	envelope.QuantifiedImpact = publicTrustObject("impact", "high", "impact public summary")
	envelope.ExpertAnalysisBundle = publicTrustObject("analysis", "medium", "analysis public summary")
	artifact, diagnostic := qfdecisions.NewNormalizer(sourceID, 1).Normalize(
		eventForPacket("event-render", packetID, envelope.IntentID, envelope.ScenarioID, envelope.TraceID),
		envelope,
		time.Now().UTC(),
	)
	if diagnostic != nil {
		t.Fatalf("Normalize diagnostic = %#v, want trusted artifact", diagnostic)
	}
	if artifact == nil {
		t.Fatal("Normalize returned nil artifact")
	}

	publisher := pipeline.NewRawArtifactPublisher(pool, natsClient)
	artifactID, err := publisher.PublishRawArtifact(ctx, *artifact)
	if err != nil {
		t.Fatalf("PublishRawArtifact: %v", err)
	}
	if artifactID == "" {
		t.Fatal("PublishRawArtifact returned empty artifact ID")
	}

	var storedSignedLink string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(metadata->>'packet_url_signed', '') FROM artifacts WHERE id = $1`, artifactID).Scan(&storedSignedLink); err != nil {
		t.Fatalf("read persisted packet_url_signed metadata: %v", err)
	}
	if storedSignedLink != envelope.PacketURLSigned {
		t.Fatalf("persisted packet_url_signed = %q, want %q", storedSignedLink, envelope.PacketURLSigned)
	}

	engine := &api.SearchEngine{Pool: pool}
	results, _, mode, err := engine.Search(ctx, api.SearchRequest{Query: uniqueThesis, Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if mode != "text_fallback" {
		t.Fatalf("search mode = %q, want text_fallback with no ML sidecar", mode)
	}

	var found *api.SearchResult
	for i := range results {
		if results[i].ArtifactID == artifactID {
			found = &results[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("search did not return persisted QF artifact %s; results=%#v", artifactID, results)
	}
	if found.QFCard == nil {
		t.Fatalf("search result %s missing qf_card", artifactID)
	}
	card := found.QFCard
	if card.CardKind != qfdecisions.CardKindGenericPacket {
		t.Fatalf("qf_card.card_kind = %q, want %q for unknown decision_type", card.CardKind, qfdecisions.CardKindGenericPacket)
	}
	if !card.ReadOnly || card.ActionEligible {
		t.Fatalf("qf_card read/action state = readOnly:%v actionEligible:%v, want read-only/no-action", card.ReadOnly, card.ActionEligible)
	}
	if card.DeepLink.Status != qfdecisions.DeepLinkStatusSignedUsed || card.DeepLink.URL != envelope.PacketURLSigned {
		t.Fatalf("qf_card deep_link = %#v, want signed_used %q", card.DeepLink, envelope.PacketURLSigned)
	}
	if !card.Placement.QueueTelegram || card.Placement.IncludeInDigest || card.Placement.ShowInQFDashboard {
		t.Fatalf("preferred_surface placement mutated beyond telegram routing: %#v", card.Placement)
	}
	rendered, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal qf_card: %v", err)
	}
	for _, forbidden := range []string{"confidence", "score", "value"} {
		if strings.Contains(string(rendered), forbidden) {
			t.Fatalf("qf_card leaked non-public trust field %q: %s", forbidden, rendered)
		}
	}
	if !strings.Contains(string(rendered), "calibration public summary") {
		t.Fatalf("qf_card missing public trust summary: %s", rendered)
	}
}

func publicTrustObject(label, severity, summary string) map[string]any {
	return map[string]any{
		"label":      label,
		"severity":   severity,
		"summary":    summary,
		"detail":     summary + " detail",
		"links":      []any{map[string]any{"label": "QF", "url": "https://qf.example.test/reference"}},
		"confidence": 0.91,
		"score":      42,
		"value":      12.5,
	}
}
